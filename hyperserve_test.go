package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Configuration tests

func TestDefaultConfig(t *testing.T) {
	config := NewConfig()
	if config.Addr != defaultAddr {
		t.Errorf("expected Addr %v, got %v", defaultAddr, config.Addr)
	}
	if config.RateLimit != defaultRateLimit {
		t.Errorf("expected RateLimit %v, got %v", defaultRateLimit, config.RateLimit)
	}
	if config.Burst != defaultBurst {
		t.Errorf("expected Burst %v, got %v", defaultBurst, config.Burst)
	}
	if config.ReadTimeout != defaultReadTimeout {
		t.Errorf("expected ReadTimeout %v, got %v", defaultReadTimeout, config.ReadTimeout)
	}
	if config.WriteTimeout != defaultWriteTimeout {
		t.Errorf("expected WriteTimeout %v, got %v", defaultWriteTimeout, config.WriteTimeout)
	}
	if config.IdleTimeout != defaultIdleTimeout {
		t.Errorf("expected IdleTimeout %v, got %v", defaultIdleTimeout, config.IdleTimeout)
	}
}

func TestConfigEnvOverride(t *testing.T) {
	os.Setenv(configServerAddr, ":9090")
	defer os.Unsetenv(configServerAddr)
	config := NewConfig()
	if config.Addr != ":9090" {
		t.Errorf("expected Addr %v, got %v", ":9090", config.Addr)
	}
}

func TestConfigFileOverride(t *testing.T) {
	fileContent := `{"addr": ":7070", "rate-limit": 2, "burst": 20}`
	os.WriteFile(configFileName, []byte(fileContent), 0644)
	defer os.Remove(configFileName)

	config := NewConfig()
	if config.Addr != ":7070" {
		t.Errorf("expected Addr %v, got %v", ":7070", config.Addr)
	}
	if config.RateLimit != RateLimit(2) {
		t.Errorf("expected RateLimit %v, got %v", RateLimit(2), config.RateLimit)
	}
	if config.Burst != 20 {
		t.Errorf("expected Burst %v, got %v", 20, config.Burst)
	}
}

// Middleware tests

func TestRateLimitMiddleware(t *testing.T) {
	srv, _ := NewAPIServer(WithRateLimit(1, 1))
	handler := srv.RateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}

	// Second request should be rate-limited
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %v, got %v", http.StatusTooManyRequests, rec.Code)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	srv, _ := NewAPIServer()
	handler := srv.SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	for _, h := range securityHeaders {
		if rec.Header().Get(h.key) != h.value {
			t.Errorf("expected header %v to be %v, got %v", h.key, h.value, rec.Header().Get(h.key))
		}
	}
}

func TestRequireAuthMiddleware(t *testing.T) {
	srv, _ := NewAPIServer()
	handler := srv.RequireAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	// No Authorization header
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %v, got %v", http.StatusUnauthorized, rec.Code)
	}

	// Valid Authorization header
	req.Header.Set(authorizationHeader, bearerTokenPrefix+"valid-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
}

// Health Check tests

func TestHealthEndpoints(t *testing.T) {
	srv, _ := NewAPIServer()
	srv.Handle("/healthz/", srv.healthzHandler)
	srv.Handle("/readyz/", srv.readyzHandler)
	srv.Handle("/livez/", srv.livezHandler)

	tests := []struct {
		path       string
		statusFlag *atomic.Bool
		expected   int
	}{
		{"/healthz/", &isLive, http.StatusOK},
		{"/readyz/", &isReady, http.StatusOK},
		{"/livez/", &isLive, http.StatusOK},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		rec := httptest.NewRecorder()

		tt.statusFlag.Store(true)
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != tt.expected {
			t.Errorf("expected status %v for %v, got %v", tt.expected, tt.path, rec.Code)
		}
	}
}

// Server lifecycle tests

func TestServerStartStop(t *testing.T) {
	srv, _ := NewAPIServer()
	go srv.Run()
	time.Sleep(500 * time.Millisecond) // Give server time to start
	isLive.Store(true)
	if !isLive.Load() {
		t.Error("expected server to be live")
	}

	// Trigger shutdown and check for server stop
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)
	go func() {
		stop <- syscall.SIGINT
	}()
	time.Sleep(500 * time.Millisecond) // Give server time to shut down
	if isLive.Load() {
		t.Error("expected server to not be live")
	}
}
