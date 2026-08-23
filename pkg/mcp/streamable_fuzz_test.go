package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func FuzzMCPStreamableHTTP(f *testing.F) {
	f.Add(
		`{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":1}`,
		"application/json, text/event-stream",
		"application/json",
		"2026-07-28",
		"tools/list",
	)
	f.Add(
		`{"jsonrpc":"2.0","method":"tools/list","method":"tools/call","id":null}`,
		"application/json;q=bogus",
		"text/plain",
		"2026-07-28, 2025-11-25",
		"tools/list, tools/call",
	)

	f.Fuzz(func(t *testing.T, body, accept, contentType, protocolVersion, method string) {
		h := NewHandler(ServerInfo{Name: "fuzz", Version: "0"})
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // A generated valid listen request must not leave the fuzz call hanging.
		req := httptest.NewRequest(http.MethodPost, "http://example.test/mcp", strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Accept", accept)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set(headerProtocolVersion, protocolVersion)
		req.Header.Set(headerMethod, method)
		h.ServeHTTP(httptest.NewRecorder(), req)

		// Framing always receives marshaled JSON internally. Arbitrary strings
		// therefore must remain one data line and cannot inject SSE fields.
		data, err := json.Marshal(rpcNotification{
			JSONRPC: "2.0",
			Method:  "notifications/resources/updated",
			Params:  map[string]any{"uri": body},
		})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		stream := newStreamableSSE(context.Background(), recorder, 1, ServerInfo{}, nil, nil, time.Second, time.Second)
		if err := stream.writeEvent(data); err != nil {
			t.Fatal(err)
		}
		want := "event: message\ndata: " + string(data) + "\n\n"
		if got := recorder.Body.String(); got != want {
			t.Fatalf("SSE frame = %q, want %q", got, want)
		}
	})
}
