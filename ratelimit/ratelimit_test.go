package ratelimit

import (
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{
			name:   "zero rate does not silently disable",
			config: Config{RequestsPerSecond: 0, Burst: 1},
		},
		{
			name:   "negative rate",
			config: Config{RequestsPerSecond: -1, Burst: 1},
		},
		{
			name:   "NaN rate",
			config: Config{RequestsPerSecond: math.NaN(), Burst: 1},
		},
		{
			name:   "positive infinite rate",
			config: Config{RequestsPerSecond: math.Inf(1), Burst: 1},
		},
		{
			name:   "negative infinite rate",
			config: Config{RequestsPerSecond: math.Inf(-1), Burst: 1},
		},
		{
			name:   "zero burst",
			config: Config{RequestsPerSecond: 1, Burst: 0},
		},
		{
			name:   "negative burst",
			config: Config{RequestsPerSecond: 1, Burst: -1},
		},
		{
			name:   "negative idle TTL",
			config: Config{RequestsPerSecond: 1, Burst: 1, IdleTTL: -time.Nanosecond},
		},
		{
			name:   "negative max clients",
			config: Config{RequestsPerSecond: 1, Burst: 1, MaxClients: -1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			middleware, err := New(test.config)
			if err == nil {
				t.Fatal("New() error = nil, want validation error")
			}
			if middleware != nil {
				t.Fatal("New() middleware is non-nil on validation failure")
			}
		})
	}
}

func TestNewAppliesFiniteDefaults(t *testing.T) {
	t.Parallel()

	config, err := normalizeConfig(Config{RequestsPerSecond: 1, Burst: 1})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	if config.retention != 10*time.Minute {
		t.Fatalf("effective retention = %v, want 10m", config.retention)
	}
	if defaultIdleTTL != 10*time.Minute {
		t.Fatalf("defaultIdleTTL = %v, want 10m", defaultIdleTTL)
	}
	if config.maxClients != 10_000 {
		t.Fatalf("MaxClients default = %d, want 10000", config.maxClients)
	}
	if defaultMaxClients != 10_000 {
		t.Fatalf("defaultMaxClients = %d, want 10000", defaultMaxClients)
	}
	if config.clientKey == nil {
		t.Fatal("default ClientKey is nil")
	}
}

func TestEffectiveRetentionCoversFullRefill(t *testing.T) {
	t.Parallel()
	if got := fullRefillDuration(1, 2_000_000_000); got != time.Nanosecond {
		t.Fatalf("fractional-nanosecond full refill = %v, want conservative 1ns ceiling", got)
	}

	tests := []struct {
		name   string
		config Config
		want   time.Duration
	}{
		{
			name: "configured idle TTL is longer",
			config: Config{
				RequestsPerSecond: 10,
				Burst:             20,
				IdleTTL:           3 * time.Second,
			},
			want: 3 * time.Second,
		},
		{
			name: "full refill is longer",
			config: Config{
				RequestsPerSecond: 2,
				Burst:             10,
				IdleTTL:           time.Second,
			},
			want: 5 * time.Second,
		},
		{
			name: "extremely slow refill saturates",
			config: Config{
				RequestsPerSecond: math.SmallestNonzeroFloat64,
				Burst:             1,
				IdleTTL:           time.Nanosecond,
			},
			want: time.Duration(math.MaxInt64),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config, err := normalizeConfig(test.config)
			if err != nil {
				t.Fatalf("normalizeConfig() error = %v", err)
			}
			if config.retention != test.want {
				t.Fatalf("effective retention = %v, want %v", config.retention, test.want)
			}
		})
	}
}

func TestDefaultClientKeyNormalizesTransportPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		first  string
		second string
	}{
		{
			name:   "IPv4 ports",
			first:  "192.0.2.10:1000",
			second: "192.0.2.10:2000",
		},
		{
			name:   "IPv4 mapped IPv6",
			first:  "[::ffff:192.0.2.11]:1000",
			second: "192.0.2.11:2000",
		},
		{
			name:   "IPv6 textual forms",
			first:  "[2001:db8::12]:1000",
			second: "[2001:0db8:0000:0000:0000:0000:0000:0012]:2000",
		},
		{
			name:   "IPv6 zone",
			first:  "[fe80::12%en0]:1000",
			second: "[fe80:0:0:0:0:0:0:12%en0]:2000",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			gate := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})
			handler := gate(okHandler())

			if got := serve(t, handler, test.first, "").Code; got != http.StatusOK {
				t.Fatalf("first status = %d, want %d", got, http.StatusOK)
			}
			if got := serve(t, handler, test.second, "").Code; got != http.StatusTooManyRequests {
				t.Fatalf("second status = %d, want %d; addresses should share a normalized key", got, http.StatusTooManyRequests)
			}
		})
	}
}

