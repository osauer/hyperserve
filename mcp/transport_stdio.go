package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	jsonrpc "github.com/osauer/hyperserve/v2/jsonrpc"
)

// stdioTransport implements Transport for stdin/stdout communication.
// Both Send and Receive are thread-safe.
type stdioTransport struct {
	scanner *bufio.Scanner
	encoder *json.Encoder
	logger  *slog.Logger
	sendMu  sync.Mutex
	recvMu  sync.Mutex
}

// NewStdioTransport creates a new stdio transport using os.Stdin / os.Stdout.
func NewStdioTransport(logger *slog.Logger) *stdioTransport {
	const maxScanTokenSize = 1024 * 1024 // 1MB
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)
	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(os.Stdout),
		logger:  logger,
	}
}

// NewStdioTransportWithIO creates a new stdio transport with custom IO.
func NewStdioTransportWithIO(r io.Reader, w io.Writer, logger *slog.Logger) *stdioTransport {
	const maxScanTokenSize = 1024 * 1024
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScanTokenSize)
	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(w),
		logger:  logger,
	}
}

func (t *stdioTransport) Send(response *jsonrpc.Response) error {
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	if err := t.encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	return nil
}

func (t *stdioTransport) SendNotification(method string, params any) error {
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	notification := rpcNotification{
		JSONRPC: jsonrpc.Version,
		Method:  method,
		Params:  params,
	}
	if err := t.encoder.Encode(notification); err != nil {
		return fmt.Errorf("failed to encode notification: %w", err)
	}
	return nil
}

func (t *stdioTransport) Receive() (*jsonrpc.Request, error) {
	t.recvMu.Lock()
	defer t.recvMu.Unlock()
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, fmt.Errorf("scanner error: %w", err)
		}
		return nil, io.EOF
	}
	var request jsonrpc.Request
	if err := json.Unmarshal(t.scanner.Bytes(), &request); err != nil {
		return nil, &stdioParseError{err}
	}
	return &request, nil
}

func (t *stdioTransport) Close() error { return nil }

// A complete malformed record is recoverable; scanner and output errors are not.
type stdioParseError struct{ error }

func (e *stdioParseError) Error() string { return "failed to unmarshal request: " + e.error.Error() }
func (e *stdioParseError) Unwrap() error { return e.error }

func createErrorResponse(code int, message string, data any) *jsonrpc.Response {
	return &jsonrpc.Response{
		JSONRPC: jsonrpc.Version,
		Error: &jsonrpc.ErrorDetails{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// RunStdioLoop runs until EOF or a terminal input/output error. Malformed
// complete JSON records receive a parse error and do not end the session.
func (h *Handler) RunStdioLoop() error {
	transport := NewStdioTransport(h.logger)
	return h.runStdioLoop(transport)
}

func (h *Handler) runStdioLoop(transport *stdioTransport) error {
	defer transport.Close()
	session := newMCPSession(context.Background(), h, transport)
	defer session.close()
	engine := h.newRPCEngine(session)

	h.logger.Debug("MCP stdio server started")

	for {
		err := h.processRequestWithTransportAndSession(transport, session, engine)
		if errors.Is(err, io.EOF) {
			h.logger.Debug("MCP stdio server shutting down", "reason", "EOF received")
			return nil
		}
		if err != nil {
			h.logger.Error("Error processing request", "error", err)
			if _, ok := errors.AsType[*stdioParseError](err); !ok {
				return err
			}
			errorResponse := createErrorResponse(jsonrpc.ErrorCodeParseError, "Request processing error", err.Error())
			if sendErr := transport.Send(errorResponse); sendErr != nil {
				return sendErr
			}
		}
	}
}
