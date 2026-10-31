package hyperserve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/auth"
	"github.com/osauer/hyperserve/v2/jsonrpc"
	"github.com/osauer/hyperserve/v2/mcp"
)

var benchmarkDurationSink time.Duration

// BenchmarkBaseline measures the raw performance of a minimal HyperServe handler
func BenchmarkBaseline(b *testing.B) {
	srv, err := New()
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
			srv, err := New(WithAddr(":0"))
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
	srv, err := New()
	if err != nil {
		b.Fatal(err)
	}
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
	srv, err := New()
	if err != nil {
		b.Fatal(err)
	}

	// Add typical security middleware stack
	srv.Use(RequestLoggerMiddleware)
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
	srv, err := New()
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
	srv, err := New()
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

// BenchmarkMCPJSONRPCOverHTTP measures JSON-RPC decoding, dispatch, encoding,
// and the in-process HTTP transport for a minimal MCP ping.
func BenchmarkMCPJSONRPCOverHTTP(b *testing.B) {
	srv, err := New(
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

	benchmarkMCPHTTP(b, srv.mcpHandler, requestData, nil)
}

// BenchmarkMCPInitializeHandshake measures the MCP initialization handshake performance
func BenchmarkMCPInitializeHandshake(b *testing.B) {
	srv, err := New(
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

	benchmarkMCPHTTP(b, srv.mcpHandler, requestData, nil)
}

// BenchmarkMCPWithMiddleware measures MCP performance with typical middleware stack
func BenchmarkMCPWithMiddleware(b *testing.B) {
	srv, err := New(
		WithAddr(":0"),
		WithMCPSupport("benchmark-server", "1.0.0"),
	)
	if err != nil {
		b.Fatal(err)
	}

	// Keep structured logging in the measured stack without making terminal I/O
	// part of the result or emitting one line per benchmark iteration.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv.Use(requestLoggerMiddleware(logger))
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

	handler := srv.middleware.applyToMux(srv.mux)
	benchmarkMCPHTTP(b, handler, requestData, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer benchmark-token")
	})
}

func testBearerAuthenticator(want string) auth.Authenticator {
	return auth.Bearer(auth.TokenVerifierFunc(func(_ context.Context, token string) (auth.Principal, error) {
		if token != want {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return auth.Principal{Issuer: "benchmark", Subject: "client"}, nil
	}))
}

// benchmarkMCPHTTP measures request construction plus in-process HTTP/MCP
// handling. A new request is mandatory on every iteration because Body is a
// stream; reusing it benchmarks EOF handling after the first request.
func benchmarkMCPHTTP(b *testing.B, handler http.Handler, requestData []byte, prepare func(*http.Request)) {
	b.Helper()
	serve := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
		req.Header.Set("Content-Type", "application/json")
		if prepare != nil {
			prepare(req)
		}
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
