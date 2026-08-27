package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	jsonrpc "github.com/osauer/hyperserve/v2/jsonrpc"
)

// httpTransport implements Transport for HTTP-based communication.
type httpTransport struct {
	w      http.ResponseWriter
	r      *http.Request
	logger *slog.Logger
}

func newHTTPTransport(w http.ResponseWriter, r *http.Request, logger *slog.Logger) *httpTransport {
	return &httpTransport{w: w, r: r, logger: logger}
}

func (t *httpTransport) Send(response *jsonrpc.Response) error {
	t.w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(t.w).Encode(response)
}

func (t *httpTransport) NoResponse() error {
	t.w.WriteHeader(http.StatusAccepted)
	return nil
}

func (t *httpTransport) Receive() (*jsonrpc.Request, error) {
	if t.r.Method != http.MethodPost {
		return nil, fmt.Errorf("%w: %s", ErrMethodNotAllowed, t.r.Method)
	}
	if !strings.Contains(t.r.Header.Get("Content-Type"), "application/json") {
		return nil, fmt.Errorf("%w: Content-Type must be application/json", ErrUnsupportedContentType)
	}
	t.r.Body = http.MaxBytesReader(t.w, t.r.Body, mcpHTTPMaxBody)
	body, err := io.ReadAll(t.r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	var request jsonrpc.Request
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("failed to decode request: %w", err)
	}
	return &request, nil
}

func (t *httpTransport) Close() error { return nil }
