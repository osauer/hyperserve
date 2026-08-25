package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/auth"
	"github.com/osauer/hyperserve/v2/pkg/mcp"
)

var benchmarkDurationSink time.Duration

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

	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkConcurrentRequestPath measures the same request path under the
// parallel scheduling used by Go HTTP servers. The two sub-benchmarks keep a
// minimal handler separate from a representative production middleware stack
// so comparisons can distinguish framework routing from middleware cost.
func BenchmarkConcurrentRequestPath(b *testing.B) {
	tests := []struct {
		name       string
		middleware func(*Server, http.Handler) http.Handler
	}{
		{
			name: "Minimal",
			middleware: func(_ *Server, next http.Handler) http.Handler {
				return next
			},
		},
		{
			name: "MiddlewareStack",
			middleware: func(srv *Server, next http.Handler) http.Handler {
				return MetricsMiddleware(srv)(
					RecoveryMiddleware(
						HeadersMiddleware(srv.options)(next),
					),
				)
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			srv, err := NewServer(WithAddr(":0"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

			srv.HandleFunc("/benchmark", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("OK"))
			})
			handler := tt.middleware(srv, srv.mux)

			probe := httptest.NewRecorder()
			handler.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "/benchmark", nil))
			if probe.Code != http.StatusOK {
				b.Fatalf("probe status = %d, want %d", probe.Code, http.StatusOK)
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				req := httptest.NewRequest(http.MethodGet, "/benchmark", nil)
				for pb.Next() {
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, req)
				}
			})
		})
	}
}

// BenchmarkMiddlewareDispatch isolates HyperServe's middleware selection and
// composition from httptest.ResponseRecorder allocations. The middleware is
// deliberately inert: this benchmark measures dispatch machinery, not the
// work performed by a particular policy.
func BenchmarkMiddlewareDispatch(b *testing.B) {
	passThrough := Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	})
	stack := func(count int) MiddlewareStack {
		ret := make(MiddlewareStack, count)
		for i := range ret {
			ret[i] = passThrough
		}
		return ret
	}

	tests := []struct {
		name      string
		path      string
		configure func(*middlewareRegistry)
	}{
		{
			name:      "NoMiddleware",
			path:      "/api/admin/resource",
			configure: func(*middlewareRegistry) {},
		},
		{
			name: "Global3",
			path: "/api/admin/resource",
			configure: func(registry *middlewareRegistry) {
				registry.Add(globalMiddlewareRoute, stack(3))
			},
		},
		{
			name: "Nested6",
			path: "/api/admin/resource",
			configure: func(registry *middlewareRegistry) {
				registry.Add("/", stack(2))
				registry.Add("/api", stack(2))
				registry.Add("/api/admin", stack(2))
			},
		},
		{
			name: "PrefixMiss6",
			path: "/api/admin/resource",
			configure: func(registry *middlewareRegistry) {
				registry.Add("/other", stack(6))
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			registry := newMiddlewareRegistry(nil)
			tt.configure(registry)

			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler := registry.applyToMux(mux)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			writer := benchmarkResponseWriter{}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				handler.ServeHTTP(writer, req)
			}
		})
	}
}

// BenchmarkDefaultMiddlewareDispatch measures the real default metrics,
// WARN-filtered request logging, and recovery stack.
func BenchmarkDefaultMiddlewareDispatch(b *testing.B) {
	srv, err := NewServer()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(srv.stopCleanup)

	srv.HandleFunc("/minimal", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/minimal", nil)
	writer := benchmarkResponseWriter{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(writer, req)
	}
}

// BenchmarkDefaultMiddlewareComponents keeps routing, metrics, disabled
// request logging, recovery, and the compiled Run path independently visible.
// This prevents a future change from hiding one expensive layer inside the
// combined default-stack number.
func BenchmarkDefaultMiddlewareComponents(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	srv := &Server{logger: logger}
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux := http.NewServeMux()
	mux.Handle("/minimal", base)
	counters := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.totalRequests.Add(1)
		base.ServeHTTP(w, r)
		srv.totalResponseTime.Add(1)
	})
	timing := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		base.ServeHTTP(w, r)
		benchmarkDurationSink = time.Since(start)
	})

	tests := []struct {
		name    string
		handler http.Handler
	}{
		{name: "Handler", handler: base},
		{name: "ServeMuxExact", handler: mux},
		{name: "MetricsCounters", handler: counters},
		{name: "MetricsTiming", handler: timing},
		{name: "Metrics", handler: MetricsMiddleware(srv)(base)},
		{name: "RequestLoggerDisabled", handler: requestLoggerMiddleware(logger)(base)},
		{name: "Recovery", handler: recoveryMiddleware(logger)(base)},
		{
			name: "DefaultStack",
			handler: applyMiddlewareStack(
				defaultMiddleware(srv),
				base,
			),
		},
		{
			name: "CompiledDefaultMux",
			handler: newMiddlewareRegistry(
				defaultMiddleware(srv),
			).compile(mux),
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, "/minimal", nil)
			writer := benchmarkResponseWriter{}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				tt.handler.ServeHTTP(writer, req)
			}
		})
	}
}

