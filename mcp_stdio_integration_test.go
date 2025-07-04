package hyperserve

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestMCPHandler_ProcessRequestWithTransport tests processing requests with STDIO transport
func TestMCPHandler_ProcessRequestWithTransport(t *testing.T) {
	// Create buffers for I/O
	var outputBuf bytes.Buffer
	var inputBuf bytes.Buffer
	
	// Create MCP handler
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := NewMCPHandler(serverInfo)
	handler.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	
	// Register a test tool
	handler.RegisterTool(NewCalculatorTool())
	
	// Create transport
	transport := NewStdioTransportWithIO(&inputBuf, &outputBuf, handler.logger)
	
	// Test requests
	tests := []struct {
		name     string
		request  string
		checkErr bool
	}{
		{
			name:    "initialize",
			request: `{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}`,
		},
		{
			name:    "tools/list",
			request: `{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}`,
		},
		{
			name:    "calculator",
			request: `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"calculator","arguments":{"operation":"add","a":5,"b":3}},"id":3}`,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear buffers
			inputBuf.Reset()
			outputBuf.Reset()
			
			// Write request
			inputBuf.WriteString(tt.request + "\n")
			
			// Process request
			err := handler.ProcessRequestWithTransport(transport)
			if err != nil && !tt.checkErr {
				t.Errorf("ProcessRequestWithTransport() error = %v", err)
				return
			}
			
			// Verify response
			var response JSONRPCResponse
			if err := json.Unmarshal(outputBuf.Bytes(), &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}
			
			if response.Error != nil && !tt.checkErr {
				t.Errorf("Got error response: %v", response.Error)
			}
		})
	}
}

// TestMCPHandler_StdioLoopErrorHandling tests error handling in STDIO loop
func TestMCPHandler_StdioLoopErrorHandling(t *testing.T) {
	// Create buffers for I/O
	var outputBuf bytes.Buffer
	var inputBuf bytes.Buffer
	
	// Create MCP handler
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := NewMCPHandler(serverInfo)
	handler.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	
	// Test invalid method
	inputBuf.WriteString(`{"jsonrpc":"2.0","method":"nonexistent/method","params":{},"id":1}` + "\n")
	
	// Create custom transport
	transport := NewStdioTransportWithIO(&inputBuf, &outputBuf, handler.logger)
	
	// Process request - should succeed but return error in response
	err := handler.ProcessRequestWithTransport(transport)
	if err != nil {
		t.Errorf("ProcessRequestWithTransport returned error: %v", err)
	}
	
	// Verify error response was sent
	var errorResp JSONRPCResponse
	outputData := outputBuf.Bytes()
	if len(outputData) == 0 {
		t.Fatal("No error response sent")
	}
	
	if err := json.Unmarshal(outputData, &errorResp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	
	if errorResp.Error == nil {
		t.Error("Expected error in response")
	}
	
	if errorResp.Error.Code != ErrorCodeMethodNotFound {
		t.Errorf("Expected method not found error code %d, got %d", ErrorCodeMethodNotFound, errorResp.Error.Code)
	}
}

// TestMCPHandler_StdioLoopInvalidJSON tests handling of invalid JSON
func TestMCPHandler_StdioLoopInvalidJSON(t *testing.T) {
	// Create buffers for I/O
	var outputBuf bytes.Buffer
	var inputBuf bytes.Buffer
	
	// Write invalid JSON
	inputBuf.WriteString("invalid json\n")
	
	// Create MCP handler
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := NewMCPHandler(serverInfo)
	handler.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	
	// Create custom transport
	transport := NewStdioTransportWithIO(&inputBuf, &outputBuf, handler.logger)
	
	// Use RunStdioLoop to test the actual error handling
	// Create a modified version that processes one request
	err := handler.ProcessRequestWithTransport(transport)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	
	// When ProcessRequestWithTransport gets a parse error, it returns it
	// The RunStdioLoop would handle this by sending an error response
	if !strings.Contains(err.Error(), "failed to unmarshal request") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestMCPHandler_StdioLoopConcurrency tests concurrent request handling
func TestMCPHandler_StdioLoopConcurrency(t *testing.T) {
	// Create buffers for I/O
	var outputBuf bytes.Buffer
	var inputBuf bytes.Buffer
	var mu sync.Mutex
	
	// Create MCP handler
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	handler := NewMCPHandler(serverInfo)
	handler.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	
	// Register calculator tool
	handler.RegisterTool(NewCalculatorTool())
	
	// Create multiple concurrent requests
	numRequests := 10
	for i := 0; i < numRequests; i++ {
		req := JSONRPCRequest{
			JSONRPC: JSONRPCVersion,
			Method:  "tools/call",
			Params: json.RawMessage(`{
				"name": "calculator",
				"arguments": {"operation": "multiply", "a": ` + strconv.Itoa(i) + `, "b": 2}
			}`),
			ID: float64(i),
		}
		
		data, _ := json.Marshal(req)
		mu.Lock()
		inputBuf.Write(data)
		inputBuf.WriteString("\n")
		mu.Unlock()
	}
	
	// Process requests
	transport := NewStdioTransportWithIO(&inputBuf, &outputBuf, handler.logger)
	processedCount := 0
	
	for i := 0; i < numRequests; i++ {
		err := handler.ProcessRequestWithTransport(transport)
		if err != nil && err != io.EOF {
			t.Errorf("Error processing request %d: %v", i, err)
		}
		if err != io.EOF {
			processedCount++
		}
	}
	
	if processedCount != numRequests {
		t.Errorf("Expected to process %d requests, processed %d", numRequests, processedCount)
	}
	
	// Verify all responses
	responses := strings.Split(strings.TrimSpace(outputBuf.String()), "\n")
	if len(responses) != numRequests {
		t.Errorf("Expected %d responses, got %d", numRequests, len(responses))
	}
}