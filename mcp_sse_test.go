package hyperserve

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPSSEEndpoint(t *testing.T) {
	// Create a server with MCP enabled
	srv, err := NewServer(
		WithMCPSupport("test-server", "1.0.0"),
		WithMCPBuiltinTools(true),
	)
	if err != nil {
		t.Fatal(err)
	}


	// Create test server
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	t.Run("SSE Connection", func(t *testing.T) {
		// Debug: Check if MCP is enabled
		if !srv.MCPEnabled() {
			t.Fatal("MCP is not enabled on the server")
		}
		
		// Debug: Check MCP endpoint
		t.Logf("MCP endpoint: %s", srv.Options.MCPEndpoint)
		t.Logf("MCP handler: %v", srv.mcpHandler)
		
		// First test base MCP endpoint
		baseResp, err := http.Get(ts.URL + "/mcp")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Base MCP endpoint status: %d", baseResp.StatusCode)
		baseResp.Body.Close()
		
		// Connect to SSE endpoint
		resp, err := http.Get(ts.URL + "/mcp/sse")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		
		// Debug: Print response status
		t.Logf("Response status: %d", resp.StatusCode)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response body: %s", body)
		}

		// Check headers
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Expected Content-Type text/event-stream, got %s", ct)
		}

		// Read first event (connection event)
		reader := bufio.NewReader(resp.Body)
		
		// Read until we get an event
		var eventData string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			
			if strings.HasPrefix(line, "data: ") {
				eventData = strings.TrimPrefix(line, "data: ")
				eventData = strings.TrimSpace(eventData)
				break
			}
		}

		// Parse connection event
		var connEvent map[string]interface{}
		if err := json.Unmarshal([]byte(eventData), &connEvent); err != nil {
			t.Fatalf("Failed to parse connection event: %v", err)
		}

		if connEvent["type"] != "connection" {
			t.Errorf("Expected connection event, got %v", connEvent)
		}

		if clientID, ok := connEvent["clientId"].(string); !ok || clientID == "" {
			t.Error("Connection event missing clientId")
		}
	})

	t.Run("HTTP Request with SSE Client ID", func(t *testing.T) {
		// First connect to get a client ID
		resp, err := http.Get(ts.URL + "/mcp/sse")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Read connection event to get client ID
		reader := bufio.NewReader(resp.Body)
		var clientID string
		
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			
			if strings.HasPrefix(line, "data: ") {
				eventData := strings.TrimPrefix(line, "data: ")
				eventData = strings.TrimSpace(eventData)
				
				var connEvent map[string]interface{}
				if err := json.Unmarshal([]byte(eventData), &connEvent); err == nil {
					if id, ok := connEvent["clientId"].(string); ok {
						clientID = id
						break
					}
				}
			}
		}

		// Now send a request with the SSE client ID
		reqBody := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"ping","id":1}`)
		req, err := http.NewRequest("POST", ts.URL+"/mcp", reqBody)
		if err != nil {
			t.Fatal(err)
		}
		
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-SSE-Client-ID", clientID)
		
		resp2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp2.Body.Close()

		// Should get 202 Accepted
		if resp2.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(resp2.Body)
			t.Errorf("Expected status 202, got %d: %s", resp2.StatusCode, body)
		}
	})
}

func TestSSEManager(t *testing.T) {
	manager := NewSSEManager()

	t.Run("Client Management", func(t *testing.T) {
		// Mock response writer and flusher
		w := httptest.NewRecorder()
		flusher := &mockFlusher{w: w}
		
		client := newSSEClient("test-client-1", w, flusher)
		
		// Add client
		manager.addClient("test-client-1", client)
		
		// Check client count
		if count := manager.GetClientCount(); count != 1 {
			t.Errorf("Expected 1 client, got %d", count)
		}

		// Send message to client
		response := &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  map[string]interface{}{"message": "test"},
			ID:      1,
		}
		
		err := manager.SendToClient("test-client-1", response)
		if err != nil {
			t.Errorf("Failed to send to client: %v", err)
		}

		// Remove client
		manager.removeClient("test-client-1")
		
		// Check client count
		if count := manager.GetClientCount(); count != 0 {
			t.Errorf("Expected 0 clients, got %d", count)
		}
	})

	t.Run("Broadcast", func(t *testing.T) {
		// Add multiple clients
		for i := 0; i < 3; i++ {
			w := httptest.NewRecorder()
			flusher := &mockFlusher{w: w}
			client := newSSEClient(fmt.Sprintf("client-%d", i), w, flusher)
			manager.addClient(fmt.Sprintf("client-%d", i), client)
		}

		// Broadcast message
		response := &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  map[string]interface{}{"broadcast": "test"},
			ID:      nil,
		}
		
		manager.BroadcastToAll(response)

		// Clean up
		for i := 0; i < 3; i++ {
			manager.removeClient(fmt.Sprintf("client-%d", i))
		}
	})
}

// mockFlusher implements http.Flusher for testing
type mockFlusher struct {
	w       *httptest.ResponseRecorder
	flushed bool
}

func (f *mockFlusher) Flush() {
	f.flushed = true
}

func TestSSEClientLifecycle(t *testing.T) {
	w := httptest.NewRecorder()
	flusher := &mockFlusher{w: w}
	client := newSSEClient("test-client", w, flusher)

	t.Run("State Transitions", func(t *testing.T) {
		// Initially not ready
		if client.IsReady() {
			t.Error("Client should not be ready initially")
		}

		// Set initialized
		client.SetInitialized()
		if !client.initialized {
			t.Error("Client should be initialized")
		}

		// Set ready
		client.SetReady()
		if !client.IsReady() {
			t.Error("Client should be ready")
		}
	})

	t.Run("Message Sending", func(t *testing.T) {
		response := &JSONRPCResponse{
			JSONRPC: "2.0",
			Result:  "test",
			ID:      1,
		}

		err := client.Send(response)
		if err != nil {
			t.Errorf("Failed to send message: %v", err)
		}

		// Try to receive from channel
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
		
		// Try to send after close
		err := client.Send(&JSONRPCResponse{})
		if err == nil {
			t.Error("Expected error when sending to closed client")
		}
	})
}

func TestMCPSSE_DirectJSONRPCPost(t *testing.T) {
	// Create a server with MCP enabled
	srv, err := NewServer(
		WithMCPSupport("test-server", "1.0.0"),
		WithMCPBuiltinTools(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create test server
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	t.Run("Direct POST to SSE endpoint", func(t *testing.T) {
		// Send JSON-RPC request directly to SSE endpoint
		reqBody := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"ping","id":1}`)
		resp, err := http.Post(ts.URL+"/mcp/sse", "application/json", reqBody)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Should get 200 OK with JSON response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, body)
		}

		// Check Content-Type is JSON
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("Expected JSON content type, got: %s", contentType)
		}

		// Parse response
		var response JSONRPCResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode JSON response: %v", err)
		}

		
		// Verify response structure
		if response.JSONRPC != "2.0" {
			t.Errorf("Expected JSONRPC 2.0, got %s", response.JSONRPC)
		}
		
		// Check ID (could be int or float64 from JSON)
		switch id := response.ID.(type) {
		case int:
			if id != 1 {
				t.Errorf("Expected ID 1, got %v", id)
			}
		case float64:
			if id != 1.0 {
				t.Errorf("Expected ID 1, got %v", id)
			}
		default:
			t.Errorf("Expected numeric ID, got %T: %v", id, id)
		}

		if response.Error != nil {
			t.Errorf("Unexpected error: %v", response.Error)
		}

		// Ping should return {"message": "pong"}
		if result, ok := response.Result.(map[string]interface{}); !ok {
			t.Errorf("Expected map result, got %v", response.Result)
		} else if msg, exists := result["message"]; !exists || msg != "pong" {
			t.Errorf("Expected 'pong' message, got %v", result)
		}
	})

	t.Run("Invalid JSON to SSE endpoint", func(t *testing.T) {
		// Send invalid JSON
		reqBody := bytes.NewBufferString(`{"invalid":json}`)
		resp, err := http.Post(ts.URL+"/mcp/sse", "application/json", reqBody)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Should get 400 Bad Request
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Wrong Content-Type to SSE endpoint", func(t *testing.T) {
		// Send with wrong content type
		reqBody := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"ping","id":1}`)
		resp, err := http.Post(ts.URL+"/mcp/sse", "text/plain", reqBody)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Should get 400 Bad Request
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}