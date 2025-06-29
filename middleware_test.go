package hyperserve

import (
	"net/http"
	"net/http/httptest"
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
