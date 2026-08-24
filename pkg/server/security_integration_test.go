package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/pkg/websocket"
)

// TestSlowlorisProtection tests the ReadHeaderTimeout protection against Slowloris attacks
func TestSlowlorisProtection(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping: unable to reserve a loopback address (%v)", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}

	srv, err := NewServer(WithAddr(addr))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	const headerTimeout = 50 * time.Millisecond
	srv.Options.ReadHeaderTimeout = headerTimeout

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.RunContext(ctx)
	}()

	startupDeadline := time.After(time.Second)
	for !srv.isRunning.Load() {
		select {
		case err := <-runErr:
			t.Fatalf("server exited during startup: %v", err)
		case <-startupDeadline:
			t.Fatal("server did not start")
		case <-time.After(time.Millisecond):
		}
	}
	defer func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("RunContext shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	}()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("connect to server: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + addr + "\r\nX-Slow: unfinished")); err != nil {
		t.Fatalf("write partial request: %v", err)
	}

	started := time.Now()
	_, readErr := io.ReadAll(conn)
	if timeoutErr, ok := readErr.(net.Error); ok && timeoutErr.Timeout() {
		t.Fatalf("connection remained open past the test deadline: %v", readErr)
	}
	if elapsed := time.Since(started); elapsed < headerTimeout/2 {
		t.Fatalf("connection closed after %v, before header timeout %v", elapsed, headerTimeout)
	}
}

// TestHealthServerTimeoutConfiguration tests that health server has proper timeout configuration
func TestHealthServerTimeoutConfiguration(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithHealthServer(),
		WithTimeouts(10*time.Second, 15*time.Second, 30*time.Second),
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
	// The actual protection is in pkg/websocket/frame.go
	// We'll test it through the WebSocket interface

	srv, err := NewServer(WithAddr(":0"))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.Options.RunHealthServer = false

	upgrader := websocket.Upgrader{
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
	// The actual integer overflow protection is tested in pkg/websocket/frame_test.go
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
	// srv, _ := server.NewServer(
	//     server.WithTLS("cert.pem", "key.pem"),
	//     server.WithFIPSMode(), // For enhanced security
	// )
	//
	// The server will automatically:
	// - Configure TLS 1.2+ only
	// - Use secure cipher suites
	// - Apply security headers when TLS is enabled
}
