package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecureWebMiddleware tests the SecureWeb middleware stack
func TestSecureWebMiddleware(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Apply SecureWeb middleware
	srv.UsePrefix("/secure", SecureWeb(srv.options))

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

// TestSecureWebAllowsRepeatedRequests verifies that SecureWeb only installs
// security headers; traffic policy belongs to separately attached middleware.
func TestSecureWebAllowsRepeatedRequests(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Apply the regular SecureWeb middleware.
	srv.UsePrefix("/secure", SecureWeb(srv.options))

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

	// The security stack must allow repeated requests.
	for i := range 50 {
		req := httptest.NewRequest(http.MethodGet, "/secure/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rec.Code)
		}
	}
}
