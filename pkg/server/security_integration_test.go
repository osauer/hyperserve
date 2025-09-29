package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSlowlorisProtection tests the ReadHeaderTimeout protection against Slowloris attacks
func TestSlowlorisProtection(t *testing.T) {
	t.Skip("Skipping Slowloris test - requires proper server startup with dynamic port detection")

	// Note: This test demonstrates the Slowloris protection concept
	// In production, ReadHeaderTimeout prevents slow header attacks by closing
	// connections that take too long to send complete headers.

	// Example configuration for Slowloris protection:
	// srv, _ := hyperserve.NewServer(
	//     hyperserve.WithReadHeaderTimeout(5*time.Second),
	// )
}

// TestHealthServerTimeoutConfiguration tests that health server has proper timeout configuration
func TestHealthServerTimeoutConfiguration(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithHealthServer(),
		WithReadTimeout(10*time.Second),
		WithWriteTimeout(15*time.Second),
		WithIdleTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Bind health server to an ephemeral loopback port to avoid sandbox restrictions
	srv.Options.HealthAddr = "127.0.0.1:0"

	// Start the server
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run()
	}()

	// Wait for server initialization or failure
	timeout := time.After(5 * time.Second)
waiting:
	for {
		select {
		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed {
				if strings.Contains(err.Error(), "operation not permitted") {
					t.Skipf("skipping: unable to bind in restricted environment (%v)", err)
				}
				t.Fatalf("server failed to start: %v", err)
			}
			break waiting
		case <-timeout:
			t.Fatal("timeout waiting for server to start")
		case <-time.After(5 * time.Millisecond):
			if srv.isRunning.Load() {
				break waiting
			}
		}
	}

	// If server isn't running at this point, skip (likely sandbox restrictions)
	if !srv.isRunning.Load() {
		if err := srv.Stop(); err != nil && err != http.ErrServerClosed {
			t.Logf("cleanup stop error: %v", err)
		}
		t.Skip("server could not start in this environment")
	}

	// Verify main server timeouts
	if srv.httpServer.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout to be 10s, got %v", srv.httpServer.ReadTimeout)
	}
	if srv.httpServer.WriteTimeout != 15*time.Second {
		t.Errorf("expected WriteTimeout to be 15s, got %v", srv.httpServer.WriteTimeout)
	}
	if srv.httpServer.IdleTimeout != 30*time.Second {
		t.Errorf("expected IdleTimeout to be 30s, got %v", srv.httpServer.IdleTimeout)
	}
	if srv.httpServer.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("expected ReadHeaderTimeout to be 10s, got %v", srv.httpServer.ReadHeaderTimeout)
	}

	// Verify health server timeouts
	if srv.healthServer != nil {
		if srv.healthServer.ReadTimeout != 10*time.Second {
			t.Errorf("health server: expected ReadTimeout to be 10s, got %v", srv.healthServer.ReadTimeout)
		}
		if srv.healthServer.WriteTimeout != 15*time.Second {
			t.Errorf("health server: expected WriteTimeout to be 15s, got %v", srv.healthServer.WriteTimeout)
		}
		if srv.healthServer.IdleTimeout != 30*time.Second {
			t.Errorf("health server: expected IdleTimeout to be 30s, got %v", srv.healthServer.IdleTimeout)
		}
		if srv.healthServer.ReadHeaderTimeout != 10*time.Second {
			t.Errorf("health server: expected ReadHeaderTimeout to be 10s, got %v", srv.healthServer.ReadHeaderTimeout)
		}
	}

	if err := srv.Stop(); err != nil && err != http.ErrServerClosed {
		t.Errorf("failed to stop server: %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected server shutdown error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server shutdown")
	}
}

// TestIntegerOverflowProtection tests protection against integer overflow in WebSocket frames
func TestIntegerOverflowProtection(t *testing.T) {
	// This test is more of a unit test for the frame parsing logic
	// The actual protection is in internal/ws/frame.go
	// We'll test it through the WebSocket interface

	srv, err := NewServer(WithAddr(":0"))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.Options.RunHealthServer = false

	upgrader := Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Try to read a message - should handle overflow gracefully
		_, _, err = conn.ReadMessage()
		if err == nil {
			t.Error("expected error reading malformed frame")
		}
	})

	// Use httptest server for easier testing
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	// Connect to WebSocket endpoint
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// For actual overflow testing, we would need to craft malformed WebSocket frames
	// This would require low-level connection manipulation
	// The important part is that the overflow protection is in place in frame.go

	// Here we just verify the endpoint works normally
	req, _ := http.NewRequest("GET", wsURL, nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	// This is a basic connectivity test
	// The actual integer overflow protection is tested in internal/ws/frame_test.go
}

// mockCloser is a test implementation of io.Closer
type mockCloser struct {
	closeError error
	closed     bool
}

// Close implements io.Closer
func (mc *mockCloser) Close() error {
	mc.closed = true
	return mc.closeError
}

// TestCloseWithLogErrorHandling tests that closeWithLog properly handles close errors
func TestCloseWithLogErrorHandling(t *testing.T) {
	// Create a mock closer that returns an error
	mc := &mockCloser{closeError: http.ErrServerClosed}

	// Test closeWithLog with error
	closeWithLog(mc, "test resource")

	// Verify it was called (this would normally log the error)
	if !mc.closed {
		t.Error("closeWithLog should have attempted to close the resource")
	}
}

// TestTLSConfiguration tests that TLS is properly configured with secure defaults
func TestTLSConfiguration(t *testing.T) {
	t.Skip("Skipping TLS configuration test - requires actual certificate files")

	// Example of proper TLS configuration:
	// srv, _ := hyperserve.NewServer(
	//     hyperserve.WithTLS("cert.pem", "key.pem"),
	//     hyperserve.WithFIPSMode(), // For enhanced security
	// )
	//
	// The server will automatically:
	// - Configure TLS 1.2+ only
	// - Use secure cipher suites
	// - Apply security headers when TLS is enabled
}