func TestDefaultClientKeyNeverTrustsForwardingHeaders(t *testing.T) {
	t.Parallel()

	t.Run("same peer cannot rotate XFF", func(t *testing.T) {
		t.Parallel()
		gate := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})
		handler := gate(okHandler())

		if got := serve(t, handler, "192.0.2.20:1000", "198.51.100.1").Code; got != http.StatusOK {
			t.Fatalf("first status = %d, want %d", got, http.StatusOK)
		}
		if got := serve(t, handler, "192.0.2.20:2000", "198.51.100.2").Code; got != http.StatusTooManyRequests {
			t.Fatalf("second status = %d, want %d", got, http.StatusTooManyRequests)
		}
	})

	t.Run("different peers do not collapse under one XFF", func(t *testing.T) {
		t.Parallel()
		gate := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})
		handler := gate(okHandler())

		for _, remoteAddr := range []string{"192.0.2.21:1000", "192.0.2.22:1000"} {
			if got := serve(t, handler, remoteAddr, "198.51.100.1").Code; got != http.StatusOK {
				t.Fatalf("status for %s = %d, want %d", remoteAddr, got, http.StatusOK)
			}
		}
	})
}

func TestMalformedClientIdentityFailsClosed(t *testing.T) {
	t.Parallel()

	gate := mustGate(t, Config{RequestsPerSecond: 1, Burst: 1})
	handler := gate(okHandler())
	for _, remoteAddr := range []string{
		"",
		"192.0.2.30",
		"example.com:1234",
		"192.0.2.30:not-a-port",
		"2001:db8::30:1234",
	} {
		recorder := serve(t, handler, remoteAddr, "")
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("RemoteAddr %q status = %d, want %d", remoteAddr, recorder.Code, http.StatusBadRequest)
		}
	}

	for name, keyFunc := range map[string]KeyFunc{
		"error": func(*http.Request) (string, error) { return "", errors.New("no identity") },
		"empty": func(*http.Request) (string, error) { return "", nil },
		"blank": func(*http.Request) (string, error) { return "  ", nil },
		"oversized": func(*http.Request) (string, error) {
			return strings.Repeat("k", maxClientKeyBytes+1), nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			customGate := mustGate(t, Config{
				RequestsPerSecond: 1,
				Burst:             1,
				ClientKey:         keyFunc,
			})
			if got := serve(t, customGate(okHandler()), "192.0.2.31:1234", "").Code; got != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
			}
		})
	}
}

