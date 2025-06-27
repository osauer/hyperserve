package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Integration tests for end-to-end server functionality

func TestServerStartStopIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(WithAddr(":0")) // Use port 0 for auto-assignment
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test server startup and shutdown
	go func() {
		err := srv.Run()
		if err != nil && !strings.Contains(err.Error(), "Server stopped") {
			t.Errorf("server run failed: %v", err)
		}
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	if err := srv.Stop(); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}
}

func TestMiddlewareStackIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Add a test endpoint
	srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Test that default middleware stack is applied
	req, _ := http.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}

	// Check that metrics were incremented
	if srv.totalRequests.Load() == 0 {
		t.Error("expected metrics middleware to increment request count")
	}
}

func TestHealthEndpointsIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test health endpoints
	healthEndpoints := []string{"/healthz", "/readyz", "/livez"}
	
	for _, endpoint := range healthEndpoints {
		req, _ := http.NewRequest("GET", endpoint, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("health endpoint %s returned status %v, expected %v", endpoint, rec.Code, http.StatusOK)
		}
	}
}

func TestTemplateRenderingIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create unique template directory
	templateDir := fmt.Sprintf("./test_integration_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	srv.Options.TemplateDir = templateDir
	defer os.RemoveAll(templateDir)

	// Create template file
	err = os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("failed to create template directory: %v", err)
	}

	templateContent := `<html><body><h1>{{.title}}</h1><p>{{.content}}</p></body></html>`
	err = os.WriteFile(templateDir+"/test.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}

	// Add template endpoint
	err = srv.HandleFuncDynamic("/template-test", "test.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"title":   "Integration Test",
			"content": "This is a template rendering test",
		}
	})
	if err != nil {
		t.Fatalf("failed to add template handler: %v", err)
	}

	// Test template rendering
	req, _ := http.NewRequest("GET", "/template-test", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("template endpoint returned status %v, expected %v", rec.Code, http.StatusOK)
	}

	expectedContent := "<html><body><h1>Integration Test</h1><p>This is a template rendering test</p></body></html>"
	if rec.Body.String() != expectedContent {
		t.Errorf("template output mismatch.\nExpected: %s\nGot: %s", expectedContent, rec.Body.String())
	}
}

func TestRateLimitingIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(WithRateLimit(1, 1)) // 1 request per second, burst of 1
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Add middleware to a test endpoint
	srv.middleware.Add("/rate-test", MiddlewareStack{RateLimitMiddleware(srv)})
	srv.HandleFunc("/rate-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// First request should succeed
	req, _ := http.NewRequest("GET", "/rate-test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	
	// Apply middleware manually for testing
	handler := RateLimitMiddleware(srv)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Errorf("first request should succeed, got status %v", rec1.Code)
	}

	// Second request should be rate limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate limited, got status %v", rec2.Code)
	}

	// Check for retry-after header
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header in rate limited response")
	}
}

func TestSecurityHeadersIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Add security middleware to test endpoint
	srv.middleware.Add("/secure-test", SecureWeb(srv.Options))
	srv.HandleFunc("/secure-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	})

	// Test that security headers are applied
	req, _ := http.NewRequest("GET", "/secure-test", nil)
	rec := httptest.NewRecorder()
	
	// Apply middleware manually for testing
	handler := HeadersMiddleware(srv.Options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	}))
	
	handler.ServeHTTP(rec, req)

	// Check for key security headers
	expectedHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
	}

	for _, header := range expectedHeaders {
		if rec.Header().Get(header) == "" {
			t.Errorf("expected security header %s to be set", header)
		}
	}
}

func TestCleanupOnServerStopIntegration(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Verify cleanup mechanisms are initialized
	if srv.cleanupTicker == nil {
		t.Error("cleanup ticker should be initialized")
	}
	if srv.cleanupDone == nil {
		t.Error("cleanup done channel should be initialized")
	}

	// Stop the server and verify cleanup
	err = srv.Stop()
	if err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}
}