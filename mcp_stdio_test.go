package hyperserve

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStdioTransport_NewStdioTransport(t *testing.T) {
	transport := NewStdioTransport()
	
	if transport == nil {
		t.Fatal("NewStdioTransport returned nil")
	}
	
	if transport.scanner == nil {
		t.Error("Scanner is nil")
	}
	
	if transport.encoder == nil {
		t.Error("Encoder is nil")
	}
	
	if transport.logger == nil {
		t.Error("Logger is nil")
	}
}

func TestStdioTransport_NewStdioTransportWithLogger(t *testing.T) {
	customLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	transport := NewStdioTransportWithLogger(customLogger)
	
	if transport == nil {
		t.Fatal("NewStdioTransportWithLogger returned nil")
	}
	
	if transport.logger != customLogger {
		t.Error("Custom logger not set correctly")
	}
}

func TestStdioTransport_NewStdioTransportWithIO(t *testing.T) {
	input := strings.NewReader("test input")
	output := &bytes.Buffer{}
	
	transport := NewStdioTransportWithIO(input, output)
	
	if transport == nil {
		t.Fatal("NewStdioTransportWithIO returned nil")
	}
	
	if transport.scanner == nil {
		t.Error("Scanner is nil")
	}
	
	if transport.encoder == nil {
		t.Error("Encoder is nil")
	}
	
	if transport.logger == nil {
		t.Error("Logger is nil")
	}
}

func TestStdioTransport_NewStdioTransportWithIOAndLogger(t *testing.T) {
	input := strings.NewReader("test input")
	output := &bytes.Buffer{}
	customLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	
	transport := NewStdioTransportWithIOAndLogger(input, output, customLogger)
	
	if transport == nil {
		t.Fatal("NewStdioTransportWithIOAndLogger returned nil")
	}
	
	if transport.logger != customLogger {
		t.Error("Custom logger not set correctly")
	}
}

func TestStdioTransport_Send(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}
	transport := NewStdioTransportWithIO(input, output)
	
	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage("1"),
		Result:  "test result",
	}
	
	err := transport.Send(response)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	
	// Verify the response was written to output
	var sentResponse JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &sentResponse); err != nil {
		t.Fatalf("Failed to unmarshal sent response: %v", err)
	}
	
	if sentResponse.JSONRPC != JSONRPCVersion {
		t.Errorf("Expected JSONRPC version %s, got %s", JSONRPCVersion, sentResponse.JSONRPC)
	}
	
	if sentResponse.ID == nil {
		t.Error("Expected ID to be set")
	} else {
		// ID could be various types, let's check the actual value
		idBytes, _ := json.Marshal(sentResponse.ID)
		if string(idBytes) != "1" {
			t.Errorf("Expected ID '1', got %s", string(idBytes))
		}
	}
	
	if sentResponse.Result != "test result" {
		t.Errorf("Expected result 'test result', got %v", sentResponse.Result)
	}
}

func TestStdioTransport_Send_ConcurrentSafety(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}
	transport := NewStdioTransportWithIO(input, output)
	
	// Test concurrent sends to verify mutex protection
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			response := &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      json.RawMessage(string(rune(id + 48))), // ASCII numbers
				Result:  "test result",
			}
			
			err := transport.Send(response)
			if err != nil {
				t.Errorf("Concurrent send failed: %v", err)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(1 * time.Second):
			t.Fatal("Concurrent send test timed out")
		}
	}
}

func TestStdioTransport_Receive(t *testing.T) {
	requestData := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "test_method",
		"params":  map[string]interface{}{"key": "value"},
		"id":      1,
	}
	
	requestJSON, _ := json.Marshal(requestData)
	input := strings.NewReader(string(requestJSON))
	output := &bytes.Buffer{}
	
	transport := NewStdioTransportWithIO(input, output)
	
	request, err := transport.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	
	if request.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC '2.0', got %s", request.JSONRPC)
	}
	
	if request.Method != "test_method" {
		t.Errorf("Expected method 'test_method', got %s", request.Method)
	}
	
	if request.ID == nil {
		t.Error("Expected ID to be set")
	} else {
		// ID could be various types, let's check the actual value
		idBytes, _ := json.Marshal(request.ID)
		if string(idBytes) != "1" {
			t.Errorf("Expected ID '1', got %s", string(idBytes))
		}
	}
}

