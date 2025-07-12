package hyperserve

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketUpgrade tests basic WebSocket upgrade functionality
func TestWebSocketUpgrade(t *testing.T) {
	// Create a test server with WebSocket handler
	srv := NewServer(
		WithPort(0), // Use random port for testing
		WithDebug(false),
	)

	upgrader := websocket.Upgrader{
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

	// Convert HTTP URL to WebSocket URL
	wsURL := strings.Replace(testServer.URL+"/ws", "http://", "ws://", 1)

	// Test WebSocket connection
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send and receive test message
	testMessage := []byte("hello websocket")
	if err := ws.WriteMessage(websocket.TextMessage, testMessage); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	_, received, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if !bytes.Equal(testMessage, received) {
		t.Fatalf("Expected %s, got %s", testMessage, received)
	}
}

// TestWebSocketWithMiddleware tests WebSocket functionality through middleware stack
func TestWebSocketWithMiddleware(t *testing.T) {
	srv := NewServer(
		WithPort(0),
		WithDebug(false),
	)

	// Add middleware stack
	srv.AddMiddleware("*", DefaultMiddleware(srv))

	upgrader := websocket.Upgrader{
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
		if err := conn.WriteMessage(websocket.TextMessage, []byte("connected")); err != nil {
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

	wsURL := strings.Replace(testServer.URL+"/ws", "http://", "ws://", 1)

	// Test connection through middleware
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect through middleware: %v", err)
	}
	defer ws.Close()

	// Read welcome message
	_, welcome, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}
	if string(welcome) != "connected" {
		t.Fatalf("Expected 'connected', got %s", welcome)
	}

	// Test echo functionality
	testMessage := []byte("middleware test")
	if err := ws.WriteMessage(websocket.TextMessage, testMessage); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	_, received, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if !bytes.Equal(testMessage, received) {
		t.Fatalf("Expected %s, got %s", testMessage, received)
	}
}

// TestWebSocketProgressUpdates tests real-time progress updates (DAW use case)
func TestWebSocketProgressUpdates(t *testing.T) {
	srv := NewServer(
		WithPort(0),
		WithDebug(false),
	)

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

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Progress tracking handler (simulates DAW rendering)
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
			conn.WriteMessage(websocket.TextMessage, []byte(`{"error": "job not found"}`))
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
			
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
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

	wsURL := strings.Replace(testServer.URL+"/ws/progress?jobId=test-job", "http://", "ws://", 1)

	// Test progress updates
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	// Read progress updates
	updateCount := 0
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			break
		}

		updateCount++
		messageStr := string(message)
		
		// Verify message format
		if !strings.Contains(messageStr, "progress") {
			t.Errorf("Expected progress update, got: %s", messageStr)
		}

		// Check for completion
		if strings.Contains(messageStr, `"complete":true`) {
			break
		}
	}

	if updateCount != 5 {
		t.Errorf("Expected 5 progress updates, got %d", updateCount)
	}
}

// TestHijackerInterface tests that the loggingResponseWriter implements http.Hijacker
func TestHijackerInterface(t *testing.T) {
	srv := NewServer(
		WithPort(0),
		WithDebug(false),
	)

	// Add logging middleware to ensure we're testing the wrapped writer
	srv.AddMiddleware("*", MiddlewareStack{RequestLoggerMiddleware})

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