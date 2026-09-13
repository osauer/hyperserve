package hyperserve

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLoggerOmitsURLSecrets(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		for _, target := range []string{
			"/pair.html?pair=synthetic-pair&nonce=synthetic-nonce",
			"http://synthetic-user:synthetic-password@example.test/files/a%2Fb?token=synthetic-token",
		} {
			t.Run(level.String()+"/"+target, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: level}))
				srv, err := New(WithLogger(logger))
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodGet, target, nil)
				query := req.URL.RawQuery
				srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
					if r.URL.RawQuery != query {
						t.Errorf("handler query = %q, want %q", r.URL.RawQuery, query)
					}
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte("ok"))
				})
				srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
				if strings.Contains(logs.String(), "synthetic-") {
					t.Errorf("request log contains URL credentials: %s", &logs)
				}
				record := requestLogRecord(t, &logs)
				if record["url"] != req.URL.EscapedPath() {
					t.Errorf("logged URL = %v, want escaped path %q", record["url"], req.URL.EscapedPath())
				}
				if record["method"] != http.MethodGet || record["status"] != float64(http.StatusAccepted) || record["bytes"] != float64(2) {
					t.Errorf("request accounting changed: %v", record)
				}
			})
		}
	}
}

func requestLogRecord(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(logs)
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["msg"] == "Request completed" {
			return record
		}
	}
	t.Fatal("request completion log missing")
	return nil
}

// TestMiddlewareLogBehavior ensures middleware registration logs only appear during setup, not per request
func TestMiddlewareLogBehavior(t *testing.T) {
	// Capture logs
	var logBuffer bytes.Buffer
	handler := slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	// Create server
	srv, err := New(
		WithAddr(":0"),
		WithLogger(slog.New(handler)),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add some middleware
	srv.UsePrefix("/api", func(next http.Handler) http.Handler { return next })
	srv.UsePrefix("/api", HeadersMiddleware(srv.options))

	// Add a test handler
	srv.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	// Make multiple requests
	httpHandler := srv.middleware.applyToMux(srv.mux)
	for i := range 3 {
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
		"Authentication middleware enabled",
		"HeadersMiddleware enabled",
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
	// Create server
	srv, err := New(WithAddr(":0"), WithLogger(slog.New(handler)))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register middleware stacks
	srv.UsePrefix("/", SecureWeb(srv.options))
	srv.UsePrefix("/api", func(next http.Handler) http.Handler { return next })

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

	// Request to /api/data
	req2 := httptest.NewRequest("GET", "/api/data", nil)
	rec2 := httptest.NewRecorder()
	httpHandler.ServeHTTP(rec2, req2)

	// Multiple requests to same route
	for range 3 {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		httpHandler.ServeHTTP(rec, req)
	}

	// Check logs
	logs := logBuffer.String()

	// Should see exactly 2 registration messages.
	stackRegisteredCount := strings.Count(logs, "Middleware registered")
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
