package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

func newStreamableRequest(t *testing.T, method string, params map[string]any, id any) *http.Request {
	t.Helper()
	if params == nil {
		params = make(map[string]any)
	}
	params["_meta"] = map[string]any{protocolVersionMetaKey: StreamableHTTPProtocolVersion}
	body := map[string]any{
		"jsonrpc": jsonrpc.Version,
		"method":  method,
		"params":  params,
	}
	if id != nil {
		body["id"] = id
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(headerProtocolVersion, StreamableHTTPProtocolVersion)
	req.Header.Set(headerMethod, method)
	return req
}

func serveStreamable(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeRPCResponse(t *testing.T, rec *httptest.ResponseRecorder) jsonrpc.Response {
	t.Helper()
	var response jsonrpc.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%q", err, rec.Body.String())
	}
	return response
}

func TestStreamableHTTPServerDiscover(t *testing.T) {
	h := newHandlerForTest(t)
	rec := serveStreamable(t, h, newStreamableRequest(t, "server/discover", nil, "discover-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	response := decodeRPCResponse(t, rec)
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want object", response.Result)
	}
	if result["resultType"] != "complete" {
		t.Errorf("resultType = %v, want complete", result["resultType"])
	}
	versions, _ := result["supportedVersions"].([]any)
	if len(versions) != 1 || versions[0] != StreamableHTTPProtocolVersion {
		t.Errorf("supportedVersions = %v", result["supportedVersions"])
	}
	meta, _ := result["_meta"].(map[string]any)
	serverInfo, _ := meta["io.modelcontextprotocol/serverInfo"].(map[string]any)
	if serverInfo["name"] != "test" || serverInfo["version"] != "0.0.1" {
		t.Errorf("server info = %v", serverInfo)
	}
}

func TestStreamableHTTPDualAcceptPOSTIsNotLegacySSE(t *testing.T) {
	h := newHandlerForTest(t)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := serveStreamable(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, POST was routed as legacy SSE", got)
	}
}

func TestInitializeEraProtocolHeaderUsesCompatibilityPath(t *testing.T) {
	h := newHandlerForTest(t)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(headerProtocolVersion, DefaultProtocolVersion)
	rec := serveStreamable(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, compatibility POST was not handled as JSON", got)
	}
}

func TestHTTPNotificationsReturnAcceptedWithoutBody(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		h := newHandlerForTest(t)
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"initialized"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := serveStreamable(t, h, req)
		if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
			t.Fatalf("status/body = %d/%q, want 202/empty", rec.Code, rec.Body.String())
		}
	})

	t.Run("streamable", func(t *testing.T) {
		h := newHandlerForTest(t)
		rec := serveStreamable(t, h, newStreamableRequest(t, "tools/list", nil, nil))
		if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
			t.Fatalf("status/body = %d/%q, want 202/empty", rec.Code, rec.Body.String())
		}
	})
}