func TestTrustedProxyClientKey(t *testing.T) {
	t.Parallel()

	proxyRanges := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("2001:db8:1::/48"),
	}
	keyFunc, err := TrustedProxyClientKey(proxyRanges)
	if err != nil {
		t.Fatalf("TrustedProxyClientKey() error = %v", err)
	}

	tests := []struct {
		name       string
		remoteAddr string
		headers    []string
		want       string
	}{
		{
			name:       "no forwarding header uses direct peer",
			remoteAddr: "198.51.100.10:1234",
			want:       "198.51.100.10",
		},
		{
			name:       "walks trusted IPv4 proxies from right",
			remoteAddr: "10.0.0.2:1234",
			headers:    []string{"198.51.100.11, 192.168.1.5"},
			want:       "198.51.100.11",
		},
		{
			name:       "ignores spoofed hops left of nearest untrusted client",
			remoteAddr: "10.0.0.2:1234",
			headers:    []string{"203.0.113.99, 198.51.100.12"},
			want:       "198.51.100.12",
		},
		{
			name:       "parses multiple header fields",
			remoteAddr: "10.0.0.2:1234",
			headers:    []string{"198.51.100.13", "192.168.1.5"},
			want:       "198.51.100.13",
		},
		{
			name:       "normalizes IPv4 mapped client",
			remoteAddr: "10.0.0.2:1234",
			headers:    []string{"::ffff:198.51.100.14"},
			want:       "198.51.100.14",
		},
		{
			name:       "walks trusted IPv6 proxies",
			remoteAddr: "[2001:db8:1::2]:1234",
			headers:    []string{"2001:db8:2::15, 2001:db8:1::3"},
			want:       "2001:db8:2::15",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remoteAddr
			for _, header := range test.headers {
				request.Header.Add("X-Forwarded-For", header)
			}
			got, err := keyFunc(request)
			if err != nil {
				t.Fatalf("KeyFunc() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("KeyFunc() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTrustedProxyClientKeyFailsClosed(t *testing.T) {
	t.Parallel()

	keyFunc, err := TrustedProxyClientKey([]netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	})
	if err != nil {
		t.Fatalf("TrustedProxyClientKey() error = %v", err)
	}

	tooManyHops := strings.TrimSuffix(strings.Repeat("198.51.100.1,", maxForwardedForHops+1), ",")
	tests := []struct {
		name       string
		remoteAddr string
		header     string
	}{
		{
			name:       "untrusted immediate peer",
			remoteAddr: "192.0.2.40:1234",
			header:     "198.51.100.40",
		},
		{
			name:       "malformed hop",
			remoteAddr: "10.0.0.2:1234",
			header:     "198.51.100.40, definitely-not-an-ip",
		},
		{
			name:       "empty hop",
			remoteAddr: "10.0.0.2:1234",
			header:     "198.51.100.40,,10.0.0.3",
		},
		{
			name:       "all hops are trusted proxies",
			remoteAddr: "10.0.0.2:1234",
			header:     "10.0.0.3,10.0.0.4",
		},
		{
			name:       "zoned forwarded address",
			remoteAddr: "10.0.0.2:1234",
			header:     "fe80::1%en0",
		},
		{
			name:       "hop count is bounded",
			remoteAddr: "10.0.0.2:1234",
			header:     tooManyHops,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.header)
			if got, err := keyFunc(request); err == nil {
				t.Fatalf("KeyFunc() = %q, nil error; want fail-closed error", got)
			}
		})
	}
}

func TestForwardedForByteLimitAndNonReflectiveErrors(t *testing.T) {
	t.Parallel()

	t.Run("exact aggregate boundary is accepted by size check", func(t *testing.T) {
		t.Parallel()
		for _, values := range [][]string{
			{strings.Repeat("x", maxForwardedForBytes)},
			{
				strings.Repeat("x", maxForwardedForBytes/2),
				strings.Repeat("y", maxForwardedForBytes/2-1),
			},
		} {
			_, err := parseForwardedFor(values)
			if err == nil {
				t.Fatal("parseForwardedFor() error = nil for invalid address")
			}
			if strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("exact-boundary error = %q, size check rejected %d bytes", err, maxForwardedForBytes)
			}
		}
	})

	t.Run("single field over aggregate boundary", func(t *testing.T) {
		t.Parallel()
		_, err := parseForwardedFor([]string{strings.Repeat("x", maxForwardedForBytes+1)})
		if err == nil || !strings.Contains(err.Error(), "exceeds 4096 bytes") {
			t.Fatalf("parseForwardedFor() error = %v, want byte-limit error", err)
		}
	})

	t.Run("multiple fields count their joining comma", func(t *testing.T) {
		t.Parallel()
		_, err := parseForwardedFor([]string{
			strings.Repeat("x", maxForwardedForBytes/2),
			strings.Repeat("y", maxForwardedForBytes/2),
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds 4096 bytes") {
			t.Fatalf("parseForwardedFor() error = %v, want aggregate byte-limit error", err)
		}
	})

	t.Run("invalid address is not reflected", func(t *testing.T) {
		t.Parallel()
		const attackerValue = "attacker-controlled-secret.invalid"
		_, err := parseForwardedFor([]string{attackerValue})
		if err == nil {
			t.Fatal("parseForwardedFor() error = nil for invalid address")
		}
		if strings.Contains(err.Error(), attackerValue) {
			t.Fatalf("parseForwardedFor() error reflected attacker input: %q", err)
		}
	})
}

func TestOversizedForwardedForFailsClosedInMiddleware(t *testing.T) {
	t.Parallel()

	keyFunc, err := TrustedProxyClientKey([]netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
	})
	if err != nil {
		t.Fatalf("TrustedProxyClientKey() error = %v", err)
	}
	gate := mustGate(t, Config{
		RequestsPerSecond: 1,
		Burst:             1,
		ClientKey:         keyFunc,
	})
	var handled atomic.Bool
	handler := gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.0.0.2:1234"
	request.Header.Set("X-Forwarded-For", strings.Repeat("x", maxForwardedForBytes+1))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if handled.Load() {
		t.Fatal("next handler ran for oversized X-Forwarded-For")
	}
}

