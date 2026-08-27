package hyperserve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/osauer/hyperserve/v2/mcp"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMCPWithSSEIntegration(t *testing.T) {
	// Create MCP handler with SSE support
	serverInfo := mcp.ServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := mcp.NewHandler(serverInfo)
	//lint:ignore SA1019 This regression intentionally exercises legacy compatibility.
	handler.SetLegacyRoutedSSEEnabled(true)

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
				if after, ok := strings.CutPrefix(line, "data: "); ok {
					events <- after
				}
			}
		}()

		// Get client ID + binding token from connection event. The
		// binding token must accompany every routed POST; missing or
		// wrong binding → 403.
		var clientID, bindingToken string
		select {
		case event := <-events:
			var connEvent map[string]any
			if err := json.Unmarshal([]byte(event), &connEvent); err != nil {
				t.Fatalf("Failed to parse connection event: %v", err)
			}
			if connEvent["type"] != "connection" {
				t.Fatalf("Expected connection event, got %v", connEvent["type"])
			}
			clientID = connEvent["clientId"].(string)
			bindingToken = connEvent["bindingToken"].(string)
			if clientID == "" || bindingToken == "" {
				t.Fatal("Missing clientId or bindingToken in connection event")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for connection event")
		}

		// 2. Send initialize request via HTTP with SSE client ID
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
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
		httpReq.Header.Set("X-SSE-Binding", bindingToken)

		httpResp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("Failed to send initialize: %v", err)
		}
		httpResp.Body.Close()

		// 3. Verify response comes through SSE
		select {
		case event := <-events:
			var response map[string]any
			if err := json.Unmarshal([]byte(event), &response); err != nil {
				t.Fatalf("Failed to parse SSE response: %v", err)
			}

			if response["id"] != float64(1) {
				t.Fatalf("Expected response ID 1, got %v", response["id"])
			}

			result, ok := response["result"].(map[string]any)
			if !ok {
				t.Fatal("No result in response")
			}

			if result["protocolVersion"] != mcp.DefaultProtocolVersion {
				t.Fatalf("Expected protocol version %s, got %v", mcp.DefaultProtocolVersion, result["protocolVersion"])
			}

		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for SSE response")
		}

		// 4. Test tool call through SSE
		toolReq := map[string]any{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]any{
				"name": "test_tool",
				"arguments": map[string]any{
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
		httpReq.Header.Set("X-SSE-Binding", bindingToken)

		httpResp, err = http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("Failed to send tool call: %v", err)
		}
		httpResp.Body.Close()

		// Verify tool response through SSE
		select {
		case event := <-events:
			var response map[string]any
			if err := json.Unmarshal([]byte(event), &response); err != nil {
				t.Fatalf("Failed to parse tool response: %v", err)
			}

			if response["id"] != float64(2) {
				t.Fatalf("Expected response ID 2, got %v", response["id"])
			}

			result, ok := response["result"].(map[string]any)
			if !ok {
				t.Fatal("No result in tool response")
			}

			if result["content"].([]any)[0].(map[string]any)["text"] != "Echo: hello" {
				t.Fatal("Unexpected tool response")
			}

		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for tool response")
		}
	})

	t.Run("Multiple SSE Clients", func(t *testing.T) {
		// Connect two SSE clients
		clients := make([]string, 2)

		for i := range 2 {
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
				if after, ok := strings.CutPrefix(line, "data: "); ok {
					data := after
					var connEvent map[string]any
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

func (t *testTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Message to echo",
			},
		},
		"required": []string{"message"},
	}
}

func (t *testTool) Execute(params map[string]any) (any, error) {
	msg, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message must be a string")
	}

	// Return a simple string - the handler will wrap it in the proper format
	return "Echo: " + msg, nil
}
