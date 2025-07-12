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

// Test CSP with no blob URLs enabled (default)
func TestCSPDefaultNoBlob(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: false,
		CSPChildSrcBlob:  false,
		CSPScriptSrcBlob: false,
		CSPMediaSrcBlob:  false,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	expectedCSP := "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; media-src 'self'; object-src 'none'; child-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	if csp != expectedCSP {
		t.Errorf("expected CSP %v, got %v", expectedCSP, csp)
	}
}

// Test CSP with worker-src blob enabled
func TestCSPWorkerSrcBlob(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: true,
		CSPChildSrcBlob:  false,
		CSPScriptSrcBlob: false,
		CSPMediaSrcBlob:  false,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "worker-src 'self' blob:") {
		t.Errorf("expected CSP to contain 'worker-src 'self' blob:', got %v", csp)
	}
}

// Test CSP with child-src blob enabled
func TestCSPChildSrcBlob(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: false,
		CSPChildSrcBlob:  true,
		CSPScriptSrcBlob: false,
		CSPMediaSrcBlob:  false,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "child-src 'self' blob:") {
		t.Errorf("expected CSP to contain 'child-src 'self' blob:', got %v", csp)
	}
}

// Test CSP with script-src blob enabled
func TestCSPScriptSrcBlob(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: false,
		CSPChildSrcBlob:  false,
		CSPScriptSrcBlob: true,
		CSPMediaSrcBlob:  false,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'unsafe-inline' blob:") {
		t.Errorf("expected CSP to contain 'script-src 'self' 'unsafe-inline' blob:', got %v", csp)
	}
}

// Test CSP with media-src blob enabled
func TestCSPMediaSrcBlob(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: false,
		CSPChildSrcBlob:  false,
		CSPScriptSrcBlob: false,
		CSPMediaSrcBlob:  true,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "media-src 'self' blob:") {
		t.Errorf("expected CSP to contain 'media-src 'self' blob:', got %v", csp)
	}
	if !strings.Contains(csp, "img-src 'self' data: blob:") {
		t.Errorf("expected CSP to contain 'img-src 'self' data: blob:', got %v", csp)
	}
}

// Test CSP with all blob options enabled
func TestCSPAllBlobEnabled(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CSPWorkerSrcBlob: true,
		CSPChildSrcBlob:  true,
		CSPScriptSrcBlob: true,
		CSPMediaSrcBlob:  true,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	csp := rec.Header().Get("Content-Security-Policy")
	expectedSubstrings := []string{
		"worker-src 'self' blob:",
		"child-src 'self' blob:",
		"script-src 'self' 'unsafe-inline' blob:",
		"media-src 'self' blob:",
		"img-src 'self' data: blob:",
	}
	
	for _, expected := range expectedSubstrings {
		if !strings.Contains(csp, expected) {
			t.Errorf("expected CSP to contain '%s', got %v", expected, csp)
		}
	}
}

// Test Permissions-Policy header doesn't contain 'speaker' directive
func TestPermissionsPolicyNoSpeaker(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	permissionsPolicy := rec.Header().Get("Permissions-Policy")
	if strings.Contains(permissionsPolicy, "speaker=") {
		t.Errorf("expected Permissions-Policy to not contain 'speaker=', got %v", permissionsPolicy)
	}
	
	// Verify other directives are still present
	expectedDirectives := []string{
		"geolocation=()",
		"microphone=()",
		"camera=()",
		"payment=()",
		"usb=()",
		"magnetometer=()",
		"gyroscope=()",
		"fullscreen=(self)",
	}
	
	for _, expected := range expectedDirectives {
		if !strings.Contains(permissionsPolicy, expected) {
			t.Errorf("expected Permissions-Policy to contain '%s', got %v", expected, permissionsPolicy)
		}
	}
}
