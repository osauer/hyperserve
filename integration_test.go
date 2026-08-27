package hyperserve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Integration tests for end-to-end server functionality

func TestServerStartStopIntegration(t *testing.T) {
	t.Parallel()
	srv, err := New(WithAddr(":0")) // Use port 0 for auto-assignment
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Disable health server for this test to avoid port conflicts
	srv.options.RunHealthServer = false

	// Channel to receive server run result
	serverResult := make(chan error, 1)

	// Test server startup and shutdown
	go func() {
		err := srv.Run(context.Background())
		// The server should exit with ErrServerClosed when stopped gracefully
		if err != nil && err != http.ErrServerClosed {
			serverResult <- fmt.Errorf("server run failed: %v", err)
		} else {
			serverResult <- nil
		}
	}()

	// Wait for server to be fully initialized (isRunning=true means httpServer is ready)
	for !srv.isRunning.Load() {
		time.Sleep(1 * time.Millisecond)
	}

	// Stop the server
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Errorf("failed to stop server: %v", err)
	}

	// Check server run result
	if err := <-serverResult; err != nil {
		t.Error(err)
	}
}

func TestMiddlewareStackIntegration(t *testing.T) {
	t.Parallel()
	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Add a test endpoint
	srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Apply middleware to the mux to create the handler
	handler := srv.middleware.applyToMux(srv.mux)

	// Test that default middleware stack is applied
	req, _ := http.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

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
	// Create server without health server to avoid port conflicts
	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Disable automatic health server and set unique port
	srv.options.RunHealthServer = false
	srv.options.HealthAddr = ":0" // Let OS assign port

	// Initialize the health server manually to set up the endpoints
	if err := srv.initHealthServer(); err != nil {
		t.Fatalf("failed to initialize health server: %v", err)
	}

	// The server should be ready (created) and running (health server started)
	// For testing purposes, manually set the server as running since we're not calling Run()
	srv.isRunning.Store(true)

	// Test health endpoints on the health server mux
	healthEndpoints := []string{"/healthz/", "/readyz/", "/livez/"} // Note: handlers use trailing slash

	for _, endpoint := range healthEndpoints {
		req, _ := http.NewRequest("GET", endpoint, nil)
		rec := httptest.NewRecorder()
		srv.healthMux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("health endpoint %s returned status %v, expected %v", endpoint, rec.Code, http.StatusOK)
			// Log the actual response for debugging
			t.Logf("Response body: %s", rec.Body.String())
		}
	}

	// Cleanup health server
	if srv.healthServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		srv.healthServer.Shutdown(ctx)
	}
}

func TestTemplateRenderingIntegration(t *testing.T) {
	t.Parallel()

	// Create unique template directory
	templateDir := fmt.Sprintf("./test_integration_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create template file BEFORE creating the server
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("failed to create template directory: %v", err)
	}

	templateContent := `<html><body><h1>{{.title}}</h1><p>{{.content}}</p></body></html>`
	err = os.WriteFile(templateDir+"/test.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}

	// Now create the server with the template directory already set up
	srv, err := New(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Templates will be parsed lazily when HandleFuncDynamic is called

	// Add template endpoint
	err = srv.HandleFuncDynamic("/template-test", "test.html", func(r *http.Request) any {
		return map[string]any{
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

func TestSecurityHeadersIntegration(t *testing.T) {
	t.Parallel()
	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Add security middleware to test endpoint
	srv.middleware.Add("/secure-test", MiddlewareStack{SecureWeb(srv.options)})
	srv.HandleFunc("/secure-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	})

	// Test that security headers are applied
	req, _ := http.NewRequest("GET", "/secure-test", nil)
	rec := httptest.NewRecorder()

	// Apply middleware manually for testing
	handler := HeadersMiddleware(srv.options)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
