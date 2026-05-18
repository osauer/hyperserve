package jsonrpc

import (
	"log/slog"
	"testing"
)

// FuzzJSONRPCParse feeds arbitrary bytes into the JSON-RPC engine's request
// path. The goal isn't to find protocol bugs — it's to ensure the parser
// never panics on attacker-controlled input. Any panic is a real bug.
func FuzzJSONRPCParse(f *testing.F) {
	// Seed corpus: shapes the parser should already handle.
	seeds := [][]byte{
		[]byte(`{"jsonrpc":"2.0","method":"ping","id":1}`),
		[]byte(`{"jsonrpc":"2.0","method":"tools/list","id":"a","params":{}}`),
		[]byte(`{"jsonrpc":"2.0","method":"x","params":[1,2,3]}`),
		[]byte(`[]`),
		[]byte(`{}`),
		[]byte(``),
		[]byte(`null`),
		[]byte(`{"jsonrpc":"2.0","id":null}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	engine := NewEngine(slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	engine.RegisterMethod("ping", func(_ any) (any, error) { return "pong", nil })
	engine.RegisterMethod("echo", func(p any) (any, error) { return p, nil })

	f.Fuzz(func(t *testing.T, data []byte) {
		// Don't blow up if ProcessRequest happens to dereference nil.
		// Any panic is a bug — let the framework surface it.
		_ = engine.ProcessRequest(data)
	})
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