type benchmarkResponseWriter struct{}

func (benchmarkResponseWriter) Header() http.Header         { return nil }
func (benchmarkResponseWriter) WriteHeader(int)             {}
func (benchmarkResponseWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkAuthenticatedAPI measures a typical protected API middleware chain.
func BenchmarkAuthenticatedAPI(b *testing.B) {
	srv, err := NewServer()
	if err != nil {
		b.Fatal(err)
	}

	// Add typical security middleware stack
	srv.Use(RequestLoggerMiddleware)
	srv.UsePrefix("/api", RateLimitMiddleware(srv))
	srv.UsePrefix("/api", auth.Require(testBearerAuthenticator("test-token")))
	srv.Use(HeadersMiddleware(srv.options))

	srv.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","data":{"id":1,"name":"test"}}`))
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Authorization", "Bearer test-token")

	b.ReportAllocs()
	handler := srv.Handler()
	for b.Loop() {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

// BenchmarkIndividualMiddleware measures the overhead of each middleware separately
func BenchmarkIndividualMiddleware(b *testing.B) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	tests := []struct {
		name       string
		middleware Middleware
		setup      func(*http.Request)
	}{
		{
			name:       "RequestLogger",
			middleware: RequestLoggerMiddleware,
		},
		{
			name:       "Recovery",
			middleware: RecoveryMiddleware,
		},
		{
			name: "RateLimit",
			middleware: func(next http.Handler) http.Handler {
				srv, _ := NewServer()
				return RateLimitMiddleware(srv)(next)
			},
		},
		{
			name:       "Auth",
			middleware: auth.Require(testBearerAuthenticator("test-token")),
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
	srv.options.StaticDir = b.TempDir()
	testFile := []byte("This is a test file for benchmarking static file serving performance.")
	if err := writeFile(srv.options.StaticDir+"/test.txt", testFile); err != nil {
		b.Fatal(err)
	}

	if err := srv.HandleStatic("/static/"); err != nil {
		b.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/static/test.txt", nil)

	b.ReportAllocs()
	for b.Loop() {
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
		Status string         `json:"status"`
		Data   map[string]any `json:"data"`
	}

	srv.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Status: "success",
			Data: map[string]any{
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

	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, req)
	}
}

// BenchmarkMCPJSONRPCProcessing measures raw JSON-RPC request processing performance
func BenchmarkMCPJSONRPCProcessing(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Simple ping request
	request := map[string]any{
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

	b.ReportAllocs()
	for b.Loop() {
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
		WithMCPSupport("benchmark-server", "1.0.0"),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		b.Fatal(err)
	}

	tests := []struct {
		name    string
		request map[string]any
	}{
		{
			name: "Calculator",
			request: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": "mcp__hyperserve__calculator",
					"arguments": map[string]any{
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
			request: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": "mcp__hyperserve__read_file",
					"arguments": map[string]any{
						"path": "benchmark.txt",
					},
				},
				"id": 2,
			},
		},
		{
			name: "ListDirectory",
			request: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": "mcp__hyperserve__list_directory",
					"arguments": map[string]any{
						"path": ".",
					},
				},
				"id": 3,
			},
		},
		{
			name: "Calculator",
			request: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": "mcp__hyperserve__calculator",
					"arguments": map[string]any{
						"operation": "add",
						"a":         2.0,
						"b":         3.0,
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
		WithMCPSupport("benchmark-server", "1.0.0"),
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
			request := map[string]any{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params": map[string]any{
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
		WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
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

	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		srv.mcpHandler.ServeHTTP(w, req)
	}
}

// BenchmarkMCPWithMiddleware measures MCP performance with typical middleware stack
func BenchmarkMCPWithMiddleware(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Add middleware stack
	srv.Use(RequestLoggerMiddleware)
	srv.UsePrefix("/mcp", RateLimitMiddleware(srv))
	srv.UsePrefix("/mcp", auth.Require(testBearerAuthenticator("benchmark-token")))

	request := map[string]any{
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

	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		handler := srv.middleware.applyToMux(srv.mux)
		handler.ServeHTTP(w, req)
	}
}

func testBearerAuthenticator(want string) auth.Authenticator {
	return auth.Bearer(auth.TokenVerifierFunc(func(_ context.Context, token string) (auth.Principal, error) {
		if token != want {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return auth.Principal{Issuer: "benchmark", Subject: "client"}, nil
	}))
}

// BenchmarkMCPLargePayload measures performance with large JSON-RPC payloads
func BenchmarkMCPLargePayload(b *testing.B) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Create large arguments for calculator (realistic but large payload)
	largeArgs := make(map[string]any)
	for i := range 1000 {
		largeArgs[fmt.Sprintf("param_%d", i)] = float64(i) * 1.23456789
	}
	largeArgs["operation"] = "add"
	largeArgs["a"] = 10.0
	largeArgs["b"] = 20.0

	request := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
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

	b.ReportAllocs()
	for b.Loop() {
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
