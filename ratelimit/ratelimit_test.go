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
	if config.idleTTL != 10*time.Minute {
		t.Fatalf("IdleTTL default = %v, want 10m", config.idleTTL)
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
		RequestsPerSecond: 0.001,
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
		t.Fatalf("capacity Retry-After = %d seconds, want 3-4 from the earliest idle expiry", seconds)
	}

	if got := serve(t, handler, "192.0.2.80:2000", "").Code; got != http.StatusOK {
		t.Fatalf("existing key at capacity status = %d, want %d", got, http.StatusOK)
	}
}

func TestExpiredEntriesArePrunedAndQuotaResets(t *testing.T) {
	gate := mustGate(t, Config{
		RequestsPerSecond: 0.001,
		Burst:             1,
		IdleTTL:           15 * time.Millisecond,
		MaxClients:        1,
	})
	handler := gate(okHandler())

	if got := serve(t, handler, "192.0.2.90:1000", "").Code; got != http.StatusOK {
		t.Fatalf("first status = %d, want %d", got, http.StatusOK)
	}
	if got := serve(t, handler, "192.0.2.90:2000", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("pre-expiry status = %d, want %d", got, http.StatusTooManyRequests)
	}

	time.Sleep(40 * time.Millisecond)
	if got := serve(t, handler, "192.0.2.90:3000", "").Code; got != http.StatusOK {
		t.Fatalf("same key after expiry status = %d, want %d", got, http.StatusOK)
	}

	time.Sleep(40 * time.Millisecond)
	if got := serve(t, handler, "192.0.2.91:1000", "").Code; got != http.StatusOK {
		t.Fatalf("new key after expiry status = %d, want %d", got, http.StatusOK)
	}
}

func TestInFlightEntryIsNotReplacedAfterIdleTTL(t *testing.T) {
	config, err := normalizeConfig(Config{
		RequestsPerSecond: 1,
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

	if allowed, _ := first.allow(time.Now()); !allowed {
		t.Fatal("first token was unexpectedly rejected")
	}
	if allowed, _ := second.allow(time.Now()); !allowed {
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

	if allowed, _ := entry.allow(start); !allowed {
		t.Fatal("initial token was rejected")
	}
	allowed, retry := entry.allow(start)
	if allowed {
		t.Fatal("immediate second token was allowed")
	}
	if retry != 500*time.Millisecond {
		t.Fatalf("retry delay = %v, want exact 500ms schedule", retry)
	}
	if allowed, _ := entry.allow(start.Add(retry)); !allowed {
		t.Fatal("token was unavailable after the reported retry schedule")
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
	for i := 0; i < requests; i++ {
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
	for i := 0; i < requests; i++ {
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
