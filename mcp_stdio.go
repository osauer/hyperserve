package hyperserve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// stdioTransport implements MCPTransport for stdin/stdout communication
type stdioTransport struct {
	scanner *bufio.Scanner
	encoder *json.Encoder
	logger  *slog.Logger
	mu      sync.Mutex // Protects encoder for concurrent sends
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport() *stdioTransport {
	return NewStdioTransportWithLogger(logger)
}

// NewStdioTransportWithLogger creates a new stdio transport with custom logger
func NewStdioTransportWithLogger(customLogger *slog.Logger) *stdioTransport {
	return &stdioTransport{
		scanner: bufio.NewScanner(os.Stdin),
		encoder: json.NewEncoder(os.Stdout),
		logger:  customLogger,
	}
}

// NewStdioTransportWithIO creates a new stdio transport with custom IO
func NewStdioTransportWithIO(r io.Reader, w io.Writer) *stdioTransport {
	return NewStdioTransportWithIOAndLogger(r, w, logger)
}

// NewStdioTransportWithIOAndLogger creates a new stdio transport with custom IO and logger
func NewStdioTransportWithIOAndLogger(r io.Reader, w io.Writer, customLogger *slog.Logger) *stdioTransport {
	return &stdioTransport{
		scanner: bufio.NewScanner(r),
		encoder: json.NewEncoder(w),
		logger:  customLogger,
	}
}

// Send sends a JSON-RPC response to stdout
func (t *stdioTransport) Send(response *JSONRPCResponse) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if err := t.encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	
	return nil
}

// Receive receives a JSON-RPC request from stdin
func (t *stdioTransport) Receive() (*JSONRPCRequest, error) {
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, fmt.Errorf("scanner error: %w", err)
		}
		return nil, io.EOF
	}
	
	var request JSONRPCRequest
	if err := json.Unmarshal(t.scanner.Bytes(), &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}
	
	return &request, nil
}

// Close closes the stdio transport (no-op)
func (t *stdioTransport) Close() error {
	return nil
}

// RunStdioLoop runs the MCP handler in stdio mode
func (h *MCPHandler) RunStdioLoop() error {
	transport := NewStdioTransport()
	defer transport.Close()
	
	h.logger.Info("MCP stdio server started")
	
	// Main message loop
	for {
		err := h.ProcessRequestWithTransport(transport)
		if err == io.EOF {
			h.logger.Info("MCP stdio server shutting down (EOF)")
			break
		}
		if err != nil {
			h.logger.Error("Error processing request", "error", err)
			// Send error response
			errorResponse := &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				Error: &JSONRPCError{
					Code:    ErrorCodeInternalError,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
			if sendErr := transport.Send(errorResponse); sendErr != nil {
				h.logger.Error("Failed to send error response", "error", sendErr)
			}
		}
	}
	
	return nil
}