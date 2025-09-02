package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWebSocketUpgrade tests basic WebSocket upgrade functionality
func TestWebSocketUpgrade(t *testing.T) {
	// Create a test server with WebSocket handler
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	upgrader := Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for testing
		},
	}

	// WebSocket echo handler
	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		// Echo messages back to client
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(messageType, p); err != nil {
				break
			}
		}
	})

	// Create test server
	testServer := httptest.NewServer(srv.mux)
	defer testServer.Close()

	// This is a simplified test since we can't easily test WebSocket client connections
	// with our basic implementation. We'll test the upgrade mechanism instead.
	
	// Test WebSocket handshake
	req, err := http.NewRequest("GET", testServer.URL+"/ws", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Add WebSocket headers
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check if it's a WebSocket upgrade response
	if resp.StatusCode != 101 {
		t.Errorf("Expected status 101, got %d", resp.StatusCode)
	}
	
	if resp.Header.Get("Upgrade") != "websocket" {
		t.Errorf("Expected Upgrade header to be 'websocket', got %s", resp.Header.Get("Upgrade"))
	}
}

// TestWebSocketWithMiddleware tests WebSocket functionality through middleware stack
func TestWebSocketWithMiddleware(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add middleware stack
	srv.AddMiddlewareStack("*", DefaultMiddleware(srv))

	upgrader := Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// WebSocket handler that works with middleware
	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Failed to upgrade connection through middleware: %v", err)
			return
		}
		defer conn.Close()

		// Send a welcome message
		if err := conn.WriteMessage(TextMessage, []byte("connected")); err != nil {
			return
		}

		// Echo loop
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if err := conn.WriteMessage(messageType, p); err != nil {
				break
			}
		}
	})

	testServer := httptest.NewServer(srv.mux)
	defer testServer.Close()

	// Test WebSocket handshake through middleware
	req, err := http.NewRequest("GET", testServer.URL+"/ws", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Add WebSocket headers
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request through middleware: %v", err)
	}
	defer resp.Body.Close()

	// Check if it's a WebSocket upgrade response
	if resp.StatusCode != 101 {
		t.Errorf("Expected status 101, got %d", resp.StatusCode)
	}
}

// TestWebSocketProgressUpdates tests real-time progress updates (generic use case)
func TestWebSocketProgressUpdates(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Mock job status for testing
	type JobStatus struct {
		ID       string  `json:"id"`
		Progress float64 `json:"progress"`
		Status   string  `json:"status"`
		Complete bool    `json:"complete"`
	}

	// Mock job storage
	jobs := map[string]*JobStatus{
		"test-job": {
			ID:       "test-job",
			Progress: 0.0,
			Status:   "starting",
			Complete: false,
		},
	}

	upgrader := Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Progress tracking handler (generic progress updates)
	srv.HandleFunc("/ws/progress", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Failed to upgrade: %v", err)
			return
		}
		defer conn.Close()

		jobID := r.URL.Query().Get("jobId")
		if jobID == "" {
			jobID = "test-job"
		}

		job, exists := jobs[jobID]
		if !exists {
			conn.WriteMessage(TextMessage, []byte(`{"error": "job not found"}`))
			return
		}

		// Simulate progress updates
		for i := 0; i < 5; i++ {
			job.Progress = float64(i) * 25.0
			job.Status = fmt.Sprintf("processing step %d", i+1)
			if i == 4 {
				job.Complete = true
				job.Status = "completed"
			}

			// Send progress update
			message := fmt.Sprintf(`{"id":"%s","progress":%.1f,"status":"%s","complete":%t}`,
				job.ID, job.Progress, job.Status, job.Complete)
			
			if err := conn.WriteMessage(TextMessage, []byte(message)); err != nil {
				return
			}

			if job.Complete {
				break
			}
			time.Sleep(10 * time.Millisecond) // Small delay for testing
		}
	})

	testServer := httptest.NewServer(srv.mux)
	defer testServer.Close()

	// Test WebSocket handshake for progress endpoint
	req, err := http.NewRequest("GET", testServer.URL+"/ws/progress?jobId=test-job", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	
	// Add WebSocket headers
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check if it's a WebSocket upgrade response
	if resp.StatusCode != 101 {
		t.Errorf("Expected status 101, got %d", resp.StatusCode)
	}
}

// TestHijackerInterface tests that the loggingResponseWriter implements http.Hijacker
func TestHijackerInterface(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add logging middleware to ensure we're testing the wrapped writer
	srv.AddMiddlewareStack("*", MiddlewareStack{RequestLoggerMiddleware})

	var hijackerSupported bool
	srv.HandleFunc("/test-hijack", func(w http.ResponseWriter, r *http.Request) {
		// Test if the ResponseWriter implements http.Hijacker
		_, ok := w.(http.Hijacker)
		hijackerSupported = ok
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("hijacker_supported=%t", ok)))
	})

	testServer := httptest.NewServer(srv.mux)
	defer testServer.Close()

	// Make a request to test hijacker interface
	resp, err := http.Get(testServer.URL + "/test-hijack")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if !hijackerSupported {
		t.Error("Expected ResponseWriter to implement http.Hijacker interface")
	}
}