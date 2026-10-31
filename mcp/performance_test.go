package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/osauer/hyperserve/v2/jsonrpc"
)

type performanceArguments struct {
	Items map[string]int `json:"items"`
}
type performanceOutput struct {
	OK bool `json:"ok"`
}

func performancePayload(n int, current bool) []byte {
	items := make(map[string]int, n)
	for i := range n {
		items[fmt.Sprintf("key%05d", i)] = i
	}
	params := map[string]any{"name": "performance", "arguments": map[string]any{"items": items}}
	if current {
		params["_meta"] = map[string]any{protocolVersionMetaKey: StreamableHTTPProtocolVersion}
	}
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": params})
	if err != nil {
		panic(err)
	}
	return raw
}
func BenchmarkMCP(b *testing.B) {
	for _, n := range []int{0, 1000, 10000} {
		for _, current := range []bool{false, true} {
			b.Run(fmt.Sprintf("fields_%d/current_%t", n, current), func(b *testing.B) {
				h := NewHandler(ServerInfo{Name: "performance", Version: "1"})
				h.RegisterTool(NewTypedTool("performance", "", func(context.Context, performanceArguments) (performanceOutput, error) {
					return performanceOutput{true}, nil
				}))
				raw := performancePayload(n, current)
				r := httptest.NewRequest("POST", "http://example.com/mcp", nil)
				r.Header.Set("Content-Type", "application/json")
				if current {
					r.Header.Set("Accept", "application/json, text/event-stream")
					r.Header.Set(headerProtocolVersion, StreamableHTTPProtocolVersion)
					r.Header.Set(headerMethod, "tools/call")
					r.Header.Set("Mcp-Name", "performance")
				}
				serve := func() *httptest.ResponseRecorder {
					r.Body = io.NopCloser(bytes.NewReader(raw))
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					return w
				}
				validate := func(w *httptest.ResponseRecorder) {
					var response jsonrpc.Response
					if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || response.Error != nil {
						b.Fatalf("invalid response: %d %s", w.Code, w.Body.String())
					}
					if bytes.Contains(w.Body.Bytes(), []byte(`"isError":true`)) {
						b.Fatal(w.Body.String())
					}
				}
				validate(serve())
				b.ReportAllocs()
				b.SetBytes(int64(len(raw)))
				var last *httptest.ResponseRecorder
				for b.Loop() {
					last = serve()
				}
				validate(last)
			})
		}
	}
}
func BenchmarkMCPParsing(b *testing.B) {
	raw := performancePayload(10000, true)
	for _, phase := range []string{"unique_keys", "request_decode", "envelope_decode"} {
		b.Run(phase, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				var err error
				switch phase {
				case "unique_keys":
					err = validateUniqueJSON(raw)
				case "request_decode":
					var r jsonrpc.Request
					err = json.Unmarshal(raw, &r)
				case "envelope_decode":
					var e map[string]json.RawMessage
					err = json.Unmarshal(raw, &e)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
