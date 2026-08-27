package ratelimit

import (
	"net/http"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/time/rate"
)

var benchmarkEntrySink *clientEntry

// BenchmarkEntryFootprint tracks the core allocation behind each bounded map
// entry. The reported metric excludes the variable key string and map-bucket
// overhead, so it is a stable lower bound rather than a total heap estimate.
func BenchmarkEntryFootprint(b *testing.B) {
	coreBytes := unsafe.Sizeof(clientEntry{}) + unsafe.Sizeof(rate.Limiter{})
	b.ReportAllocs()

	for b.Loop() {
		benchmarkEntrySink = &clientEntry{
			bucket: rate.NewLimiter(20, 40),
		}
	}
	b.ReportMetric(float64(coreBytes), "core-bytes/entry")
	b.ReportMetric(float64(coreBytes*defaultMaxClients)/(1024*1024), "default-core-MiB")
	runtime.KeepAlive(benchmarkEntrySink)
}

func BenchmarkMiddlewareExistingClient(b *testing.B) {
	gate, err := New(Config{
		RequestsPerSecond: 1e12,
		Burst:             1_000_000_000,
		ClientKey: func(*http.Request) (string, error) {
			return "benchmark-client", nil
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	handler := gate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	if err != nil {
		b.Fatal(err)
	}
	writer := benchmarkResponseWriter{header: make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		handler.ServeHTTP(writer, request)
	}
}

type benchmarkResponseWriter struct {
	header http.Header
}

func (w benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (benchmarkResponseWriter) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (benchmarkResponseWriter) WriteHeader(int) {}
