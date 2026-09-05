package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkPrefixBreadth(b *testing.B) {
	for _, n := range []int{0, 8, 64, 512, 2048} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			registry := newMiddlewareRegistry(nil)
			mw := Middleware(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
			})
			for i := range n {
				registry.Add(fmt.Sprintf("/prefix%05d", i), MiddlewareStack{mw})
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
			h := registry.compile(mux)
			r := httptest.NewRequest("GET", "/unrelated", nil)
			w := benchmarkResponseWriter{}
			b.ReportAllocs()
			for b.Loop() {
				h.ServeHTTP(w, r)
			}
		})
	}
}
func BenchmarkMetricsParallel(b *testing.B) {
	for _, mode := range []string{"plain", "shared", "per_worker_control"} {
		b.Run(mode, func(b *testing.B) {
			base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
			shared := MetricsMiddleware(&Server{})(base)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var h http.Handler = base
				if mode == "shared" {
					h = shared
				}
				if mode == "per_worker_control" {
					h = MetricsMiddleware(&Server{})(base)
				}
				r := httptest.NewRequest("GET", "/", nil)
				w := benchmarkResponseWriter{}
				for pb.Next() {
					h.ServeHTTP(w, r)
				}
			})
		})
	}
}

func BenchmarkMetricsRead(b *testing.B) {
	var srv Server
	srv.AddMetrics(1, 1)
	var requests uint64
	var elapsed int64
	b.ReportAllocs()
	for b.Loop() {
		requests = srv.TotalRequests()
		elapsed = srv.TotalResponseTime()
	}
	if requests != 1 || elapsed != 1 {
		b.Fatal("incorrect totals")
	}
}
