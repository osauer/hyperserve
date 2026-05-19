package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestMetricsMiddlewareIncrementsTotalRequests(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	handler := MetricsMiddleware(srv)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	if srv.totalRequests.Load() != 1 {
		t.Errorf("expected totalRequests to be 1, got %v", srv.totalRequests.Load())
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		AuthTokenValidatorFunc: func(token string) (bool, error) {
			return token == "valid-token", nil
		},
	}
	handler := AuthMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		AuthTokenValidatorFunc: func(token string) (bool, error) {
			return false, nil
		},
	}
	handler := AuthMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %v, got %v", http.StatusUnauthorized, rec.Code)
	}
}

func TestRecoveryMiddlewareRecoversFromPanic(t *testing.T) {
	t.Parallel()
	handler := RecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %v, got %v", http.StatusInternalServerError, rec.Code)
	}
}

func TestRateLimitMiddlewareAllowsRequest(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	srv.Options.RateLimit = rate.Every(time.Second)
	srv.Options.Burst = 1
	handler := RateLimitMiddleware(srv)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
}

func TestRateLimitMiddlewareBlocksRequest(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	srv.Options.RateLimit = rate.Every(time.Second)
	srv.Options.Burst = 1
	handler := RateLimitMiddleware(srv)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.2:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	// Second request should be blocked
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %v, got %v", http.StatusTooManyRequests, rec2.Code)
	}
}

// Test Hardened Mode functionality
func TestHeadersMiddlewareWithHardenedMode(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		HardenedMode: true,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// In hardened mode, Server header should not be set
	serverHeader := rec.Header().Get("Server")
	if serverHeader != "" {
		t.Errorf("expected no Server header in hardened mode, got %v", serverHeader)
	}

	// Other security headers should still be present
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options header to be set")
	}
}

func TestHeadersMiddlewareWithoutHardenedMode(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		HardenedMode: false,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// In normal mode, Server header should be set
	serverHeader := rec.Header().Get("Server")
	if serverHeader != "hyperserve" {
		t.Errorf("expected Server header to be 'hyperserve', got %v", serverHeader)
	}
}

// TestHSTSOnlyOverTLS pins the contract: HSTS is set when EnableTLS is true,
// and is *not* set on plaintext responses. Sending HSTS over HTTP is at best
// no-op and (with `preload` ahead of a reverse-proxy terminator) actively
// harmful, so the empty-header case is part of the contract, not a gap.
func TestHSTSOnlyOverTLS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		enableTLS  bool
		wantHeader string // empty == header must be absent
	}{
		{
			name:       "plaintext omits HSTS",
			enableTLS:  false,
			wantHeader: "",
		},
		{
			name:       "tls sets two-year preload HSTS",
			enableTLS:  true,
			wantHeader: "max-age=63072000; includeSubDomains; preload",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := &ServerOptions{EnableTLS: tc.enableTLS}
			handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("Strict-Transport-Security")
			if got != tc.wantHeader {
				t.Errorf("HSTS header = %q, want %q", got, tc.wantHeader)
			}
		})
	}
}

func TestCSPGenerationWithoutWebWorkerSupport(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWebWorkerSupport: false,
	}

	csp := generateCSP(options)

	// Should not contain blob: URLs
	if strings.Contains(csp, "blob:") {
		t.Errorf("expected CSP to not contain blob: URLs when WebWorker support is disabled")
	}

	// Should contain basic directives
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("expected CSP to contain default-src 'self'")
	}

	// Should contain child-src without blob:
	if !strings.Contains(csp, "child-src 'self'") {
		t.Errorf("expected CSP to contain child-src 'self'")
	}

	// Should not contain worker-src directive (will fall back to child-src)
	if strings.Contains(csp, "worker-src") {
		t.Errorf("expected CSP to not contain worker-src directive when WebWorker support is disabled")
	}
}

func TestCSPGenerationWithWebWorkerSupport(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWebWorkerSupport: true,
	}

	csp := generateCSP(options)

	// Should contain blob: URLs for workers
	if !strings.Contains(csp, "worker-src 'self' blob:") {
		t.Errorf("expected CSP to contain worker-src 'self' blob: when WebWorker support is enabled")
	}

	// Should contain blob: URLs for child-src
	if !strings.Contains(csp, "child-src 'self' blob:") {
		t.Errorf("expected CSP to contain child-src 'self' blob: when WebWorker support is enabled")
	}

	// Should contain basic directives
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("expected CSP to contain default-src 'self'")
	}
}