func TestStdioTransport_Receive_EOF(t *testing.T) {
	// Empty input should return EOF
	input := strings.NewReader("")
	output := &bytes.Buffer{}
	
	transport := NewStdioTransportWithIO(input, output)
	
	_, err := transport.Receive()
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestStdioTransport_Receive_InvalidJSON(t *testing.T) {
	input := strings.NewReader("invalid json")
	output := &bytes.Buffer{}
	
	transport := NewStdioTransportWithIO(input, output)
	
	_, err := transport.Receive()
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	
	if !strings.Contains(err.Error(), "failed to unmarshal request") {
		t.Errorf("Expected unmarshal error, got %v", err)
	}
}

func TestStdioTransport_Close(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}
	transport := NewStdioTransportWithIO(input, output)
	
	err := transport.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestMCPHandler_RunStdioLoop_SingleRequest(t *testing.T) {
	// Create a test handler
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Create test input with initialize request
	initRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": MCPVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
		"id": 1,
	}
	
	requestJSON, _ := json.Marshal(initRequest)
	input := strings.NewReader(string(requestJSON))
	output := &bytes.Buffer{}
	
	// Create transport for testing ProcessRequestWithTransport
	transport := NewStdioTransportWithIO(input, output)
	
	// Process a single request to test the loop logic
	err := handler.ProcessRequestWithTransport(transport)
	if err != nil {
		t.Errorf("ProcessRequestWithTransport failed: %v", err)
	}
	
	// Verify response was written
	if output.Len() == 0 {
		t.Error("No response written to output")
	}
	
	// Verify the response is valid JSON
	var response JSONRPCResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
}

func TestStdioTransport_ErrorHandling(t *testing.T) {
	// Test error response creation
	input := strings.NewReader("invalid json")
	output := &bytes.Buffer{}
	
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	transport := NewStdioTransportWithIO(input, output)
	
	err := handler.ProcessRequestWithTransport(transport)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
	
	if !strings.Contains(err.Error(), "failed to unmarshal request") {
		t.Errorf("Expected unmarshal error, got %v", err)
	}
}

func TestStdioTransport_Integration(t *testing.T) {
	// Test full integration with MCP protocol
	handler := NewMCPHandler(MCPServerInfo{Name: "test-server", Version: "1.0.0"})
	
	// Add a calculator tool for testing
	handler.RegisterTool(NewCalculatorTool())
	
	// Test sequence of requests
	testCases := []struct {
		name     string
		request  map[string]interface{}
		expectOK bool
	}{
		{
			name: "initialize",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "initialize",
				"params": map[string]interface{}{
					"protocolVersion": MCPVersion,
					"capabilities":    map[string]interface{}{},
					"clientInfo": map[string]interface{}{
						"name":    "test-client",
						"version": "1.0.0",
					},
				},
				"id": 1,
			},
			expectOK: true,
		},
		{
			name: "tools/list",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/list",
				"id":      2,
			},
			expectOK: true,
		},
		{
			name: "tools/call",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "calculator",
					"arguments": map[string]interface{}{
						"operation": "add",
						"a":         5,
						"b":         3,
					},
				},
				"id": 3,
			},
			expectOK: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestJSON, _ := json.Marshal(tc.request)
			input := strings.NewReader(string(requestJSON))
			output := &bytes.Buffer{}
			
			transport := NewStdioTransportWithIO(input, output)
			
			err := handler.ProcessRequestWithTransport(transport)
			if tc.expectOK && err != nil {
				t.Errorf("Expected success, got error: %v", err)
			}
			
			if !tc.expectOK && err == nil {
				t.Error("Expected error, got success")
			}
			
			if tc.expectOK {
				// Verify response was written
				if output.Len() == 0 {
					t.Error("No response written to output")
				}
				
				// Verify the response is valid JSON
				var response JSONRPCResponse
				if err := json.Unmarshal(output.Bytes(), &response); err != nil {
					t.Errorf("Failed to unmarshal response: %v", err)
				}
				
				if response.Error != nil {
					t.Errorf("Expected no error, got %+v", response.Error)
				}
			}
		})
	}
}