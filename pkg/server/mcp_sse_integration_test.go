package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMCPWithSSEIntegration(t *testing.T) {
	// Create MCP handler with SSE support
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := NewMCPHandler(serverInfo)

	// Register a test tool
	handler.RegisterTool(&testTool{})

	// Create test server
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", handler.ServeHTTP)
	mux.HandleFunc("/mcp/sse", handler.ServeHTTP)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping: unable to bind test listener: %v", err)
		return
	}

	server := &http.Server{Handler: mux}
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		<-done
	}()

	t.Run("SSE Connection and MCP Flow", func(t *testing.T) {
		// 1. Connect SSE client
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/mcp", nil)
		if err != nil {
			t.Fatalf("Failed to create SSE request: %v", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to connect SSE: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Fatalf("Expected text/event-stream, got %s", ct)
		}

		// Create scanner for SSE events
		scanner := bufio.NewScanner(resp.Body)
		events := make(chan string, 10)
		go func() {
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					events <- strings.TrimPrefix(line, "data: ")
				}
			}
		}()

		// Get client ID from connection event
		var clientID string
		select {
		case event := <-events:
			var connEvent map[string]interface{}
			if err := json.Unmarshal([]byte(event), &connEvent); err != nil {
				t.Fatalf("Failed to parse connection event: %v", err)
			}
			if connEvent["type"] != "connection" {
				t.Fatalf("Expected connection event, got %v", connEvent["type"])
			}
			clientID = connEvent["clientId"].(string)
			if clientID == "" {
				t.Fatal("No client ID in connection event")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for connection event")
		}

		// 2. Send initialize request via HTTP with SSE client ID
		initReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
			"id": 1,
		}

		reqBody, _ := json.Marshal(initReq)
		httpReq, err := http.NewRequest("POST", baseURL+"/mcp", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("Failed to create HTTP request: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-SSE-Client-ID", clientID)

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("Failed to send initialize: %v", err)
		}
		httpResp.Body.Close()

		// 3. Verify response comes through SSE
		select {
		case event := <-events:
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(event), &response); err != nil {
				t.Fatalf("Failed to parse SSE response: %v", err)
			}

			if response["id"] != float64(1) {
				t.Fatalf("Expected response ID 1, got %v", response["id"])
			}

			result, ok := response["result"].(map[string]interface{})
			if !ok {
				t.Fatal("No result in response")
			}

			if result["protocolVersion"] != "2024-11-05" {
				t.Fatalf("Expected protocol version 2024-11-05, got %v", result["protocolVersion"])
			}

		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for SSE response")
		}

		// 4. Test tool call through SSE
		toolReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "test_tool",
				"arguments": map[string]interface{}{
					"message": "hello",
				},
			},
			"id": 2,
		}

		reqBody, _ = json.Marshal(toolReq)
		httpReq, err = http.NewRequest("POST", baseURL+"/mcp", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("Failed to create tool request: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-SSE-Client-ID", clientID)

		httpResp, err = http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("Failed to send tool call: %v", err)
		}
		httpResp.Body.Close()

		// Verify tool response through SSE
		select {
		case event := <-events:
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(event), &response); err != nil {
				t.Fatalf("Failed to parse tool response: %v", err)
			}

			if response["id"] != float64(2) {
				t.Fatalf("Expected response ID 2, got %v", response["id"])
			}

			result, ok := response["result"].(map[string]interface{})
			if !ok {
				t.Fatal("No result in tool response")
			}

			if result["content"].([]interface{})[0].(map[string]interface{})["text"] != "Echo: hello" {
				t.Fatal("Unexpected tool response")
			}

		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for tool response")
		}
	})

	t.Run("Multiple SSE Clients", func(t *testing.T) {
		// Connect two SSE clients
		clients := make([]string, 2)

		for i := 0; i < 2; i++ {
			req, err := http.NewRequest("GET", baseURL+"/mcp", nil)
			if err != nil {
				t.Fatalf("Failed to create request for client %d: %v", i, err)
			}
			req.Header.Set("Accept", "text/event-stream")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to connect SSE client %d: %v", i, err)
			}
			defer resp.Body.Close()

			// Read connection event to get client ID
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					var connEvent map[string]interface{}
					if err := json.Unmarshal([]byte(data), &connEvent); err == nil {
						if connEvent["type"] == "connection" {
							clients[i] = connEvent["clientId"].(string)
							break
						}
					}
				}
			}

			if clients[i] == "" {
				t.Fatalf("No client ID for client %d", i)
			}
		}

		// Verify different client IDs
		if clients[0] == clients[1] {
			t.Fatal("Clients have same ID")
		}
	})
}

// Simple test tool for integration testing
type testTool struct{}

func (t *testTool) Name() string {
	return "test_tool"
}

func (t *testTool) Description() string {
	return "Test tool for integration testing"
}

func (t *testTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message to echo",
			},
		},
		"required": []string{"message"},
	}
}

func (t *testTool) Execute(params map[string]interface{}) (interface{}, error) {
	msg, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message must be a string")
	}

	// Return a simple string - the handler will wrap it in the proper format
	return "Echo: " + msg, nil
}
