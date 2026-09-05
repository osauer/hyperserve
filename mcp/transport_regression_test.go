package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTransportsPreserveContext(t *testing.T) {
	type principalKey struct{}
	for _, current := range []bool{false, true} {
		h := newHandlerForTest(t)
		seen := make(chan context.Context, 1)
		stopped := make(chan struct{})
		h.RegisterTool(NewTypedTool("wait", "", func(ctx context.Context, _ struct{}) (string, error) {
			seen <- ctx
			<-ctx.Done()
			close(stopped)
			return "", ctx.Err()
		}))
		r := httptest.NewRequest("POST", "http://example.com/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"wait","arguments":{}}}`))
		r.Header.Set("Content-Type", "application/json")
		if current {
			r = newStreamableRequest(t, "tools/call", map[string]any{"name": "wait", "arguments": map[string]any{}}, 1)
			r.Header.Set(headerName, "wait")
		}
		ctx, cancel := context.WithCancel(context.WithValue(t.Context(), principalKey{}, "principal-123"))
		defer cancel()
		finished := make(chan struct{})
		go func() { h.ServeHTTP(httptest.NewRecorder(), r.WithContext(ctx)); close(finished) }()
		select {
		case toolCtx := <-seen:
			if got := toolCtx.Value(principalKey{}); got != "principal-123" {
				t.Errorf("current=%t principal=%v", current, got)
			}
		case <-time.After(time.Second):
			t.Fatal("tool did not start")
		}
		cancel()
		for _, done := range []chan struct{}{stopped, finished} {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("request cancellation did not finish tool and HTTP dispatch")
			}
		}
	}
}

func TestStdioTerminalErrorAndRecordRecovery(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		wantError   bool
		responses   int
	}{
		{"oversized", strings.Repeat("x", 1<<20+1), true, 0},
		{"malformed then valid", "bad json\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n", false, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerForTest(t)
			var output bytes.Buffer
			transport := NewStdioTransportWithIO(strings.NewReader(tt.input), &output, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := h.runStdioLoop(transport); (err != nil) != tt.wantError {
				t.Fatalf("error=%v", err)
			}
			if got := bytes.Count(output.Bytes(), []byte("\n")); got != tt.responses {
				t.Fatalf("responses=%d body=%s", got, &output)
			}
		})
	}
}

func TestTypedStructuredResults(t *testing.T) {
	type output struct {
		Answer int `json:"answer"`
	}
	for _, tt := range []struct {
		name      string
		tool      Tool
		want      string
		wantError bool
	}{
		{"object", NewTypedTool("test", "", func(context.Context, struct{}) (output, error) { return output{42}, nil }), `{"answer":42}`, false},
		{"scalar", NewTypedTool("test", "", func(context.Context, struct{}) (int, error) { return 42, nil }), `{"result":42}`, false},
		{"array", NewTypedTool("test", "", func(context.Context, struct{}) ([]output, error) { return []output{{42}}, nil }), `{"result":[{"answer":42}]}`, false},
		{"nil object", NewTypedTool("test", "", func(context.Context, struct{}) (*output, error) { return nil, nil }), "", true},
		{"nil scalar", NewTypedTool("test", "", func(context.Context, struct{}) (*int, error) { return nil, nil }), "", true},
		{"nil array", NewTypedTool("test", "", func(context.Context, struct{}) ([]output, error) { return nil, nil }), "", true},
		{"explicit error", NewTypedTool("test", "", func(context.Context, struct{}) (map[string]any, error) {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "application error"}}}, nil
		}), "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandlerForTest(t)
			h.RegisterTool(tt.tool)
			wire := h.ProcessRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{}}}`))
			var response struct {
				Result struct {
					Content    []struct{ Text string }
					Structured json.RawMessage `json:"structuredContent"`
					IsError    bool            `json:"isError"`
				}
			}
			if err := json.Unmarshal(wire, &response); err != nil {
				t.Fatal(err)
			}
			if response.Result.IsError != tt.wantError {
				t.Fatalf("wire=%s", wire)
			}
			if !tt.wantError && (string(response.Result.Structured) != tt.want || len(response.Result.Content) != 1 || response.Result.Content[0].Text != tt.want) {
				t.Fatalf("wire=%s", wire)
			}
		})
	}
}
