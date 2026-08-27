package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// recordingTool returns a fixed result and tracks the arguments it was called
// with. Used by tools/call dispatch tests.
type recordingTool struct {
	stubTool
	lastArgs map[string]any
	result   any
	err      error
}

type panickingTool struct {
	stubTool
	panicValue any
}

func (t *panickingTool) Execute(map[string]any) (any, error) {
	panic(t.panicValue)
}

type contextPanickingTool struct {
	panickingTool
}

func (t *contextPanickingTool) ExecuteWithContext(context.Context, map[string]any) (any, error) {
	panic(t.panicValue)
}

func (t *recordingTool) Execute(params map[string]any) (any, error) {
	t.lastArgs = params
	return t.result, t.err
}

func newHandlerForTest(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(ServerInfo{Name: "test", Version: "0.0.1"})
}

func TestHandlerToolPanicPreservesIdentity(t *testing.T) {
	tests := []struct {
		name string
		tool func(any) Tool
	}{
		{
			name: "plain Tool",
			tool: func(value any) Tool {
				return &panickingTool{stubTool: stubTool{name: "boom"}, panicValue: value}
			},
		},
		{
			name: "ToolWithContext",
			tool: func(value any) Tool {
				return &contextPanickingTool{panickingTool: panickingTool{
					stubTool:   stubTool{name: "boom"},
					panicValue: value,
				}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panicValue := &struct{ source string }{source: tt.name}
			h := newHandlerForTest(t)
			h.RegisterTool(tt.tool(panicValue))

			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_, _ = h.handleToolsCallContext(context.Background(), map[string]any{
					"name":      "boom",
					"arguments": map[string]any{},
				})
			}()
			if recovered != panicValue {
				t.Fatalf("recovered panic = %#v, want original value %#v", recovered, panicValue)
			}
		})
	}
}

func TestHandlerRegisterToolAndLookup(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterTool(&stubTool{name: "calc"})

	if h.ToolCount() != 1 {
		t.Errorf("ToolCount = %d, want 1", h.ToolCount())
	}
	if !h.HasTool("calc") {
		t.Error("HasTool(calc) = false, want true")
	}
	if tool, ok := h.Tool("calc"); !ok || tool.Name() != "calc" {
		t.Errorf("Tool(calc) = (%v, %v), want (calc, true)", tool, ok)
	}
	if _, ok := h.Tool("missing"); ok {
		t.Error("Tool(missing) returned ok=true for unknown tool")
	}
	if got := h.RegisteredTools(); !slices.Contains(got, "calc") {
		t.Errorf("RegisteredTools = %v, want to contain calc", got)
	}
}

func TestHandlerRegisterToolInNamespace(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterToolInNamespace(&stubTool{name: "ping"}, "alpha")

	want := "mcp__alpha__ping"
	if !h.HasTool(want) {
		t.Errorf("HasTool(%q) = false; registered names = %v", want, h.RegisteredTools())
	}
	// Empty namespace falls back to the server name (cmp.Or branch).
	h.RegisterToolInNamespace(&stubTool{name: "echo"}, "")
	fallback := "mcp__test__echo" // server name set in newHandlerForTest
	if !h.HasTool(fallback) {
		t.Errorf("HasTool(%q) = false; expected fallback to server name", fallback)
	}
}

func TestHandlerRegisterResourceVariants(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterResource(&stubResource{uri: "config://x"})

	if h.ResourceCount() != 1 {
		t.Errorf("ResourceCount = %d, want 1", h.ResourceCount())
	}
	if !h.HasResource("config://x") {
		t.Error("HasResource(config://x) = false")
	}

	h.RegisterResourceInNamespace(&stubResource{uri: "config://y"}, "alpha")
	want := "mcp__alpha__config://y"
	if !h.HasResource(want) {
		t.Errorf("HasResource(%q) = false; registered = %v", want, h.RegisteredResources())
	}

	// Empty namespace is rejected (logs error and does not register).
	prevCount := h.ResourceCount()
	h.RegisterResourceInNamespace(&stubResource{uri: "config://z"}, "")
	if h.ResourceCount() != prevCount {
		t.Errorf("ResourceCount changed after empty-namespace register; got %d, want %d",
			h.ResourceCount(), prevCount)
	}
}

func TestHandlerRegisterNamespace(t *testing.T) {
	h := newHandlerForTest(t)
	err := h.RegisterNamespace("dev",
		WithNamespaceTools(&stubTool{name: "diag"}),
		WithNamespaceResources(&stubResource{uri: "status://now"}),
	)
	if err != nil {
		t.Fatalf("RegisterNamespace returned error: %v", err)
	}
	if !h.HasNamespace("dev") {
		t.Error("HasNamespace(dev) = false")
	}
	if !h.HasTool("mcp__dev__diag") {
		t.Errorf("namespace tool not prefixed; registered = %v", h.RegisteredTools())
	}
	if !h.HasResource("mcp__dev__status://now") {
		t.Errorf("namespace resource not prefixed; registered = %v", h.RegisteredResources())
	}

	// Empty namespace name is an error.
	if err := h.RegisterNamespace(""); err == nil {
		t.Error("RegisterNamespace(\"\") = nil, want error")
	}
}

