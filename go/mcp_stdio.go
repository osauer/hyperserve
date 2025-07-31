package hyperserve

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// stdioTransport implements MCPTransport for stdin/stdout communication
// Note: Both Send and Receive are thread-safe, protected by mutex
type stdioTransport struct {
	scanner *bufio.Scanner
	encoder *json.Encoder
	logger  *slog.Logger
	mu      sync.Mutex // Protects both encoder and scanner for thread safety
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(logger *slog.Logger) *stdioTransport {
	scanner := bufio.NewScanner(os.Stdin)
	// Set reasonable buffer limits to prevent memory exhaustion
	// Max line size: 1MB (suitable for most JSON-RPC requests)
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, 0, 64*1024)      // 64KB initial buffer
	scanner.Buffer(buf, maxScanTokenSize)
	
	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(os.Stdout),
		logger:  logger,
	}
}

// NewStdioTransportWithIO creates a new stdio transport with custom IO
func NewStdioTransportWithIO(r io.Reader, w io.Writer, logger *slog.Logger) *stdioTransport {
	scanner := bufio.NewScanner(r)
	// Set reasonable buffer limits to prevent memory exhaustion
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, 0, 64*1024)      // 64KB initial buffer
	scanner.Buffer(buf, maxScanTokenSize)
	
	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(w),
		logger:  logger,
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
	t.mu.Lock()
	defer t.mu.Unlock()
	
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

// createErrorResponse creates a standard JSON-RPC error response
func createErrorResponse(code int, message string, data interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// RunStdioLoop runs the MCP handler in stdio mode
// The loop continues processing requests until EOF is received on stdin.
// EOF is treated as a normal shutdown signal (e.g., when stdin is closed).
// This behavior is appropriate for stdio servers which typically run
// for the lifetime of the parent process.
func (h *MCPHandler) RunStdioLoop() error {
	transport := NewStdioTransport(h.logger)
	// Note: Close() is currently a no-op but called for future compatibility
	defer transport.Close()
	
	h.logger.Debug("MCP stdio server started")
	
	// Main message loop
	for {
		err := h.ProcessRequestWithTransport(transport)
		if errors.Is(err, io.EOF) {
			h.logger.Debug("MCP stdio server shutting down", "reason", "EOF received")
			break
		}
		if err != nil {
			h.logger.Error("Error processing request", "error", err)
			// Determine appropriate error code based on error type
			errorCode := ErrorCodeInternalError
			if strings.Contains(err.Error(), "unmarshal") || strings.Contains(err.Error(), "parse") {
				errorCode = ErrorCodeParseError
			} else if strings.Contains(err.Error(), "scanner error") {
				errorCode = ErrorCodeInvalidRequest
			}
			
			// Send error response
			errorResponse := createErrorResponse(errorCode, "Request processing error", err.Error())
			if sendErr := transport.Send(errorResponse); sendErr != nil {
				h.logger.Error("Failed to send error response", "error", sendErr)
				// Critical failure: both request processing and error response failed
				// Log the critical state but continue processing to maintain service
				h.logger.Error("Critical: Unable to send error response to client", 
					"original_error", err.Error(),
					"send_error", sendErr.Error())
			}
		}
	}
	
	return nil
}