func TestTrustedProxyClientKeyValidatesAndCopiesRanges(t *testing.T) {
	t.Parallel()

	if _, err := TrustedProxyClientKey(nil); err == nil {
		t.Fatal("TrustedProxyClientKey(nil) error = nil")
	}
	if _, err := TrustedProxyClientKey([]netip.Prefix{{}}); err == nil {
		t.Fatal("TrustedProxyClientKey(invalid prefix) error = nil")
	}
	mapped := netip.PrefixFrom(netip.MustParseAddr("::ffff:10.0.0.0"), 104)
	if _, err := TrustedProxyClientKey([]netip.Prefix{mapped}); err == nil {
		t.Fatal("TrustedProxyClientKey(mapped prefix) error = nil")
	}

	ranges := []netip.Prefix{netip.PrefixFrom(netip.MustParseAddr("10.1.2.3"), 8)}
	keyFunc, err := TrustedProxyClientKey(ranges)
	if err != nil {
		t.Fatalf("TrustedProxyClientKey() error = %v", err)
	}
	ranges[0] = netip.MustParsePrefix("192.168.0.0/16")

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "10.9.8.7:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.50")
	got, err := keyFunc(request)
	if err != nil {
		t.Fatalf("copied KeyFunc() error = %v", err)
	}
	if got != "198.51.100.50" {
		t.Fatalf("copied KeyFunc() = %q, want %q", got, "198.51.100.50")
	}
}

func TestQuotaNamespacesAreSharedOnlyWhenMiddlewareIsReused(t *testing.T) {
	t.Parallel()

	shared := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})
	firstShared := shared(okHandler())
	secondShared := shared(okHandler())
	if got := serve(t, firstShared, "192.0.2.60:1000", "").Code; got != http.StatusOK {
		t.Fatalf("first shared status = %d, want %d", got, http.StatusOK)
	}
	if got := serve(t, secondShared, "192.0.2.60:2000", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second shared status = %d, want %d", got, http.StatusTooManyRequests)
	}

	firstIsolated := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})(okHandler())
	secondIsolated := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})(okHandler())
	for i, handler := range []http.Handler{firstIsolated, secondIsolated} {
		if got := serve(t, handler, "192.0.2.61:1000", "").Code; got != http.StatusOK {
			t.Fatalf("isolated handler %d status = %d, want %d", i, got, http.StatusOK)
		}
	}
}

func TestReusedMiddlewareChargesAtMostOncePerRequest(t *testing.T) {
	t.Parallel()

	gate := mustGate(t, Config{RequestsPerSecond: 0.001, Burst: 1})
	var handled atomic.Int64
	handler := gate(gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled.Add(1)
		w.WriteHeader(http.StatusOK)
	})))

	if got := serve(t, handler, "192.0.2.70:1000", "").Code; got != http.StatusOK {
		t.Fatalf("first status = %d, want %d; nested reuse charged twice", got, http.StatusOK)
	}
	if got := handled.Load(); got != 1 {
		t.Fatalf("handled count after first request = %d, want 1", got)
	}
	if got := serve(t, handler, "192.0.2.70:2000", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", got, http.StatusTooManyRequests)
	}
}

