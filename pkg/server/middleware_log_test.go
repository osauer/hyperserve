package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMiddlewareLogBehavior ensures middleware registration logs only appear during setup, not per request
func TestMiddlewareLogBehavior(t *testing.T) {
	// Capture logs
	var logBuffer bytes.Buffer
	handler := slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	oldLogger := logger
	logger = slog.New(handler)
	defer func() { logger = oldLogger }()

	// Create server
	srv, err := NewServer(
		WithAddr(":0"),
		WithAuthTokenValidator(func(token string) (bool, error) {
			return true, nil // Accept all tokens for testing
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add some middleware
	srv.AddMiddleware("/api", RateLimitMiddleware(srv))
	srv.AddMiddleware("/api", AuthMiddleware(srv.Options))

	// Add a test handler
	srv.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Make multiple requests
	httpHandler := srv.middleware.applyToMux(srv.mux)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		httpHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i, rec.Code)
		}
	}

	// Check logs
	logs := logBuffer.String()

	// Count occurrences of expected log messages
	defaultMiddlewareCount := strings.Count(logs, "Default middleware registered")
	middlewareRegisteredCount := strings.Count(logs, "Middleware registered")

	// These should appear ONLY during setup
	if defaultMiddlewareCount != 1 {
		t.Errorf("Expected 'Default middleware registered' to appear exactly once, found %d times", defaultMiddlewareCount)
	}

	if middlewareRegisteredCount != 2 { // We registered 2 middleware
		t.Errorf("Expected 'Middleware registered' to appear exactly 2 times, found %d times", middlewareRegisteredCount)
	}

	// These should NOT appear at all (we removed them)
	prohibitedPatterns := []string{
		"MetricsMiddleware enabled",
		"RequestLoggerMiddleware enabled",
		"RecoveryMiddleware enabled",
		"RateLimitMiddleware enabled",
		"AuthMiddleware enabled",
		"HeadersMiddleware enabled",
		"ResponseTimeMiddleware enabled",
		"TraceMiddleware enabled",
		"ChaosMiddleware enabled",
		"trailingSlashMiddleware enabled",
	}

	for _, pattern := range prohibitedPatterns {
		count := strings.Count(logs, pattern)
		if count > 0 {
			t.Errorf("Pattern '%s' should not appear in logs, but found %d times", pattern, count)
			t.Logf("This indicates middleware is being recreated on each request instead of once during setup")
		}
	}
}

// TestMiddlewareOnlyLogsOncePerRoute ensures middleware logs only appear once per route registration
func TestMiddlewareOnlyLogsOncePerRoute(t *testing.T) {
	// Capture logs
	var logBuffer bytes.Buffer
	handler := slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	oldLogger := logger
	logger = slog.New(handler)
	defer func() { logger = oldLogger }()

	// Create server
	srv, err := NewServer(WithAddr(":0"))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register middleware stacks
	srv.AddMiddlewareStack("/", SecureWeb(srv.Options))
	srv.AddMiddlewareStack("/api", SecureAPI(srv))

	// Add handlers
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("home"))
	})
	srv.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	})

	// Make requests to different routes
	httpHandler := srv.middleware.applyToMux(srv.mux)

	// Request to /
	req1 := httptest.NewRequest("GET", "/", nil)
	rec1 := httptest.NewRecorder()
	httpHandler.ServeHTTP(rec1, req1)

	// Request to /api/data (will fail auth but that's ok)
	req2 := httptest.NewRequest("GET", "/api/data", nil)
	rec2 := httptest.NewRecorder()
	httpHandler.ServeHTTP(rec2, req2)

	// Multiple requests to same route
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		httpHandler.ServeHTTP(rec, req)
	}

	// Check logs
	logs := logBuffer.String()

	// Should see exactly 2 "Middleware stack registered" messages
	stackRegisteredCount := strings.Count(logs, "Middleware stack registered")
	if stackRegisteredCount != 2 {
		t.Errorf("Expected 'Middleware stack registered' exactly 2 times, found %d", stackRegisteredCount)
	}

	// Should see 1 "Default middleware registered" message
	defaultCount := strings.Count(logs, "Default middleware registered")
	if defaultCount != 1 {
		t.Errorf("Expected 'Default middleware registered' exactly once, found %d", defaultCount)
	}

	// Should NOT see any "enabled" messages from middleware factories
	if strings.Contains(logs, "Middleware enabled") {
		t.Error("Found 'Middleware enabled' in logs - middleware factories are being called per request")
		t.Logf("Full logs:\n%s", logs)
	}
}
