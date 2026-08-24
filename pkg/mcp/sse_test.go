package mcp

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	jsonrpc "github.com/osauer/hyperserve/v2/pkg/jsonrpc"
)

func TestSSEManager(t *testing.T) {
	manager := newSSEManager()

	t.Run("Client Management", func(t *testing.T) {
		w := httptest.NewRecorder()
		flusher := &mockFlusher{w: w}

		client := newSSEClient("test-client-1", "test-binding-1", w, flusher)
		manager.addClient("test-client-1", client)

		if count := manager.ClientCount(); count != 1 {
			t.Errorf("Expected 1 client, got %d", count)
		}

		response := &jsonrpc.Response{
			JSONRPC: "2.0",
			Result:  map[string]any{"message": "test"},
			ID:      1,
		}
		if err := manager.SendToClient("test-client-1", response); err != nil {
			t.Errorf("Failed to send to client: %v", err)
		}

		manager.removeClient("test-client-1")
		if count := manager.ClientCount(); count != 0 {
			t.Errorf("Expected 0 clients, got %d", count)
		}
	})

	t.Run("Broadcast", func(t *testing.T) {
		for i := range 3 {
			w := httptest.NewRecorder()
			flusher := &mockFlusher{w: w}
			client := newSSEClient(fmt.Sprintf("client-%d", i), fmt.Sprintf("binding-%d", i), w, flusher)
			manager.addClient(fmt.Sprintf("client-%d", i), client)
		}

		response := &jsonrpc.Response{
			JSONRPC: "2.0",
			Result:  map[string]any{"broadcast": "test"},
			ID:      nil,
		}
		manager.BroadcastToAll(response)

		for i := range 3 {
			manager.removeClient(fmt.Sprintf("client-%d", i))
		}
	})
}

// mockFlusher implements http.Flusher for testing
type mockFlusher struct {
	w       *httptest.ResponseRecorder
	flushed bool
}

func (f *mockFlusher) Flush() { f.flushed = true }

func TestSSEClientLifecycle(t *testing.T) {
	w := httptest.NewRecorder()
	flusher := &mockFlusher{w: w}
	client := newSSEClient("test-client", "test-binding", w, flusher)

	t.Run("State Transitions", func(t *testing.T) {
		if client.IsReady() {
			t.Error("Client should not be ready initially")
		}
		client.SetInitialized()
		if !client.initialized {
			t.Error("Client should be initialized")
		}
		client.SetReady()
		if !client.IsReady() {
			t.Error("Client should be ready")
		}
	})

	t.Run("Message Sending", func(t *testing.T) {
		response := &jsonrpc.Response{
			JSONRPC: "2.0",
			Result:  "test",
			ID:      1,
		}
		if err := client.Send(response); err != nil {
			t.Errorf("Failed to send message: %v", err)
		}
		select {
		case msg := <-client.messageChan:
			if msg.ID != 1 {
				t.Errorf("Expected message ID 1, got %v", msg.ID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Message not received in channel")
		}
	})

	t.Run("Close", func(t *testing.T) {
		client.Close()
		if err := client.Send(&jsonrpc.Response{}); err == nil {
			t.Error("Expected error when sending to closed client")
		}
	})
}