func TestCapacityRejectsNewKeysWithoutEvictingActiveClients(t *testing.T) {
	t.Parallel()

	gate := mustGate(t, Config{
		RequestsPerSecond: 1_000,
		Burst:             2,
		IdleTTL:           4 * time.Second,
		MaxClients:        2,
	})
	handler := gate(okHandler())

	for _, remoteAddr := range []string{"192.0.2.80:1000", "192.0.2.81:1000"} {
		if got := serve(t, handler, remoteAddr, "").Code; got != http.StatusOK {
			t.Fatalf("initial status for %s = %d, want %d", remoteAddr, got, http.StatusOK)
		}
	}

	capacity := serve(t, handler, "192.0.2.82:1000", "")
	if capacity.Code != http.StatusTooManyRequests {
		t.Fatalf("new key at capacity status = %d, want %d", capacity.Code, http.StatusTooManyRequests)
	}
	if !strings.Contains(capacity.Body.String(), "capacity") {
		t.Fatalf("capacity response body = %q, want capacity reason", capacity.Body.String())
	}
	seconds := assertRetryHeaders(t, capacity)
	if seconds < 3 || seconds > 4 {
		t.Fatalf("capacity Retry-After = %d seconds, want 3-4 from effective-retention backoff", seconds)
	}

	if got := serve(t, handler, "192.0.2.80:2000", "").Code; got != http.StatusOK {
		t.Fatalf("existing key at capacity status = %d, want %d", got, http.StatusOK)
	}
}

func TestCapacityRetryDoesNotUnderstateRefreshedExpiry(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 10,
		Burst:             1,
		IdleTTL:           10 * time.Second,
		MaxClients:        1,
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	p := &policy{
		config:  config,
		clients: make(map[string]*clientEntry),
	}

	entry, _, ok := p.entryFor("existing", start)
	if !ok {
		t.Fatal("initial entryFor() rejected")
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start }); !allowed {
		t.Fatal("initial token was rejected")
	}

	refreshTime := start.Add(9 * time.Second)
	refreshed, _, ok := p.entryFor("existing", refreshTime)
	if !ok || refreshed != entry {
		t.Fatal("existing entry was not refreshed in place")
	}
	refreshed.allowAt(func() time.Time { return refreshTime })

	attackTime := start.Add(9*time.Second + 500*time.Millisecond)
	if _, retry, accepted := p.entryFor("new", attackTime); accepted {
		t.Fatal("new entry was accepted at capacity")
	} else {
		untilRefreshedExpiry := refreshTime.Add(config.retention).Sub(attackTime)
		if retry < untilRefreshedExpiry {
			t.Fatalf("capacity retry = %v, understates refreshed expiry in %v", retry, untilRefreshedExpiry)
		}
		if retry != config.retention {
			t.Fatalf("capacity retry = %v, want conservative effective retention %v", retry, config.retention)
		}
	}
}

func TestFullPoolMissDoesNotRequireWriterLockBeforeExpiry(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 100,
		Burst:             1,
		IdleTTL:           time.Minute,
		MaxClients:        1,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &policy{config: config, clients: make(map[string]*clientEntry)}
	entry, _, ok := p.entryFor("existing", start)
	if !ok {
		t.Fatal("initial entry rejected")
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start }); !allowed {
		t.Fatal("initial token rejected")
	}

	// Holding a read lock blocks any writer but permits the full-pool fast
	// rejection to take its own read lock. This makes writer-lock regression
	// deterministic without relying on timing under load.
	p.mu.RLock()
	type result struct {
		retry    time.Duration
		accepted bool
	}
	resultCh := make(chan result, 1)
	go func() {
		_, retry, accepted := p.entryFor("rotating-miss", start.Add(time.Second))
		resultCh <- result{retry: retry, accepted: accepted}
	}()

	select {
	case got := <-resultCh:
		p.mu.RUnlock()
		if got.accepted {
			t.Fatal("full-pool miss was accepted")
		}
		if got.retry != config.retention {
			t.Fatalf("retry = %v, want conservative retention %v", got.retry, config.retention)
		}
	case <-time.After(500 * time.Millisecond):
		p.mu.RUnlock()
		<-resultCh
		t.Fatal("full-pool miss waited for the writer lock")
	}
}

func TestExpiredEntriesArePrunedAndQuotaResets(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 1,
		Burst:             1,
		IdleTTL:           2 * time.Second,
		MaxClients:        1,
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	p := &policy{
		config:  config,
		clients: make(map[string]*clientEntry),
	}

	entry, _, ok := p.entryFor("first", start)
	if !ok {
		t.Fatal("initial entryFor() rejected")
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start }); !allowed {
		t.Fatal("initial token was rejected")
	}
	entry, _, ok = p.entryFor("first", start)
	if !ok {
		t.Fatal("pre-expiry entryFor() rejected existing key")
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start }); allowed {
		t.Fatal("pre-expiry token was allowed")
	}

	firstExpiry := start.Add(config.retention)
	replacement, _, ok := p.entryFor("first", firstExpiry)
	if !ok {
		t.Fatal("same key was rejected after fully refilled bucket expired")
	}
	if replacement == entry {
		t.Fatal("expired bucket was not replaced")
	}
	if allowed, _ := replacement.allowAt(func() time.Time { return firstExpiry }); !allowed {
		t.Fatal("replacement token was rejected")
	}

	secondExpiry := firstExpiry.Add(config.retention)
	newEntry, _, ok := p.entryFor("second", secondExpiry)
	if !ok {
		t.Fatal("new key was rejected after existing bucket expired")
	}
	newEntry.allowAt(func() time.Time { return secondExpiry })
}

