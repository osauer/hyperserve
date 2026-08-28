package builtin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hyperserve "github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/jsonrpc"
	"github.com/osauer/hyperserve/v2/mcp"
	"github.com/osauer/hyperserve/v2/mcp/builtin"
)

var benchmarkToolResult any

type toolBenchmark struct {
	name string
	tool mcp.Tool
	args map[string]any
}

// BenchmarkMCPToolExecution measures the built-in tool implementation only.
// JSON-RPC decoding, MCP dispatch, and HTTP transport are deliberately absent.
func BenchmarkMCPToolExecution(b *testing.B) {
	tests := builtinToolBenchmarks(b)
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			result, err := tt.tool.Execute(tt.args)
			if err != nil {
				b.Fatalf("tool preflight: %v", err)
			}
			if result == nil {
				b.Fatal("tool preflight returned a nil result")
			}

			b.ReportAllocs()
			for b.Loop() {
				result, err = tt.tool.Execute(tt.args)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkToolResult = result
			}
		})
	}
}

// BenchmarkMCPToolCallHTTP measures a complete in-process tools/call request:
// fresh HTTP request construction, JSON-RPC decoding/encoding, MCP dispatch,
// and execution of the same built-in tools benchmarked directly above.
func BenchmarkMCPToolCallHTTP(b *testing.B) {
	tests := builtinToolBenchmarks(b)
	srv, err := hyperserve.New(
		hyperserve.WithAddr(":0"),
		hyperserve.WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}
	registered := make(map[string]bool)
	for _, tt := range tests {
		if registered[tt.tool.Name()] {
			continue
		}
		if err := srv.RegisterMCPTool(tt.tool); err != nil {
			b.Fatal(err)
		}
		registered[tt.tool.Name()] = true
	}

	for id, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			requestData := marshalBenchmarkRequest(b, map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name":      tt.tool.Name(),
					"arguments": tt.args,
				},
				"id": id + 1,
			})
			benchmarkMCPHTTP(b, srv.MCPHandler(), requestData)
		})
	}
}

// BenchmarkMCPResourceReadHTTP measures complete resources/read requests for
// the concrete built-in resources rather than an unregistered error path.
func BenchmarkMCPResourceReadHTTP(b *testing.B) {
	srv, err := hyperserve.New(
		hyperserve.WithAddr(":0"),
		hyperserve.WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}
	resources := []mcp.Resource{
		builtin.NewConfigResource(srv.Options()),
		builtin.NewMetricsResource(srv),
		builtin.NewSystemResource(),
		builtin.NewServerLogResource(100),
	}
	for _, resource := range resources {
		if err := srv.RegisterMCPResource(resource); err != nil {
			b.Fatal(err)
		}
	}

	for id, resource := range resources {
		b.Run(resource.Name(), func(b *testing.B) {
			requestData := marshalBenchmarkRequest(b, map[string]any{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params":  map[string]any{"uri": resource.URI()},
				"id":      id + 1,
			})
			benchmarkMCPHTTP(b, srv.MCPHandler(), requestData)
		})
	}
}

// BenchmarkMCPLargeToolCallHTTP measures a valid tools/call request with a
// large argument object, including protocol and HTTP overhead.
func BenchmarkMCPLargeToolCallHTTP(b *testing.B) {
	tool := builtin.NewCalculatorTool()
	srv, err := hyperserve.New(
		hyperserve.WithAddr(":0"),
		hyperserve.WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}
	if err := srv.RegisterMCPTool(tool); err != nil {
		b.Fatal(err)
	}

	largeArgs := make(map[string]any, 1003)
	for i := range 1000 {
		largeArgs[fmt.Sprintf("param_%d", i)] = float64(i) * 1.23456789
	}
	largeArgs["operation"] = "add"
	largeArgs["a"] = 10.0
	largeArgs["b"] = 20.0
	requestData := marshalBenchmarkRequest(b, map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool.Name(),
			"arguments": largeArgs,
		},
		"id": 1,
	})
	benchmarkMCPHTTP(b, srv.MCPHandler(), requestData)
}

func builtinToolBenchmarks(b *testing.B) []toolBenchmark {
	b.Helper()
	tempDir := b.TempDir()
	content := strings.Repeat("benchmark test content ", 100)
	if err := os.WriteFile(filepath.Join(tempDir, "benchmark.txt"), []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	fileRead, err := builtin.NewFileReadTool(tempDir)
	if err != nil {
		b.Fatal(err)
	}
	listDirectory, err := builtin.NewListDirectoryTool(tempDir)
	if err != nil {
		b.Fatal(err)
	}
	calculator := builtin.NewCalculatorTool()
	return []toolBenchmark{
		{name: "CalculatorMultiply", tool: calculator, args: map[string]any{"operation": "multiply", "a": 123.456, "b": 789.123}},
		{name: "FileRead", tool: fileRead, args: map[string]any{"path": "benchmark.txt"}},
		{name: "ListDirectory", tool: listDirectory, args: map[string]any{"path": "."}},
		{name: "CalculatorAdd", tool: calculator, args: map[string]any{"operation": "add", "a": 2.0, "b": 3.0}},
	}
}

func marshalBenchmarkRequest(b *testing.B, request map[string]any) []byte {
	b.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func benchmarkMCPHTTP(b *testing.B, handler http.Handler, requestData []byte) {
	b.Helper()
	serve := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	validateMCPBenchmarkResponse(b, serve())
	b.ReportAllocs()
	for b.Loop() {
		w := serve()
		b.StopTimer()
		validateMCPBenchmarkResponse(b, w)
		b.StartTimer()
	}
}

func validateMCPBenchmarkResponse(b *testing.B, w *httptest.ResponseRecorder) {
	b.Helper()
	if w.Code != http.StatusOK {
		b.Fatalf("MCP response status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var response jsonrpc.Response
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		b.Fatalf("decode MCP response: %v; body=%s", err, w.Body.String())
	}
	if response.Error != nil {
		b.Fatalf("MCP JSON-RPC error = %+v", response.Error)
	}
	if response.Result == nil {
		b.Fatal("MCP response has neither result nor error")
	}
	if result, ok := response.Result.(map[string]any); ok {
		if isError, _ := result["isError"].(bool); isError {
			b.Fatalf("MCP tool result reported isError: %s", w.Body.String())
		}
	}
}
