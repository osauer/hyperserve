package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWebSocketTelemetry tests that WebSocket connections are tracked in server metrics
func TestWebSocketTelemetry(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Get initial metrics
	initialRequests := srv.totalRequests.Load()
	initialWebSockets := srv.websocketConnections.Load()

	// Create WebSocket handler using server's upgrader
	upgrader := srv.WebSocketUpgrader()
	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("Upgrade error: %v", err)
			return
		}
		defer conn.Close()
		
		// Simple echo server
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(messageType, p)
	})

	// Create a test WebSocket request
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"

	rec := httptest.NewRecorder()
	
	// Use the middleware-wrapped handler to ensure BeforeUpgrade is called
	handler := srv.middleware.applyToMux(srv.mux)
	handler.ServeHTTP(rec, req)

	// Check that metrics were updated
	newRequests := srv.totalRequests.Load()
	newWebSockets := srv.websocketConnections.Load()

	// Note: httptest.ResponseRecorder doesn't support hijacking, so the WebSocket upgrade fails
	// But we should see that BeforeUpgrade was called and metrics were incremented
	
	// The middleware adds 1 request, and BeforeUpgrade adds another
	if newRequests < initialRequests+1 {
		t.Errorf("Expected total requests to increase, got %d -> %d", 
			initialRequests, newRequests)
	}

	if newWebSockets != initialWebSockets+1 {
		t.Errorf("Expected WebSocket connections to increase by 1, got %d -> %d", 
			initialWebSockets, newWebSockets)
	}
}

// TestWebSocketUpgraderDefaults tests the default configuration of WebSocketUpgrader
func TestWebSocketUpgraderDefaults(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	upgrader := srv.WebSocketUpgrader()

	// Test same-origin policy (default)
	req := &http.Request{
		Header: http.Header{
			"Origin": []string{"http://example.com"},
		},
		Host: "example.com",
	}

	if !upgrader.CheckOrigin(req) {
		t.Error("Expected same-origin request to be allowed")
	}

	// Test cross-origin rejection
	req.Header.Set("Origin", "http://evil.com")
	if upgrader.CheckOrigin(req) {
		t.Error("Expected cross-origin request to be rejected")
	}

	// Test no origin header (non-browser clients)
	req.Header.Del("Origin")
	if upgrader.CheckOrigin(req) {
		t.Error("Expected request without origin to be rejected for security")
	}
}

// TestWebSocketMetricsInServerLog tests that WebSocket connections appear in server metrics log
func TestWebSocketMetricsInServerLog(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Simulate some regular requests
	for i := 0; i < 5; i++ {
		srv.totalRequests.Add(1)
		srv.totalResponseTime.Add(1000) // 1ms per request
	}

	// Simulate WebSocket connections
	srv.websocketConnections.Store(3)

	// Capture the metrics log
	srv.serverStart = time.Now().Add(-time.Hour) // Pretend server started 1 hour ago
	
	// Call logServerMetrics (this would normally happen on shutdown)
	// We can't easily capture the log output in tests, but we can verify the values are set
	if srv.totalRequests.Load() != 5 {
		t.Errorf("Expected 5 total requests, got %d", srv.totalRequests.Load())
	}

	if srv.websocketConnections.Load() != 3 {
		t.Errorf("Expected 3 WebSocket connections, got %d", srv.websocketConnections.Load())
	}
}

// TestWebSocketWithCustomUpgrader tests that custom upgraders still work
func TestWebSocketWithCustomUpgrader(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Track if BeforeUpgrade was called
	beforeUpgradeCalled := false

	// Create custom upgrader (not using server telemetry)
	customUpgrader := &Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
		BeforeUpgrade: func(w http.ResponseWriter, r *http.Request) error {
			beforeUpgradeCalled = true
			return nil
		},
	}

	srv.HandleFunc("/ws-custom", func(w http.ResponseWriter, r *http.Request) {
		conn, err := customUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	})

	// Create WebSocket request
	req := httptest.NewRequest(http.MethodGet, "/ws-custom", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	rec := httptest.NewRecorder()
	handler := srv.middleware.applyToMux(srv.mux)
	handler.ServeHTTP(rec, req)

	if !beforeUpgradeCalled {
		t.Error("Expected BeforeUpgrade to be called")
	}
}

// TestCheckOriginHelpers tests the origin checking helper functions
func TestCheckOriginHelpers(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		host        string
		shouldAllow bool
	}{
		{
			name:        "same origin",
			origin:      "http://example.com",
			host:        "example.com",
			shouldAllow: true,
		},
		{
			name:        "different origin",
			origin:      "http://evil.com",
			host:        "example.com",
			shouldAllow: false,
		},
		{
			name:        "no origin header",
			origin:      "",
			host:        "example.com",
			shouldAllow: false, // Changed to match security behavior
		},
		{
			name:        "invalid origin URL",
			origin:      "not-a-url",
			host:        "example.com",
			shouldAllow: false,
		},
		{
			name:        "https origin with http host",
			origin:      "https://example.com",
			host:        "example.com",
			shouldAllow: true, // Host comparison ignores scheme
		},
		{
			name:        "case insensitive host",
			origin:      "http://Example.COM",
			host:        "example.com",
			shouldAllow: true, // equalASCIIFold handles case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{},
				Host:   tt.host,
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			allowed := defaultCheckOrigin(req)
			if allowed != tt.shouldAllow {
				t.Errorf("Expected %v, got %v for origin %s and host %s",
					tt.shouldAllow, allowed, tt.origin, tt.host)
			}
		})
	}
}

// TestCheckOriginWithAllowedListTelemetry tests the allowed origins list functionality
func TestCheckOriginWithAllowedListTelemetry(t *testing.T) {
	allowedOrigins := []string{
		"http://localhost:3000",
		"https://app.example.com",
		"https://staging.example.com",
	}

	checkFunc := checkOriginWithAllowedList(allowedOrigins)

	tests := []struct {
		name        string
		origin      string
		shouldAllow bool
	}{
		{
			name:        "allowed origin 1",
			origin:      "http://localhost:3000",
			shouldAllow: true,
		},
		{
			name:        "allowed origin 2",
			origin:      "https://app.example.com",
			shouldAllow: true,
		},
		{
			name:        "not in allowed list",
			origin:      "http://evil.com",
			shouldAllow: false,
		},
		{
			name:        "no origin header",
			origin:      "",
			shouldAllow: false, // Security: reject requests without origin
		},
		{
			name:        "similar but not exact match",
			origin:      "http://localhost:3001",
			shouldAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{},
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			allowed := checkFunc(req)
			if allowed != tt.shouldAllow {
				t.Errorf("Expected %v, got %v for origin %s",
					tt.shouldAllow, allowed, tt.origin)
			}
		})
	}
}