func TestSlowRefillBucketCannotResetBeforeFullRefill(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 0.001,
		Burst:             1,
		IdleTTL:           15 * time.Millisecond,
		MaxClients:        1,
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	p := &policy{
		config:  config,
		clients: make(map[string]*clientEntry),
	}

	entry, _, ok := p.entryFor("slow", start)
	if !ok {
		t.Fatal("initial entryFor() rejected")
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start }); !allowed {
		t.Fatal("initial token was rejected")
	}
	afterConfiguredTTL := start.Add(40 * time.Millisecond)
	sameEntry, _, ok := p.entryFor("slow", afterConfiguredTTL)
	if !ok || sameEntry != entry {
		t.Fatal("slow bucket reset after configured IdleTTL but before full refill")
	}
	if allowed, _ := sameEntry.allowAt(func() time.Time { return afterConfiguredTTL }); allowed {
		t.Fatal("same key received a fresh token before full refill")
	}
	if _, _, ok := p.entryFor("new", afterConfiguredTTL); ok {
		t.Fatal("new key replaced slow bucket before full refill")
	}
}

func TestInFlightEntryIsNotReplacedAfterIdleTTL(t *testing.T) {
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 1_000_000_000_000,
		Burst:             2,
		IdleTTL:           time.Nanosecond,
		MaxClients:        1,
	})
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	p := &policy{
		config:  config,
		clients: make(map[string]*clientEntry),
	}

	first, _, ok := p.entryFor("client", time.Now())
	if !ok {
		t.Fatal("first entryFor() rejected")
	}
	time.Sleep(time.Millisecond)
	second, _, ok := p.entryFor("client", time.Now())
	if !ok {
		t.Fatal("second entryFor() rejected")
	}
	if first != second {
		t.Fatal("in-flight client bucket was replaced after IdleTTL")
	}

	if allowed, _ := first.allow(); !allowed {
		t.Fatal("first token was unexpectedly rejected")
	}
	if allowed, _ := second.allow(); !allowed {
		t.Fatal("second token was unexpectedly rejected")
	}
	if got := first.active.Load(); got != 0 {
		t.Fatalf("active leases = %d, want 0", got)
	}
}

func TestQuotaRetryHeadersFollowTokenSchedule(t *testing.T) {
	gate := mustGate(t, Config{RequestsPerSecond: 0.25, Burst: 1})
	handler := gate(okHandler())

	if got := serve(t, handler, "192.0.2.100:1000", "").Code; got != http.StatusOK {
		t.Fatalf("first status = %d, want %d", got, http.StatusOK)
	}
	rejected := serve(t, handler, "192.0.2.100:2000", "")
	if rejected.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", rejected.Code, http.StatusTooManyRequests)
	}
	seconds := assertRetryHeaders(t, rejected)
	if seconds < 3 || seconds > 4 {
		t.Fatalf("Retry-After = %d seconds, want 3-4 from the four-second token schedule", seconds)
	}
}

func TestRejectedReservationDoesNotConsumeFutureToken(t *testing.T) {
	start := time.Now()
	entry := &clientEntry{bucket: rate.NewLimiter(2, 1)}
	entry.active.Store(3)

	if allowed, _ := entry.allowAt(func() time.Time { return start }); !allowed {
		t.Fatal("initial token was rejected")
	}
	allowed, retry := entry.allowAt(func() time.Time { return start })
	if allowed {
		t.Fatal("immediate second token was allowed")
	}
	if retry != 500*time.Millisecond {
		t.Fatalf("retry delay = %v, want exact 500ms schedule", retry)
	}
	if allowed, _ := entry.allowAt(func() time.Time { return start.Add(retry) }); !allowed {
		t.Fatal("token was unavailable after the reported retry schedule")
	}
}

