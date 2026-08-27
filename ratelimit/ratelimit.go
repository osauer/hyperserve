package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultIdleTTL       = 10 * time.Minute
	defaultMaxClients    = 10_000
	maxClientKeyBytes    = 512
	maxForwardedForHops  = 64
	clientIdentityStatus = http.StatusBadRequest
)

// KeyFunc returns the quota key for a request. Keys must be non-empty and at
// most 512 bytes. Returning an error rejects the request without invoking the
// next handler.
type KeyFunc func(*http.Request) (string, error)

// Config configures one independent rate-limit quota namespace.
type Config struct {
	// RequestsPerSecond is the sustained token refill rate. It must be finite
	// and greater than zero.
	RequestsPerSecond float64

	// Burst is the maximum number of tokens available at once. It must be
	// greater than zero.
	Burst int

	// ClientKey selects the quota identity. Nil uses the normalized transport
	// peer from Request.RemoteAddr and never trusts forwarding headers.
	ClientKey KeyFunc

	// IdleTTL controls when an unused client bucket becomes eligible for
	// opportunistic pruning. Zero selects the finite default of 10 minutes.
	IdleTTL time.Duration

	// MaxClients bounds the number of client buckets. Zero selects the finite
	// default of 10,000. At capacity, new clients are rejected rather than
	// replacing an active bucket.
	MaxClients int
}

type normalizedConfig struct {
	requestsPerSecond float64
	burst             int
	clientKey         KeyFunc
	idleTTL           time.Duration
	maxClients        int
}

type policy struct {
	config normalizedConfig

	mu      sync.RWMutex
	clients map[string]*clientEntry
	// nextPruneUnix is a conservative lower bound on the earliest bucket
	// expiry. A refreshed entry can make it early but never late, which avoids
	// a full map scan for every unknown key during a capacity attack.
	nextPruneUnix int64
}

type clientEntry struct {
	mu       sync.Mutex
	bucket   *rate.Limiter
	lastSeen atomic.Int64
	active   atomic.Int64
}

type chargedPolicyMarker struct{}

// New validates config and returns standard net/http middleware. Each call to
// New owns a separate quota namespace. Reusing the returned function shares its
// quotas, including when it is mounted on multiple paths.
func New(config Config) (func(http.Handler) http.Handler, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	p := &policy{
		config:  normalized,
		clients: make(map[string]*clientEntry),
	}
	return p.middleware, nil
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	if math.IsNaN(config.RequestsPerSecond) || math.IsInf(config.RequestsPerSecond, 0) || config.RequestsPerSecond <= 0 {
		return normalizedConfig{}, errors.New("ratelimit: RequestsPerSecond must be finite and greater than zero")
	}
	if config.Burst <= 0 {
		return normalizedConfig{}, errors.New("ratelimit: Burst must be greater than zero")
	}
	if config.IdleTTL < 0 {
		return normalizedConfig{}, errors.New("ratelimit: IdleTTL must not be negative")
	}
	if config.MaxClients < 0 {
		return normalizedConfig{}, errors.New("ratelimit: MaxClients must not be negative")
	}

	idleTTL := config.IdleTTL
	if idleTTL == 0 {
		idleTTL = defaultIdleTTL
	}
	maxClients := config.MaxClients
	if maxClients == 0 {
		maxClients = defaultMaxClients
	}
	clientKey := config.ClientKey
	if clientKey == nil {
		clientKey = transportPeerClientKey
	}

	return normalizedConfig{
		requestsPerSecond: config.RequestsPerSecond,
		burst:             config.Burst,
		clientKey:         clientKey,
		idleTTL:           idleTTL,
		maxClients:        maxClients,
	}, nil
}

