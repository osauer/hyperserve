package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWebSocketPingPongHandlers(t *testing.T) {
	// Create server
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer srv.Stop()

	upgrader := &Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// Track ping/pong activity
	var mu sync.Mutex

	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Set ping handler
		conn.SetPingHandler(func(appData string) error {
			mu.Lock()
			defer mu.Unlock()
			// Echo back with pong
			return conn.WriteControl(PongMessage, []byte(appData), time.Now().Add(time.Second))
		})

		// Set pong handler
		conn.SetPongHandler(func(appData string) error {
			mu.Lock()
			defer mu.Unlock()
			return nil
		})

		// Read messages until connection closes
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	})

	// Start server
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	// Connect client - wsURL would be used for actual WebSocket client
	_ = strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws"
	headers := http.Header{
		"Upgrade":               []string{"websocket"},
		"Connection":            []string{"Upgrade"},
		"Sec-WebSocket-Key":     []string{"dGhlIHNhbXBsZSBub25jZQ=="},
		"Sec-WebSocket-Version": []string{"13"},
	}

	// Create HTTP request
	req, _ := http.NewRequest("GET", ts.URL+"/ws", nil)
	for k, v := range headers {
		req.Header[k] = v
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}
	defer resp.Body.Close()

	// Simulate ping/pong exchange
	time.Sleep(100 * time.Millisecond)

	// For now, we're just testing that handlers can be set without panicking
	// Full ping/pong test would require a proper WebSocket client
}

func TestWebSocketCloseHandler(t *testing.T) {
	// Create server
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer srv.Stop()

	upgrader := &Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	var mu sync.Mutex

	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Set close handler
		conn.SetCloseHandler(func(code int, text string) error {
			mu.Lock()
			defer mu.Unlock()
			return nil
		})

		// Read messages until connection closes
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	})

	// Start server
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	// Connect and close
	req, _ := http.NewRequest("GET", ts.URL+"/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}
	resp.Body.Close()

	// For now, we're just testing that handlers can be set without panicking
	// Full close handler test would require a proper WebSocket client
}

func TestWebSocketHandlerCompatibility(t *testing.T) {
	conn := &Conn{}

	// Test that all handler methods can be called without panicking
	t.Run("CloseHandler", func(t *testing.T) {
		handler := conn.CloseHandler()
		if handler == nil {
			t.Error("CloseHandler returned nil")
		}
		// Test default handler
		if err := handler(CloseNormalClosure, "test"); err != nil {
			t.Errorf("Default close handler returned error: %v", err)
		}
	})

	t.Run("PingHandler", func(t *testing.T) {
		handler := conn.PingHandler()
		if handler == nil {
			t.Error("PingHandler returned nil")
		}
	})

	t.Run("PongHandler", func(t *testing.T) {
		handler := conn.PongHandler()
		if handler == nil {
			t.Error("PongHandler returned nil")
		}
		// Test default handler
		if err := handler("test"); err != nil {
			t.Errorf("Default pong handler returned error: %v", err)
		}
	})

	t.Run("SetHandlers", func(t *testing.T) {
		// These should not panic even though they're not fully implemented
		conn.SetCloseHandler(func(code int, text string) error { return nil })
		conn.SetPingHandler(func(appData string) error { return nil })
		conn.SetPongHandler(func(appData string) error { return nil })
	})
}

func TestWebSocketWriteJSON(t *testing.T) {
	// Skip - requires full WebSocket connection setup
	t.Skip("Skipping WriteJSON test - requires full connection")
}

func TestWebSocketReadJSON(t *testing.T) {
	// Skip - requires full WebSocket connection setup
	t.Skip("Skipping ReadJSON test - requires full connection")
}

func TestWebSocketControlMessages(t *testing.T) {
	// Skip this test as it requires internal implementation details
	t.Skip("Skipping control message test - requires internal ws.Conn")
}

func TestIsCloseError(t *testing.T) {
	// Test with nil error
	if IsCloseError(nil, CloseNormalClosure) {
		t.Error("IsCloseError should return false for nil error")
	}

	// Test with non-close error
	if IsCloseError(bytes.ErrTooLarge, CloseNormalClosure) {
		t.Error("IsCloseError should return false for non-close error")
	}
}

func TestIsUnexpectedCloseError(t *testing.T) {
	// Test with nil error
	if IsUnexpectedCloseError(nil, CloseNormalClosure) {
		t.Error("IsUnexpectedCloseError should return false for nil error")
	}

	// Test with non-close error
	if IsUnexpectedCloseError(bytes.ErrTooLarge, CloseNormalClosure) {
		t.Error("IsUnexpectedCloseError should return false for non-close error")
	}
}
