package websocket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestServer builds an httptest server with the provided handler chain.
func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestUpgraderHandshake(t *testing.T) {
	upgrader := Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Simple echo loop
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, payload); err != nil {
				return
			}
		}
	})

	server := newTestServer(t, mux)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/ws", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Upgrade") != "websocket" {
		t.Fatalf("expected Upgrade header 'websocket', got %q", resp.Header.Get("Upgrade"))
	}
}

func TestUpgraderWithMiddleware(t *testing.T) {
	upgrader := Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		conn.WriteMessage(TextMessage, []byte("connected"))
	})

	// Simple middleware chain that records requests and passes through.
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", time.Now().UnixNano()))
			next.ServeHTTP(w, r)
		})
	}

	server := newTestServer(t, mw(handler))

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}
}

func TestProgressUpdates(t *testing.T) {
	jobs := map[string]struct {
		status   string
		progress float64
	}{
		"job-1": {status: "running", progress: 0.5},
	}

	upgrader := Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/progress", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		jobID := r.URL.Query().Get("jobId")
		if jobID == "" {
			jobID = "job-1"
		}
		job := jobs[jobID]
		payload := fmt.Sprintf(`{"status":"%s","progress":%.2f}`, job.status, job.progress)
		conn.WriteMessage(TextMessage, []byte(payload))
	})

	server := newTestServer(t, mux)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/ws/progress?jobId=job-1", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
}
