package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

// TestStdioTransport_SendReceive tests basic send and receive operations
func TestStdioTransport_SendReceive(t *testing.T) {
	// Create buffers for I/O
	var outputBuf bytes.Buffer
	inputData := `{"jsonrpc":"2.0","method":"test","params":{"foo":"bar"},"id":1}` + "\n"
	inputBuf := strings.NewReader(inputData)

	// Create transport with custom I/O
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(inputBuf, &outputBuf, logger)

	// Test Receive
	request, err := transport.Receive()
	if err != nil {
		t.Fatalf("Failed to receive request: %v", err)
	}

	if request.Method != "test" {
		t.Errorf("Expected method 'test', got %s", request.Method)
	}

	if idFloat, ok := request.ID.(float64); !ok || idFloat != 1 {
		t.Errorf("Expected ID 1, got %v (type %T)", request.ID, request.ID)
	}

	// Test Send
	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  json.RawMessage(`{"success":true}`),
		ID:      float64(1),
	}

	err = transport.Send(response)
	if err != nil {
		t.Fatalf("Failed to send response: %v", err)
	}

	// Verify output
	var sentResponse JSONRPCResponse
	if err := json.Unmarshal(outputBuf.Bytes(), &sentResponse); err != nil {
		t.Fatalf("Failed to unmarshal sent response: %v", err)
	}

	if sentResponse.ID != response.ID {
		t.Errorf("Response ID mismatch: expected %v, got %v", response.ID, sentResponse.ID)
	}
}

// TestStdioTransport_ReceiveEOF tests EOF handling
func TestStdioTransport_ReceiveEOF(t *testing.T) {
	// Empty input to trigger EOF
	inputBuf := strings.NewReader("")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(inputBuf, io.Discard, logger)

	_, err := transport.Receive()
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	}
}