func TestStreamableHTTPHeaderValidation(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
		wantCode   int
	}{
		{
			name: "missing protocol version",
			mutate: func(r *http.Request) {
				r.Header.Del(headerProtocolVersion)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   errorCodeHeaderMismatch,
		},
		{
			name: "unsupported protocol version",
			mutate: func(r *http.Request) {
				r.Header.Set(headerProtocolVersion, "2099-01-01")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   errorCodeUnsupportedProtocolVersion,
		},
		{
			name: "body version mismatch",
			mutate: func(r *http.Request) {
				data := `{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25"}},"id":1}`
				r.Body = ioNopCloser(data)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   errorCodeHeaderMismatch,
		},
		{
			name: "method mismatch",
			mutate: func(r *http.Request) {
				r.Header.Set(headerMethod, "resources/list")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   errorCodeHeaderMismatch,
		},
		{
			name: "duplicate method header",
			mutate: func(r *http.Request) {
				r.Header.Add(headerMethod, "tools/list")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   errorCodeHeaderMismatch,
		},
		{
			name: "unknown method",
			mutate: func(r *http.Request) {
				r.Header.Set(headerMethod, "unknown/method")
				data := `{"jsonrpc":"2.0","method":"unknown/method","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":1}`
				r.Body = ioNopCloser(data)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   jsonrpc.ErrorCodeMethodNotFound,
		},
		{
			name: "legacy ping removed from current revision",
			mutate: func(r *http.Request) {
				r.Header.Set(headerMethod, "ping")
				data := `{"jsonrpc":"2.0","method":"ping","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":1}`
				r.Body = ioNopCloser(data)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   jsonrpc.ErrorCodeMethodNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerForTest(t)
			req := newStreamableRequest(t, "tools/list", nil, 1)
			tt.mutate(req)
			rec := serveStreamable(t, h, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			response := decodeRPCResponse(t, rec)
			if response.Error == nil || response.Error.Code != tt.wantCode {
				t.Fatalf("error = %+v, want code %d", response.Error, tt.wantCode)
			}
		})
	}
}

func ioNopCloser(body string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(body))
}

func TestStreamableHTTPNameHeaderValidation(t *testing.T) {
	h := newHandlerForTest(t)
	tool := &recordingTool{name: "echo-世界", result: "ok"}
	h.RegisterTool(tool)

	req := newStreamableRequest(t, "tools/call", map[string]any{
		"name":      tool.Name(),
		"arguments": map[string]any{"value": "hello"},
	}, 7)
	encoded := base64.StdEncoding.EncodeToString([]byte(tool.Name()))
	req.Header.Set(headerName, "=?base64?"+encoded+"?=")
	rec := serveStreamable(t, h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	bad := newStreamableRequest(t, "tools/call", map[string]any{
		"name":      tool.Name(),
		"arguments": map[string]any{},
	}, 8)
	bad.Header.Set(headerName, "another-tool")
	badRec := serveStreamable(t, h, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("mismatched name status = %d, want 400", badRec.Code)
	}
	response := decodeRPCResponse(t, badRec)
	if response.Error == nil || response.Error.Code != errorCodeHeaderMismatch {
		t.Fatalf("error = %+v, want HeaderMismatch", response.Error)
	}
}

type headerAnnotatedTool struct{}

func (t *headerAnnotatedTool) Name() string        { return "headers" }
func (t *headerAnnotatedTool) Description() string { return "validates mirrored headers" }
func (t *headerAnnotatedTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"region": map[string]any{"type": "string", "x-mcp-header": "Region"},
			"count":  map[string]any{"type": "integer", "x-mcp-header": "Count"},
			"nested": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean", "x-mcp-header": "Enabled"},
				},
			},
		},
	}
}
func (t *headerAnnotatedTool) Execute(map[string]any) (any, error) { return "ok", nil }

func TestStreamableHTTPToolParameterHeaders(t *testing.T) {
	h := newHandlerForTest(t)
	tool := &headerAnnotatedTool{}
	h.RegisterTool(tool)
	newRequest := func() *http.Request {
		req := newStreamableRequest(t, "tools/call", map[string]any{
			"name": tool.Name(),
			"arguments": map[string]any{
				"region": "東京",
				"count":  42,
				"nested": map[string]any{"enabled": true},
			},
		}, 1)
		req.Header.Set(headerName, tool.Name())
		req.Header.Set("Mcp-Param-Region", "=?base64?"+base64.StdEncoding.EncodeToString([]byte("東京"))+"?=")
		req.Header.Set("Mcp-Param-Count", "42.0")
		req.Header.Set("Mcp-Param-Enabled", "true")
		return req
	}

	rec := serveStreamable(t, h, newRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("valid headers status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	missing := newRequest()
	missing.Header.Del("Mcp-Param-Region")
	missingRec := serveStreamable(t, h, missing)
	if missingRec.Code != http.StatusBadRequest {
		t.Fatalf("missing header status = %d, want 400", missingRec.Code)
	}
	if response := decodeRPCResponse(t, missingRec); response.Error == nil || response.Error.Code != errorCodeHeaderMismatch {
		t.Fatalf("missing header error = %+v, want HeaderMismatch", response.Error)
	}

	duplicate := newRequest()
	duplicate.Header.Add("Mcp-Param-Count", "42")
	duplicateRec := serveStreamable(t, h, duplicate)
	if duplicateRec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate header status = %d, want 400", duplicateRec.Code)
	}
}

func TestStreamableHTTPRejectsMalformedTransportRequests(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{
			name: "missing SSE accept",
			mutate: func(r *http.Request) {
				r.Header.Set("Accept", "application/json")
			},
			wantStatus: http.StatusNotAcceptable,
		},
		{
			name: "wildcard does not replace explicit JSON",
			mutate: func(r *http.Request) {
				r.Header.Set("Accept", "*/*, text/event-stream")
			},
			wantStatus: http.StatusNotAcceptable,
		},
		{
			name: "wrong content type",
			mutate: func(r *http.Request) {
				r.Header.Set("Content-Type", "text/plain")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "multiple messages",
			mutate: func(r *http.Request) {
				data := `{"jsonrpc":"2.0","method":"ping"} {"jsonrpc":"2.0","method":"ping"}`
				r.Body = ioNopCloser(data)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "oversized body",
			mutate: func(r *http.Request) {
				data := `{"jsonrpc":"2.0","method":"ping","params":{"payload":"` + strings.Repeat("x", streamableHTTPMaxBody) + `"}}`
				r.Body = ioNopCloser(data)
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newStreamableRequest(t, "ping", nil, 1)
			tt.mutate(req)
			rec := serveStreamable(t, newHandlerForTest(t), req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestMCPOriginValidation(t *testing.T) {
	newRequest := func(origin string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"ping","id":1}`))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		return req
	}

	for _, origin := range []string{"", "http://example.com", "http://example.com:80"} {
		rec := serveStreamable(t, newHandlerForTest(t), newRequest(origin))
		if rec.Code != http.StatusOK {
			t.Errorf("origin %q status = %d, want 200", origin, rec.Code)
		}
	}
	for _, origin := range []string{"null", "https://example.com", "http://evil.example", "http://example.com/path"} {
		rec := serveStreamable(t, newHandlerForTest(t), newRequest(origin))
		if rec.Code != http.StatusForbidden {
			t.Errorf("origin %q status = %d, want 403", origin, rec.Code)
		}
	}

	h := newHandlerForTest(t)
	h.SetOriginValidator(func(r *http.Request) bool {
		return r.Header.Get("Origin") == "https://trusted.example"
	})
	rec := serveStreamable(t, h, newRequest("https://trusted.example"))
	if rec.Code != http.StatusOK {
		t.Fatalf("custom validator status = %d, want 200", rec.Code)
	}
}

type cancellationTool struct {
	canceled chan error
}

func (t *cancellationTool) Name() string           { return "cancel" }
func (t *cancellationTool) Description() string    { return "waits for cancellation" }
func (t *cancellationTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (t *cancellationTool) Execute(map[string]any) (any, error) {
	return nil, errors.New("context required")
}
func (t *cancellationTool) ExecuteWithContext(ctx context.Context, _ map[string]any) (any, error) {
	<-ctx.Done()
	t.canceled <- ctx.Err()
	return nil, ctx.Err()
}

func TestStreamableHTTPPropagatesRequestCancellation(t *testing.T) {
	h := newHandlerForTest(t)
	tool := &cancellationTool{canceled: make(chan error, 1)}
	h.RegisterTool(tool)
	req := newStreamableRequest(t, "tools/call", map[string]any{
		"name":      tool.Name(),
		"arguments": map[string]any{},
	}, 1)
	req.Header.Set(headerName, tool.Name())
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)
	rec := serveStreamable(t, h, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want JSON-RPC response; body=%s", rec.Code, rec.Body.String())
	}
	if err := <-tool.canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("tool context error = %v, want context.Canceled", err)
	}
}
