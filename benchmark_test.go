package hyperserve

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
)

// BenchmarkBaseline measures the raw performance of a minimal HyperServe handler
func BenchmarkBaseline(b *testing.B) {
	srv, err := NewServer()
	if err != nil {
		b.Fatal(err)
	}

	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkSecureAPI measures a typical secure API setup with multiple middleware
func BenchmarkSecureAPI(b *testing.B) {
	srv, err := NewServer(
		WithAuthTokenValidator(func(token string) (bool, error) {
			return token == "test-token", nil
		}),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Add typical security middleware stack
	srv.AddMiddleware("*", RequestLoggerMiddleware)
	srv.AddMiddleware("*", TraceMiddleware)
	srv.AddMiddleware("/api", RateLimitMiddleware(srv))
	srv.AddMiddleware("/api", AuthMiddleware(srv.Options))
	srv.AddMiddleware("*", HeadersMiddleware(srv.Options))

	srv.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","data":{"id":1,"name":"test"}}`))
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkIndividualMiddleware measures the overhead of each middleware separately
func BenchmarkIndividualMiddleware(b *testing.B) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	tests := []struct {
		name       string
		middleware func(http.Handler) http.HandlerFunc
		setup      func(*http.Request)
	}{
		{
			name:       "RequestLogger",
			middleware: RequestLoggerMiddleware,
		},
		{
			name:       "Trace",
			middleware: TraceMiddleware,
		},
		{
			name:       "Recovery",
			middleware: RecoveryMiddleware,
		},
		{
			name: "RateLimit",
			middleware: func(next http.Handler) http.HandlerFunc {
				srv, _ := NewServer()
				return RateLimitMiddleware(srv)(next)
			},
		},
		{
			name: "Auth",
			middleware: func(next http.Handler) http.HandlerFunc {
				opts := &ServerOptions{
					AuthTokenValidatorFunc: func(token string) (bool, error) {
						return token == "test-token", nil
					},
				}
				return AuthMiddleware(opts)(next)
			},
			setup: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer test-token")
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			handler := tt.middleware(baseHandler)
			req := httptest.NewRequest("GET", "/", nil)
			if tt.setup != nil {
				tt.setup(req)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkStaticFile measures static file serving performance
func BenchmarkStaticFile(b *testing.B) {
	srv, err := NewServer()
	if err != nil {
		b.Fatal(err)
	}

	// Create a temporary static file
	srv.Options.StaticDir = b.TempDir()
	testFile := []byte("This is a test file for benchmarking static file serving performance.")
	if err := writeFile(srv.Options.StaticDir+"/test.txt", testFile); err != nil {
		b.Fatal(err)
	}

	srv.HandleStatic("/static/")
	req := httptest.NewRequest("GET", "/static/test.txt", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkJSON measures JSON response performance
func BenchmarkJSON(b *testing.B) {
	srv, err := NewServer()
	if err != nil {
		b.Fatal(err)
	}

	type Response struct {
		Status string                 `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}

	srv.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Status: "success",
			Data: map[string]interface{}{
				"id":     12345,
				"name":   "Test User",
				"email":  "test@example.com",
				"active": true,
				"score":  98.5,
				"tags":   []string{"premium", "verified"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	req := httptest.NewRequest("GET", "/json", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkMCPJSONRPCProcessing measures raw JSON-RPC request processing performance
func BenchmarkMCPJSONRPCProcessing(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Simple ping request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      1,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mcpHandler.ServeHTTP(w, req)
	}
}

// BenchmarkMCPToolExecution measures tool execution performance for different tools
func BenchmarkMCPToolExecution(b *testing.B) {
	// Create temporary directory for file tools
	tempDir := b.TempDir()
	testFile := filepath.Join(tempDir, "benchmark.txt")
	testContent := strings.Repeat("benchmark test content ", 100) // ~2KB of text
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		b.Fatal(err)
	}

	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		b.Fatal(err)
	}

	tests := []struct {
		name    string
		request map[string]interface{}
	}{
		{
			name: "Calculator",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "calculator",
					"arguments": map[string]interface{}{
						"operation": "multiply",
						"a":         123.456,
						"b":         789.123,
					},
				},
				"id": 1,
			},
		},
		{
			name: "FileRead",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",  
				"params": map[string]interface{}{
					"name": "read_file",
					"arguments": map[string]interface{}{
						"path": "benchmark.txt",
					},
				},
				"id": 2,
			},
		},
		{
			name: "ListDirectory",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "list_directory",
					"arguments": map[string]interface{}{
						"path": ".",
					},
				},
				"id": 3,
			},
		},
		{
			name: "HTTPRequest",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "http_request",
					"arguments": map[string]interface{}{
						"url":    "https://httpbin.org/json",
						"method": "GET",
					},
				},
				"id": 4,
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			requestData, err := json.Marshal(tt.request)
			if err != nil {
				b.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
			req.Header.Set("Content-Type", "application/json")

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				srv.mcpHandler.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkMCPResourceAccess measures resource access performance
func BenchmarkMCPResourceAccess(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPServerInfo("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Generate some metrics by making requests
	srv.totalRequests.Store(1000)
	srv.totalResponseTime.Store(50000000) // 50ms in nanoseconds

	tests := []struct {
		name string
		uri  string
	}{
		{
			name: "ConfigResource",
			uri:  "config://server/options",
		},
		{
			name: "MetricsResource", 
			uri:  "metrics://server/stats",
		},
		{
			name: "SystemResource",
			uri:  "system://runtime/info",
		},
		{
			name: "LogsResource",
			uri:  "logs://server/recent",
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params": map[string]interface{}{
					"uri": tt.uri,
				},
				"id": 1,
			}

			requestData, err := json.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
			req.Header.Set("Content-Type", "application/json")

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				srv.mcpHandler.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkMCPInitializeHandshake measures the MCP initialization handshake performance
func BenchmarkMCPInitializeHandshake(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPServerInfo("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": MCPVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "benchmark-client",
				"version": "1.0.0",
			},
		},
		"id": 1,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mcpHandler.ServeHTTP(w, req)
	}
}

// BenchmarkMCPWithMiddleware measures MCP performance with typical middleware stack
func BenchmarkMCPWithMiddleware(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithAuthTokenValidator(func(token string) (bool, error) {
			return token == "benchmark-token", nil
		}),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Add middleware stack
	srv.AddMiddleware("", RequestLoggerMiddleware)
	srv.AddMiddleware("", TraceMiddleware)
	srv.AddMiddleware("/mcp", RateLimitMiddleware(srv))
	srv.AddMiddleware("/mcp", AuthMiddleware(srv.Options))

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer benchmark-token")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler := srv.middleware.WrapHandler(srv.mux)
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkMCPLargePayload measures performance with large JSON-RPC payloads
func BenchmarkMCPLargePayload(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Create large arguments for calculator (realistic but large payload)
	largeArgs := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largeArgs[fmt.Sprintf("param_%d", i)] = float64(i) * 1.23456789
	}
	largeArgs["operation"] = "add"
	largeArgs["a"] = 10.0
	largeArgs["b"] = 20.0

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "calculator",
			"arguments": largeArgs,
		},
		"id": 1,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		b.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		srv.mcpHandler.ServeHTTP(w, req)
	}
}

// Helper function to write files
func writeFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