func TestHeadersMiddlewareCSPWebWorkerSupport(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWebWorkerSupport: true,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header to be set")
	}

	// Should contain blob: URLs for workers
	if !strings.Contains(csp, "worker-src 'self' blob:") {
		t.Errorf("expected CSP to contain worker-src 'self' blob: when WebWorker support is enabled")
	}

	// Should contain blob: URLs for child-src
	if !strings.Contains(csp, "child-src 'self' blob:") {
		t.Errorf("expected CSP to contain child-src 'self' blob: when WebWorker support is enabled")
	}
}

func TestHeadersMiddlewarePermissionsPolicyFixed(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	permissionsPolicy := rec.Header().Get("Permissions-Policy")
	if permissionsPolicy == "" {
		t.Error("expected Permissions-Policy header to be set")
	}

	// Should not contain the invalid 'speaker' directive
	if strings.Contains(permissionsPolicy, "speaker") {
		t.Errorf("expected Permissions-Policy to not contain invalid 'speaker' directive")
	}

	// Should contain valid directives
	if !strings.Contains(permissionsPolicy, "geolocation=()") {
		t.Errorf("expected Permissions-Policy to contain geolocation=()")
	}
}

// countingMW returns a middleware that increments hits whenever a request
// flows through it. Combined with a counter map keyed by route, this is the
// minimal probe for "did the prefix match the request path".
func countingMW(hits *int) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*hits++
			next.ServeHTTP(w, r)
		}
	}
}

// TestMiddlewarePathPrefixBoundary verifies that a middleware registered at
// "/api" fires for "/api", "/api/", and "/api/foo" — but NOT for "/api2/foo"
// or "/apifoo". The pre-v0.34.1 implementation used `strings.HasPrefix` with
// no path boundary check, so "/api" matched "/api2/foo". Regression test.
func TestMiddlewarePathPrefixBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path     string
		wantHits int // 1 = mw should fire for /api; 0 = must not fire
	}{
		{"/api", 1},       // exact match
		{"/api/", 1},      // trailing slash
		{"/api/foo", 1},   // deeper path
		{"/api/v2/x", 1},  // even deeper
		{"/api2/foo", 0},  // share prefix but different route — the bug
		{"/apifoo", 0},    // no separator at all
		{"/apiserver", 0}, // common gotcha
		{"/", 0},          // unrelated
		{"/other", 0},     // unrelated
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			srv, err := NewServer()
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			var hits int
			srv.AddMiddleware("/api", countingMW(&hits))
			srv.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			srv.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			srv.HandleFunc("/api2/foo", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			srv.HandleFunc("/apifoo", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			srv.HandleFunc("/apiserver", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			srv.HandleFunc("/other", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			handler := srv.middleware.applyToMux(srv.mux)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if hits != tc.wantHits {
				t.Errorf("path %q: middleware fired %d times, want %d", tc.path, hits, tc.wantHits)
			}
		})
	}
}

// TestMiddlewareEmptyKeyMatchesAll pins the legacy "" key behaviour: some
// callers (including pkg/mcp/builtin tests) use `srv.AddMiddleware("", ...)`
// as a synonym for "apply to every route". Before the boundary fix, that
// worked by accident (HasPrefix accepts an empty key). After the fix, an
// empty key needs an explicit short-circuit — otherwise the next-char
// boundary check indexes a zero-length string and panics. This test
// guards against regression of that short-circuit.
func TestMiddlewareEmptyKeyMatchesAll(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/", "/api", "/api2/foo", "/deep/nested/x"} {
		t.Run(path, func(t *testing.T) {
			srv, err := NewServer()
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			var hits int
			srv.AddMiddleware("", countingMW(&hits))
			srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := srv.middleware.applyToMux(srv.mux)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if hits != 1 {
				t.Errorf("path %q: empty-key middleware fired %d times, want 1", path, hits)
			}
		})
	}
}

// TestMiddlewareRootPrefixMatches verifies the documented "/" key still
// fires for every path — the only legitimate prefix-without-boundary case.
func TestMiddlewareRootPrefixMatches(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/", "/api", "/api2/foo", "/anything"} {
		t.Run(path, func(t *testing.T) {
			srv, err := NewServer()
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			var hits int
			srv.AddMiddleware("/", countingMW(&hits))
			srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := srv.middleware.applyToMux(srv.mux)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if hits != 1 {
				t.Errorf("path %q: root middleware fired %d times, want 1", path, hits)
			}
		})
	}
}