func (p *policy) middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wasCharged(r.Context(), p) {
			next.ServeHTTP(w, r)
			return
		}

		key, err := p.config.clientKey(r)
		if err != nil || strings.TrimSpace(key) == "" || len(key) > maxClientKeyBytes {
			http.Error(w, http.StatusText(clientIdentityStatus), clientIdentityStatus)
			return
		}

		entry, capacityRetry, ok := p.entryFor(key, time.Now())
		if !ok {
			writeTooManyRequests(w, capacityRetry, "rate limit client capacity exceeded")
			return
		}

		allowed, quotaRetry := entry.allow(time.Now())
		if !allowed {
			writeTooManyRequests(w, quotaRetry, "rate limit exceeded")
			return
		}

		ctx := context.WithValue(r.Context(), p, chargedPolicyMarker{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func wasCharged(ctx context.Context, candidate *policy) bool {
	_, charged := ctx.Value(candidate).(chargedPolicyMarker)
	return charged
}

func (p *policy) entryFor(key string, now time.Time) (*clientEntry, time.Duration, bool) {
	cutoff := now.Add(-p.config.idleTTL).UnixNano()
	nowUnix := now.UnixNano()

	p.mu.RLock()
	entry, exists := p.clients[key]
	if exists && entry.lastSeen.Load() > cutoff {
		entry.touch(nowUnix)
		entry.active.Add(1)
		p.mu.RUnlock()
		return entry, 0, true
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Another request may have created or refreshed this key while the write
	// lock was pending.
	if entry, exists = p.clients[key]; exists {
		if entry.lastSeen.Load() > cutoff {
			entry.touch(nowUnix)
			entry.active.Add(1)
			return entry, 0, true
		}
		if entry.active.Load() > 0 {
			// A request can outlive a deliberately tiny IdleTTL while waiting
			// for this client's token lock. Keep that live bucket authoritative.
			entry.touch(nowUnix)
			entry.active.Add(1)
			return entry, 0, true
		}
		delete(p.clients, key)
	}

	// If the lower bound is still in the future, no bucket can yet be stale.
	// Once it is reached, one scan both prunes and computes the next bound.
	if p.nextPruneUnix == 0 || nowUnix >= p.nextPruneUnix {
		p.pruneExpiredLocked(cutoff, nowUnix)
	}

	if len(p.clients) >= p.config.maxClients {
		return nil, p.capacityRetryLocked(nowUnix), false
	}

	entry = &clientEntry{
		bucket: rate.NewLimiter(rate.Limit(p.config.requestsPerSecond), p.config.burst),
	}
	entry.lastSeen.Store(nowUnix)
	entry.active.Store(1)
	// Clone prevents a short custom key from retaining an arbitrarily large
	// backing string for the lifetime of the bucket.
	p.clients[strings.Clone(key)] = entry
	expires := expiryUnixNano(nowUnix, p.config.idleTTL)
	if p.nextPruneUnix == 0 || expires < p.nextPruneUnix {
		p.nextPruneUnix = expires
	}
	return entry, 0, true
}

func (p *policy) pruneExpiredLocked(cutoff, nowUnix int64) {
	p.nextPruneUnix = 0
	for key, entry := range p.clients {
		lastSeen := entry.lastSeen.Load()
		if lastSeen <= cutoff {
			if entry.active.Load() == 0 {
				delete(p.clients, key)
				continue
			}
			// An in-flight request is active even if a very short TTL elapsed
			// while it waited for its per-client token decision.
			lastSeen = entry.touch(nowUnix)
		}
		expires := expiryUnixNano(lastSeen, p.config.idleTTL)
		if p.nextPruneUnix == 0 || expires < p.nextPruneUnix {
			p.nextPruneUnix = expires
		}
	}
}

func (p *policy) capacityRetryLocked(nowUnix int64) time.Duration {
	if p.nextPruneUnix <= nowUnix {
		return time.Nanosecond
	}
	return time.Duration(p.nextPruneUnix - nowUnix)
}

func expiryUnixNano(lastSeen int64, idleTTL time.Duration) int64 {
	if lastSeen > math.MaxInt64-int64(idleTTL) {
		return math.MaxInt64
	}
	return lastSeen + int64(idleTTL)
}

func (entry *clientEntry) touch(nowUnix int64) int64 {
	for {
		lastSeen := entry.lastSeen.Load()
		if nowUnix <= lastSeen {
			return lastSeen
		}
		if entry.lastSeen.CompareAndSwap(lastSeen, nowUnix) {
			return nowUnix
		}
	}
}

func (entry *clientEntry) allow(now time.Time) (bool, time.Duration) {
	defer entry.active.Add(-1)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.bucket.AllowN(now, 1) {
		return true, 0
	}

	reservation := entry.bucket.ReserveN(now, 1)
	if !reservation.OK() {
		return false, time.Nanosecond
	}
	delay := reservation.DelayFrom(now)
	reservation.CancelAt(now)
	if delay <= 0 {
		delay = time.Nanosecond
	}
	return false, delay
}

func writeTooManyRequests(w http.ResponseWriter, retryAfter time.Duration, message string) {
	seconds := retryAfterSeconds(retryAfter)
	w.Header().Set("Retry-After", seconds)
	// RateLimit-Reset uses the same computed delta as Retry-After. The delta
	// comes from the token reservation or earliest idle expiry that rejected
	// this request.
	w.Header().Set("RateLimit-Reset", seconds)
	http.Error(w, message, http.StatusTooManyRequests)
}

func retryAfterSeconds(delay time.Duration) string {
	seconds := max(int64(math.Ceil(delay.Seconds())), 1)
	return strconv.FormatInt(seconds, 10)
}

func transportPeerClientKey(r *http.Request) (string, error) {
	peer, err := transportPeer(r)
	if err != nil {
		return "", err
	}
	return peer.String(), nil
}

func transportPeer(r *http.Request) (netip.Addr, error) {
	if r == nil {
		return netip.Addr{}, errors.New("ratelimit: nil request")
	}
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("ratelimit: invalid RemoteAddr: %w", err)
	}
	return peer.Addr().Unmap(), nil
}

// TrustedProxyClientKey returns a KeyFunc that accepts X-Forwarded-For only
// from an immediate transport peer in proxyRanges. It parses every hop and
// walks right-to-left, through trusted proxies, to the first untrusted client.
// Malformed headers, headers received from an untrusted peer, and chains with
// no untrusted client fail closed. With no X-Forwarded-For header, the transport
// peer remains the client identity.
//
// The prefix slice is validated, normalized, and copied. At least one valid,
// unzoned prefix is required.
func TrustedProxyClientKey(proxyRanges []netip.Prefix) (KeyFunc, error) {
	trusted, err := normalizeProxyRanges(proxyRanges)
	if err != nil {
		return nil, err
	}

	return func(r *http.Request) (string, error) {
		peer, err := transportPeer(r)
		if err != nil {
			return "", err
		}

		values := r.Header.Values("X-Forwarded-For")
		if len(values) == 0 {
			return peer.String(), nil
		}
		if peer.Zone() != "" || !addressInPrefixes(peer, trusted) {
			return "", errors.New("ratelimit: X-Forwarded-For received from an untrusted transport peer")
		}

		hops, err := parseForwardedFor(values)
		if err != nil {
			return "", err
		}
		for _, hop := range slices.Backward(hops) {
			if !addressInPrefixes(hop, trusted) {
				return hop.String(), nil
			}
		}
		return "", errors.New("ratelimit: X-Forwarded-For contains no untrusted client hop")
	}, nil
}

func normalizeProxyRanges(proxyRanges []netip.Prefix) ([]netip.Prefix, error) {
	if len(proxyRanges) == 0 {
		return nil, errors.New("ratelimit: TrustedProxyClientKey requires at least one proxy range")
	}

	trusted := make([]netip.Prefix, len(proxyRanges))
	for i, proxyRange := range proxyRanges {
		if !proxyRange.IsValid() || proxyRange.Addr().Zone() != "" {
			return nil, fmt.Errorf("ratelimit: proxy range %d is invalid", i)
		}
		if proxyRange.Addr().Is4In6() {
			return nil, fmt.Errorf("ratelimit: proxy range %d uses an IPv4-mapped IPv6 address; use an IPv4 prefix", i)
		}
		trusted[i] = proxyRange.Masked()
	}
	return trusted, nil
}

func parseForwardedFor(values []string) ([]netip.Addr, error) {
	hops := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		for rawHop := range strings.SplitSeq(value, ",") {
			if len(hops) == maxForwardedForHops {
				return nil, fmt.Errorf("ratelimit: X-Forwarded-For exceeds %d hops", maxForwardedForHops)
			}
			rawHop = strings.TrimSpace(rawHop)
			if rawHop == "" {
				return nil, errors.New("ratelimit: X-Forwarded-For contains an empty hop")
			}
			hop, err := netip.ParseAddr(rawHop)
			if err != nil || hop.Zone() != "" {
				return nil, fmt.Errorf("ratelimit: X-Forwarded-For contains invalid address %q", rawHop)
			}
			hops = append(hops, hop.Unmap())
		}
	}
	if len(hops) == 0 {
		return nil, errors.New("ratelimit: X-Forwarded-For is empty")
	}
	return hops, nil
}

func addressInPrefixes(address netip.Addr, prefixes []netip.Prefix) bool {
	address = address.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