// TestStdioTransport_ReceiveInvalidJSON tests invalid JSON handling
func TestStdioTransport_ReceiveInvalidJSON(t *testing.T) {
	inputData := "invalid json\n"
	inputBuf := strings.NewReader(inputData)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(inputBuf, io.Discard, logger)

	_, err := transport.Receive()
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to unmarshal request") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestStdioTransport_SendError tests send error handling
func TestStdioTransport_SendError(t *testing.T) {
	// Create a writer that always fails
	failWriter := &failingWriter{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(strings.NewReader(""), failWriter, logger)

	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  json.RawMessage(`{"test":true}`),
	}

	err := transport.Send(response)
	if err == nil {
		t.Error("Expected error when sending to failing writer")
	}

	if !strings.Contains(err.Error(), "failed to encode response") {
		t.Errorf("Expected encode error, got: %v", err)
	}
}

// TestStdioTransport_ConcurrentSend tests concurrent send operations
func TestStdioTransport_ConcurrentSend(t *testing.T) {
	var outputBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(strings.NewReader(""), &outputBuf, logger)

	// Send multiple responses concurrently
	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func(id int) {
			result := map[string]int{"id": id}
			resultJSON, _ := json.Marshal(result)
			response := &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				Result:  json.RawMessage(resultJSON),
				ID:      float64(id),
			}
			if err := transport.Send(response); err != nil {
				t.Errorf("Failed to send response %d: %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all sends to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify all responses were sent
	lines := strings.Split(strings.TrimSpace(outputBuf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(lines))
	}
}

// TestStdioTransport_Close tests the close operation
func TestStdioTransport_Close(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransport(logger)

	err := transport.Close()
	if err != nil {
		t.Errorf("Close should not return error, got: %v", err)
	}
}

// TestCreateErrorResponse tests the error response helper
func TestCreateErrorResponse(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
		data    interface{}
		wantErr bool
	}{
		{
			name:    "Basic error",
			code:    -32603,
			message: "Internal error",
			data:    "test error",
		},
		{
			name:    "Error with nil data",
			code:    -32600,
			message: "Invalid request",
			data:    nil,
		},
		{
			name:    "Error with complex data",
			code:    -32602,
			message: "Invalid params",
			data:    map[string]string{"field": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := createErrorResponse(tt.code, tt.message, tt.data)

			if response.JSONRPC != JSONRPCVersion {
				t.Errorf("Expected JSONRPC version %s, got %s", JSONRPCVersion, response.JSONRPC)
			}

			if response.Error == nil {
				t.Fatal("Expected error in response, got nil")
			}

			if response.Error.Code != tt.code {
				t.Errorf("Expected error code %d, got %d", tt.code, response.Error.Code)
			}

			if response.Error.Message != tt.message {
				t.Errorf("Expected error message %s, got %s", tt.message, response.Error.Message)
			}

			// For complex data types, do type-specific comparison
			switch expected := tt.data.(type) {
			case map[string]string:
				if actual, ok := response.Error.Data.(map[string]string); ok {
					if len(actual) != len(expected) {
						t.Errorf("Expected error data %v, got %v", tt.data, response.Error.Data)
					}
					for k, v := range expected {
						if actual[k] != v {
							t.Errorf("Expected error data %v, got %v", tt.data, response.Error.Data)
						}
					}
				} else {
					t.Errorf("Expected error data to be map[string]string, got %T", response.Error.Data)
				}
			default:
				if response.Error.Data != tt.data {
					t.Errorf("Expected error data %v, got %v", tt.data, response.Error.Data)
				}
			}
		})
	}
}

// TestStdioTransport_LargeInput tests handling of large JSON inputs
func TestStdioTransport_LargeInput(t *testing.T) {
	// Create a large JSON payload (900KB, under the 1MB limit)
	largeData := make(map[string]string)
	for i := 0; i < 9000; i++ {
		key := "field_" + strconv.Itoa(i)
		largeData[key] = strings.Repeat("x", 100) // 100 chars per value
	}

	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		Method:  "test_large",
		Params:  largeData,
		ID:      float64(1),
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal large request: %v", err)
	}

	inputBuf := bytes.NewReader(append(requestJSON, '\n'))
	var outputBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(inputBuf, &outputBuf, logger)

	// Should successfully receive large input
	receivedRequest, err := transport.Receive()
	if err != nil {
		t.Fatalf("Failed to receive large request: %v", err)
	}

	if receivedRequest.Method != "test_large" {
		t.Errorf("Expected method 'test_large', got %s", receivedRequest.Method)
	}
}

// TestStdioTransport_ExceedsBufferLimit tests handling of inputs that exceed buffer limit
func TestStdioTransport_ExceedsBufferLimit(t *testing.T) {
	// Create a payload that exceeds 1MB limit
	veryLargeData := strings.Repeat("x", 1024*1024+1) // 1MB + 1 byte
	inputData := `{"jsonrpc":"2.0","method":"test","params":"` + veryLargeData + `","id":1}` + "\n"

	inputBuf := strings.NewReader(inputData)
	var outputBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	transport := NewStdioTransportWithIO(inputBuf, &outputBuf, logger)

	// Should fail with scanner error
	_, err := transport.Receive()
	if err == nil {
		t.Error("Expected error for oversized input, got nil")
	}

	if !strings.Contains(err.Error(), "scanner error") {
		t.Errorf("Expected scanner error for oversized input, got: %v", err)
	}
}

// TestStdioTransport_MalformedJSON tests various malformed JSON inputs
func TestStdioTransport_MalformedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"Truncated JSON", `{"jsonrpc":"2.0","method":"test"`},
		{"Invalid escape", `{"jsonrpc":"2.0","method":"test\u","id":1}`},
		{"Trailing comma", `{"jsonrpc":"2.0","method":"test","id":1,}`},
		{"Missing quotes", `{jsonrpc:"2.0","method":"test","id":1}`},
		{"Empty line", ""},
		{"Whitespace only", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputBuf := strings.NewReader(tt.input + "\n")
			var outputBuf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			transport := NewStdioTransportWithIO(inputBuf, &outputBuf, logger)

			_, err := transport.Receive()
			if err == nil {
				t.Error("Expected error for malformed JSON")
			}

			// Empty input results in EOF, others in unmarshal errors
			if tt.name == "Empty line" || tt.name == "Whitespace only" {
				if err != io.EOF && !strings.Contains(err.Error(), "unmarshal") {
					t.Errorf("Expected EOF or unmarshal error, got: %v", err)
				}
			} else if !strings.Contains(err.Error(), "unmarshal") {
				t.Errorf("Expected unmarshal error, got: %v", err)
			}
		})
	}
}

// failingWriter is a writer that always returns an error
type failingWriter struct{}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write failed")
}
