package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestSecureWebMiddleware tests the SecureWeb middleware stack
func TestSecureWebMiddleware(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Apply SecureWeb middleware
	srv.AddMiddlewareStack("/secure", SecureWeb(srv.Options))

	// Create a test handler
	srv.HandleFunc("/secure/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Test security headers are applied
	req := httptest.NewRequest(http.MethodGet, "/secure/test", nil)
	rec := httptest.NewRecorder()
	
	// Use the middleware-wrapped handler
	handler := srv.middleware.applyToMux(srv.mux)
	handler.ServeHTTP(rec, req)

	// Check security headers
	expectedHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		actual := rec.Header().Get(header)
		if actual != expected {
			t.Errorf("Expected header %s to be %s, got %s", header, expected, actual)
		}
	}

	// Verify response is OK
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

// TestSecureWebWithoutRateLimit tests SecureWeb middleware without rate limiting
func TestSecureWebWithoutRateLimit(t *testing.T) {
	// Create server without rate limiting (RateLimit = 0)
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Apply regular SecureWeb middleware (no rate limiting)
	srv.AddMiddlewareStack("/secure", SecureWeb(srv.Options))

	// Create a test handler
	srv.HandleFunc("/secure/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Apply middleware
	handler := srv.middleware.applyToMux(srv.mux)
	
	// Test that security headers are applied
	req := httptest.NewRequest(http.MethodGet, "/secure/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	
	// Check security headers
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Expected X-Content-Type-Options header to be set")
	}
	
	// Verify no rate limiting - send multiple requests
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/secure/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		
		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rec.Code)
		}
	}
}

// TestRateLimitingUnderLoad tests rate limiting middleware under concurrent load
func TestRateLimitingUnderLoad(t *testing.T) {
	srv, err := NewServer(
		WithRateLimit(50, 100), // 50 req/s, burst 100
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Apply rate limiting middleware directly
	srv.AddMiddleware("/api", RateLimitMiddleware(srv))
	srv.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply middleware
	handler := srv.middleware.applyToMux(srv.mux)

	// Run concurrent requests
	var wg sync.WaitGroup
	successCount := 0
	rateLimitCount := 0
	var mu sync.Mutex

	numGoroutines := 10
	requestsPerGoroutine := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
				// Use same IP for all requests to trigger rate limiting
				req.RemoteAddr = "192.168.1.100:12345"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				mu.Lock()
				if rec.Code == http.StatusOK {
					successCount++
				} else if rec.Code == http.StatusTooManyRequests {
					rateLimitCount++
				}
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	totalRequests := numGoroutines * requestsPerGoroutine
	t.Logf("Total requests: %d, Success: %d, Rate limited: %d", 
		totalRequests, successCount, rateLimitCount)

	// We should have some successful requests
	if successCount == 0 {
		t.Error("Expected some successful requests")
	}
	
	// And some rate limited requests (since we're sending 200 requests quickly)
	if rateLimitCount == 0 {
		t.Error("Expected some rate limited requests")
	}
}