func TestAllowCapturesDecisionTimeWhileHoldingClientLock(t *testing.T) {
	initial := time.Unix(1_700_000_000, 0)
	decision := initial.Add(5 * time.Second)
	entry := &clientEntry{bucket: rate.NewLimiter(1, 1)}
	entry.lastSeen.Store(initial.UnixNano())
	entry.active.Store(1)

	allowed, _ := entry.allowAt(func() time.Time {
		if entry.mu.TryLock() {
			entry.mu.Unlock()
			t.Fatal("decision clock ran before acquiring the client token lock")
		}
		if got := entry.active.Load(); got != 1 {
			t.Fatalf("active leases while making decision = %d, want 1", got)
		}
		return decision
	})
	if !allowed {
		t.Fatal("token was unexpectedly rejected")
	}
	if got := entry.lastSeen.Load(); got != decision.UnixNano() {
		t.Fatalf("lastSeen = %d, want decision time %d", got, decision.UnixNano())
	}
	if got := entry.active.Load(); got != 0 {
		t.Fatalf("active leases after decision = %d, want 0", got)
	}
}

func TestConcurrentQuotaDecisionsAreAtomic(t *testing.T) {
	const (
		requests = 128
		burst    = 16
	)

	gate := mustGate(t, Config{RequestsPerSecond: 0.000001, Burst: burst})
	handler := gate(okHandler())
	var allowed atomic.Int64
	var rejected atomic.Int64
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(requests)
	for i := range requests {
		go func(port int) {
			defer workers.Done()
			<-start
			status := serve(t, handler, "192.0.2.110:"+strconv.Itoa(10_000+port), "").Code
			switch status {
			case http.StatusOK:
				allowed.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				t.Errorf("status = %d, want 200 or 429", status)
			}
		}(i)
	}
	close(start)
	workers.Wait()

	if got := allowed.Load(); got != burst {
		t.Fatalf("allowed = %d, want exact burst %d", got, burst)
	}
	if got := rejected.Load(); got != requests-burst {
		t.Fatalf("rejected = %d, want %d", got, requests-burst)
	}
}

func TestConcurrentCapacityRemainsBounded(t *testing.T) {
	const (
		requests   = 96
		maxClients = 12
	)

	gate := mustGate(t, Config{
		RequestsPerSecond: 1,
		Burst:             1,
		IdleTTL:           time.Minute,
		MaxClients:        maxClients,
	})
	handler := gate(okHandler())
	var allowed atomic.Int64
	var rejected atomic.Int64
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(requests)
	for i := range requests {
		go func(client int) {
			defer workers.Done()
			<-start
			remoteAddr := "192.0.2." + strconv.Itoa(client+1) + ":1234"
			status := serve(t, handler, remoteAddr, "").Code
			switch status {
			case http.StatusOK:
				allowed.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				t.Errorf("status = %d, want 200 or 429", status)
			}
		}(i)
	}
	close(start)
	workers.Wait()

	if got := allowed.Load(); got != maxClients {
		t.Fatalf("allowed clients = %d, want capacity %d", got, maxClients)
	}
	if got := rejected.Load(); got != requests-maxClients {
		t.Fatalf("rejected clients = %d, want %d", got, requests-maxClients)
	}
}

func mustGate(t *testing.T, config Config) func(http.Handler) http.Handler {
	t.Helper()
	gate, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return gate
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func serve(t *testing.T, handler http.Handler, remoteAddr, forwardedFor string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertRetryHeaders(t *testing.T, recorder *httptest.ResponseRecorder) int64 {
	t.Helper()
	retryAfter := recorder.Header().Get("Retry-After")
	reset := recorder.Header().Get("RateLimit-Reset")
	if retryAfter == "" || reset == "" {
		t.Fatalf("retry headers = Retry-After %q, RateLimit-Reset %q; both must be set", retryAfter, reset)
	}
	if retryAfter != reset {
		t.Fatalf("Retry-After = %q, RateLimit-Reset = %q; want matching schedule", retryAfter, reset)
	}
	seconds, err := strconv.ParseInt(retryAfter, 10, 64)
	if err != nil || seconds <= 0 {
		t.Fatalf("Retry-After = %q, want positive integer seconds", retryAfter)
	}
	return seconds
}
