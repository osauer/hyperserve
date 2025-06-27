package hyperserve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		Status string            `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}

	srv.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Status: "success",
			Data: map[string]interface{}{
				"id":        12345,
				"name":      "Test User",
				"email":     "test@example.com",
				"active":    true,
				"score":     98.5,
				"tags":      []string{"premium", "verified"},
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