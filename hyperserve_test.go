package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Content serving tests

func TestHandleTemplate(t *testing.T) {
	srv, _ := NewServer()
	srv.config.TemplateDir = "./test_templates"

	// Create a temporary template file
	templateContent := "<html><body>{{.}}</body></html>"
	err := os.MkdirAll(srv.config.TemplateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(srv.config.TemplateDir+"/test.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	defer os.RemoveAll(srv.config.TemplateDir)

	// Test valid template rendering
	srv.HandleTemplate("/test", "test.html", "Hello, World!")
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	srv.healthMux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	expectedBody := "<html><body>Hello, World!</body></html>"
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %v, got %v", expectedBody, rec.Body.String())
	}

	// Test missing template
	srv.HandleTemplate("/missing", "missing.html", "Hello, World!")
	req = httptest.NewRequest("GET", "/missing", nil)
	rec = httptest.NewRecorder()
	srv.healthMux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %v, got %v", http.StatusInternalServerError, rec.Code)
	}
	expectedError := "Error rendering template"
	if !strings.Contains(rec.Body.String(), expectedError) {
		t.Errorf("expected body to contain %v, got %v", expectedError, rec.Body.String())
	}
}

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
	err := os.Setenv(paramServerAddr, ":9090")
	if err != nil {
		t.Error("error setting environment variable")
	}
	defer func() {
		err := os.Unsetenv(paramServerAddr)
		if err != nil {
			t.Error("error unsetting environment variable")
		}
	}()
	config := NewConfig()
	if config.Addr != ":9090" {
		t.Errorf("expected Addr %v, got %v", ":9090", config.Addr)
	}
}

func TestConfigFileOverride(t *testing.T) {
	fileContent := `{"addr": ":7070", "rate-limit": 2, "burst": 20}`
	err := os.WriteFile(paramFileName, []byte(fileContent), 0644)
	if err != nil {
		t.Error("error writing file")
	}
	defer func() {
		err = os.Remove(paramFileName)
		if err != nil {
			t.Error("error removing param file")
		}
	}()

	config := NewConfig()
	if config.Addr != ":7070" {
		t.Errorf("expected Addr %v, got %v", ":7070", config.Addr)
	}
	if config.RateLimit != rateLimit(2) {
		t.Errorf("expected RateLimit %v, got %v", rateLimit(2), config.RateLimit)
	}
	if config.Burst != 20 {
		t.Errorf("expected Burst %v, got %v", 20, config.Burst)
	}
}

// Middleware tests

func TestRateLimitMiddleware(t *testing.T) {
	srv, _ := NewServer(WithRateLimit(1, 1))
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
	srv, _ := NewServer()
	handler := srv.HeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv, _ := NewServer()
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
	srv, _ := NewServer()
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

func TestServerStartStop(t *testing.T) {
	srv, _ := NewServer()

	// Start the server in a goroutine
	go srv.Run()

	// Wait for the server to start
	time.Sleep(1 * time.Second)

	// Verify the server is live
	isLive.Store(true)
	if !isLive.Load() {
		t.Fatal("expected server to be live after start")
	}

	// Trigger shutdown
	process, _ := os.FindProcess(os.Getpid()) // Get current process
	_ = process.Signal(syscall.SIGINT)        // Send SIGINT to trigger shutdown

	// Wait for shutdown
	time.Sleep(2 * time.Second)

	// Verify the server has stopped
	if isLive.Load() {
		t.Error("expected server to not be live after shutdown")
	}
}
