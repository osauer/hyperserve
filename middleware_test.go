package hyperserve

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestUsePrefixRejectsPathsThatCannotMatchURLPath(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{
		"api",
		"/api?tenant=x",
		"/api#fragment",
		"/api/{id}",
		"/a/../admin",
		"/api//admin",
		"/api%2Fadmin",
	} {
		prefix := prefix
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Fatalf("UsePrefix(%q) did not panic", prefix)
				}
			}()
			srv, err := New()
			if err != nil {
				t.Fatal(err)
			}
			srv.UsePrefix(prefix, func(next http.Handler) http.Handler { return next })
		})
	}
}

func TestUsePrefixAcceptsIntentionalUniversalRootAndTrailingSlash(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"", "/", "/api", "/api/"} {
		srv, err := New()
		if err != nil {
			t.Fatal(err)
		}
		srv.UsePrefix(prefix, func(next http.Handler) http.Handler { return next })
	}
}

func TestMetricsMiddlewareIncrementsTotalRequests(t *testing.T) {
	t.Parallel()
	srv, _ := New()
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

func TestHeadersMiddlewareOmitsServerHeaderByDefault(t *testing.T) {
	t.Parallel()
	options := Options{}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	serverHeader := rec.Header().Get("Server")
	if serverHeader != "" {
		t.Errorf("expected no Server header by default, got %v", serverHeader)
	}

	// Other security headers should still be present
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options header to be set")
	}
}

func TestHeadersMiddlewareWithServerHeader(t *testing.T) {
	t.Parallel()
	options := Options{
		ServerHeader: "example-service",
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	serverHeader := rec.Header().Get("Server")
	if serverHeader != "example-service" {
		t.Errorf("expected configured Server header, got %v", serverHeader)
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
			options := Options{EnableTLS: tc.enableTLS}
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
	options := Options{
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
	options := Options{
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
	options := Options{
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
	options := Options{}
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
func countingMW(hits *int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*hits++
			next.ServeHTTP(w, r)
		})
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
			srv, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			var hits int
			srv.UsePrefix("/api", countingMW(&hits))
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
// callers (including mcp/builtin tests) use `srv.UsePrefix("", ...)`
// as a synonym for "apply to every route". Before the boundary fix, that
// worked by accident (HasPrefix accepts an empty key). After the fix, an
// empty key needs an explicit short-circuit — otherwise the next-char
// boundary check indexes a zero-length string and panics. This test
// guards against regression of that short-circuit.
func TestMiddlewareEmptyKeyMatchesAll(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/", "/api", "/api2/foo", "/deep/nested/x"} {
		t.Run(path, func(t *testing.T) {
			srv, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			var hits int
			srv.UsePrefix("", countingMW(&hits))
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
			srv, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			var hits int
			srv.UsePrefix("/", countingMW(&hits))
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

func TestUsePrefixRejectsMissingLeadingSlash(t *testing.T) {
	t.Parallel()

	srv, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("UsePrefix accepted a nonempty prefix without a leading slash")
		}
	}()
	srv.UsePrefix("api", countingMW(new(int)))
}

func TestMiddlewarePlanCompilesOnceAndPreservesOrder(t *testing.T) {
	t.Parallel()

	registry := newMiddlewareRegistry(nil)
	constructs := make(map[string]int)
	var trace []string
	record := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			constructs[name]++
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace = append(trace, "enter "+name)
				next.ServeHTTP(w, r)
				trace = append(trace, "exit "+name)
			})
		}
	}

	registry.Add(globalMiddlewareRoute, MiddlewareStack{record("global")})
	registry.Add("/", MiddlewareStack{record("root")})
	registry.Add("/api", MiddlewareStack{record("api")})
	registry.Add("/api/admin", MiddlewareStack{record("admin")})

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		trace = append(trace, "handler")
		w.WriteHeader(http.StatusNoContent)
	})
	handler := registry.applyToMux(mux)

	for range 2 {
		handler.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/api/admin/resource", nil),
		)
	}

	for _, name := range []string{"global", "root", "api", "admin"} {
		if constructs[name] != 1 {
			t.Errorf("%s middleware constructed %d times, want 1", name, constructs[name])
		}
	}

	wantOneRequest := []string{
		"enter global",
		"enter root",
		"enter api",
		"enter admin",
		"handler",
		"exit admin",
		"exit api",
		"exit root",
		"exit global",
	}
	want := append(append([]string(nil), wantOneRequest...), wantOneRequest...)
	if strings.Join(trace, "|") != strings.Join(want, "|") {
		t.Fatalf("middleware trace = %v, want %v", trace, want)
	}
}

func TestMiddlewareRegisteredAfterHandlerConstructionBeforeServing(t *testing.T) {
	t.Parallel()

	registry := newMiddlewareRegistry(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := registry.applyToMux(mux)

	var hits int
	registry.Add("/api", MiddlewareStack{countingMW(&hits)})
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/resource", nil),
	)

	if hits != 1 {
		t.Fatalf("middleware registered before serving fired %d times, want 1", hits)
	}
}

func TestMiddlewarePlanCompilationSharedByConcurrentFirstRequests(t *testing.T) {
	t.Parallel()

	registry := newMiddlewareRegistry(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := registry.applyToMux(mux)

	var constructs atomic.Int64
	var hits atomic.Int64
	registry.Add("/api", MiddlewareStack{func(next http.Handler) http.Handler {
		constructs.Add(1)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			next.ServeHTTP(w, r)
		})
	}})

	const requests = 32
	var wg sync.WaitGroup
	for range requests {
		wg.Go(func() {
			handler.ServeHTTP(
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodGet, "/api/resource", nil),
			)
		})
	}
	wg.Wait()

	if got := constructs.Load(); got != 1 {
		t.Fatalf("middleware constructed %d times, want 1", got)
	}
	if got := hits.Load(); got != requests {
		t.Fatalf("middleware handled %d requests, want %d", got, requests)
	}
}

func TestMiddlewareRegistrationAfterServingPanics(t *testing.T) {
	t.Parallel()

	registry := newMiddlewareRegistry(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := registry.applyToMux(mux)
	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/", nil),
	)

	defer func() {
		got := recover()
		if got != "hyperserve: middleware registered after serving started" {
			t.Fatalf("panic = %v, want clear configuration-freeze error", got)
		}
	}()
	registry.Add(globalMiddlewareRoute, MiddlewareStack{countingMW(new(int))})
}

func TestRequestLoggerDisabledPassesThroughResponseWriter(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	underlying := httptest.NewRecorder()
	var received http.ResponseWriter
	handler := requestLoggerMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received = w
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(underlying, httptest.NewRequest(http.MethodGet, "/", nil))

	if received != underlying {
		t.Fatalf("disabled request logger passed %T, want original %T", received, underlying)
	}
}
