package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"os"
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

// CORS Tests
func TestHeadersMiddlewareWithoutCORSOrigins(t *testing.T) {
	t.Parallel()
	options := &ServerOptions{
		CORSOrigins: []string{}, // Empty, should default to wildcard
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	corsHeader := rec.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != "*" {
		t.Errorf("expected wildcard CORS origin, got %s", corsHeader)
	}
}

func TestHeadersMiddlewareWithSingleCORSOrigin(t *testing.T) {
	t.Parallel()
	expectedOrigin := "https://example.com"
	options := &ServerOptions{
		CORSOrigins: []string{expectedOrigin},
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	corsHeader := rec.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != expectedOrigin {
		t.Errorf("expected CORS origin %s, got %s", expectedOrigin, corsHeader)
	}
}

func TestHeadersMiddlewareWithMultipleCORSOrigins(t *testing.T) {
	t.Parallel()
	origins := []string{"https://example.com", "https://app.example.com", "https://api.example.com"}
	expectedHeader := "https://example.com, https://app.example.com, https://api.example.com"
	options := &ServerOptions{
		CORSOrigins: origins,
	}
	handler := HeadersMiddleware(options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	corsHeader := rec.Header().Get("Access-Control-Allow-Origin")
	if corsHeader != expectedHeader {
		t.Errorf("expected CORS origins %s, got %s", expectedHeader, corsHeader)
	}
}

func TestWithCORSOriginsFunctionalOption(t *testing.T) {
	t.Parallel()
	origins := []string{"https://test1.com", "https://test2.com"}
	srv, err := NewServer(WithCORSOrigins(origins...))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	
	if len(srv.Options.CORSOrigins) != 2 {
		t.Errorf("expected 2 CORS origins, got %d", len(srv.Options.CORSOrigins))
	}
	
	for i, expected := range origins {
		if srv.Options.CORSOrigins[i] != expected {
			t.Errorf("expected origin %s at index %d, got %s", expected, i, srv.Options.CORSOrigins[i])
		}
	}
}

func TestCORSOriginsFromEnvironmentVariable(t *testing.T) {
	t.Parallel()
	// Set environment variable
	originsStr := "https://env1.com, https://env2.com,https://env3.com"
	os.Setenv("HS_CORS_ORIGINS", originsStr)
	defer os.Unsetenv("HS_CORS_ORIGINS")
	
	// Create new server options which will read from environment
	options := NewServerOptions()
	
	expectedOrigins := []string{"https://env1.com", "https://env2.com", "https://env3.com"}
	if len(options.CORSOrigins) != 3 {
		t.Errorf("expected 3 CORS origins from env var, got %d", len(options.CORSOrigins))
	}
	
	for i, expected := range expectedOrigins {
		if options.CORSOrigins[i] != expected {
			t.Errorf("expected origin %s at index %d, got %s", expected, i, options.CORSOrigins[i])
		}
	}
}
