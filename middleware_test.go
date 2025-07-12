package hyperserve

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