func TestHandlerCapabilities(t *testing.T) {
	h := newHandlerForTest(t)
	caps := h.Capabilities()
	if caps.Tools == nil || caps.Resources == nil {
		t.Errorf("Capabilities missing Tools or Resources: %+v", caps)
	}
	if caps.SSE != nil {
		t.Errorf("Capabilities.SSE = %+v, want nil by default", caps.SSE)
	}
	h.SetLegacyRoutedSSEEnabled(true)
	if caps = h.Capabilities(); caps.SSE == nil || !caps.SSE.Enabled {
		t.Errorf("Capabilities.SSE = %+v after opt-in, want enabled", caps.SSE)
	}
}

func TestServeHTTPRejectsGETByDefault(t *testing.T) {
	h := newHandlerForTest(t)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Errorf("Allow = %q, want POST", got)
	}
}

func TestServeHTTPReturnsLegacyJSONStatusOnGETWhenEnabled(t *testing.T) {
	h := newHandlerForTest(t)
	h.SetLegacyRoutedSSEEnabled(true)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if got["status"] != "ready" {
		t.Errorf("status = %v, want \"ready\"", got["status"])
	}
}

// TestServeHTTPDispatchToolsCall exercises the POST → JSON-RPC dispatch path
// through ServeHTTP, the place a real MCP client lands. This is the largest
// blind spot the v0.32.0 coverage report flagged.
func TestServeHTTPDispatchToolsCall(t *testing.T) {
	h := newHandlerForTest(t)
	tool := &recordingTool{name: "echo", result: map[string]any{"out": "ok"}}
	h.RegisterTool(tool)

	body := []byte(`{
		"jsonrpc":"2.0",
		"method":"tools/call",
		"params":{"name":"echo","arguments":{"in":"hi"}},
		"id":7
	}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body is not JSON: %v body=%s", err, rec.Body.String())
	}
	if resp["error"] != nil {
		t.Fatalf("unexpected error in response: %v", resp["error"])
	}
	if id, _ := resp["id"].(float64); int(id) != 7 {
		t.Errorf("id = %v, want 7", resp["id"])
	}
	if tool.lastArgs["in"] != "hi" {
		t.Errorf("tool received args = %v, want in=hi", tool.lastArgs)
	}
}

// TestServeHTTPUnknownMethodReturnsRPCError exercises the JSON-RPC error path
// where the method is not registered.
func TestServeHTTPUnknownMethodReturnsRPCError(t *testing.T) {
	h := newHandlerForTest(t)
	body := []byte(`{"jsonrpc":"2.0","method":"does_not_exist","id":1}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"error\"") {
		t.Errorf("expected JSON-RPC error envelope; got %s", rec.Body.String())
	}
}

func TestCompatibilityHTTPRejectsOversizedBody(t *testing.T) {
	h := newHandlerForTest(t)
	body := `{"jsonrpc":"2.0","method":"ping","params":{"payload":"` + strings.Repeat("x", mcpHTTPMaxBody) + `"},"id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
}

func TestLegacyRoutedSSERejectsOversizedBody(t *testing.T) {
	h := newHandlerForTest(t)
	h.SetLegacyRoutedSSEEnabled(true)
	streamRecorder := httptest.NewRecorder()
	client := newSSEClient("client", "binding", streamRecorder, streamRecorder)
	if _, ok := h.sseManager.admitClient("client", client); !ok {
		t.Fatal("failed to admit test SSE client")
	}
	defer h.sseManager.CloseAll()

	body := `{"jsonrpc":"2.0","method":"ping","params":{"payload":"` + strings.Repeat("x", mcpHTTPMaxBody) + `"},"id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-SSE-Client-ID", "client")
	req.Header.Set("X-SSE-Binding", "binding")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
}

func TestIsJSONAccepted(t *testing.T) {
	cases := map[string]bool{
		"":                                    false,
		"application/json":                    true,
		"APPLICATION/JSON":                    true,
		"*/*":                                 true,
		"text/html, application/json;q=0.9":   true,
		"text/html":                           false,
		"application/*":                       true,
		"text/event-stream, application/json": true,
	}
	for accept, want := range cases {
		got := isJSONAccepted(accept)
		if got != want {
			t.Errorf("isJSONAccepted(%q) = %v, want %v", accept, got, want)
		}
	}
}
