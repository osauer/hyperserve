# HyperServe Benchmarks

This directory contains performance benchmarks for HyperServe, measuring both raw performance and the overhead of various middleware configurations.

## Running Benchmarks

### Quick Start

```bash
# Run all benchmarks
go test -bench=. -benchmem ./benchmarks

# Run specific benchmark
go test -bench=BenchmarkBaseline -benchmem ./benchmarks

# Run with multiple iterations for stability
go test -bench=. -benchmem -count=5 ./benchmarks

# Save results
go test -bench=. -benchmem ./benchmarks > benchmarks/results.txt
```

### Profiling

```bash
# CPU profiling
go test -bench=BenchmarkSecureAPI -cpuprofile=cpu.prof ./benchmarks
go tool pprof cpu.prof

# Memory profiling
go test -bench=BenchmarkSecureAPI -memprofile=mem.prof ./benchmarks
go tool pprof mem.prof
```

## Benchmark Descriptions

### BenchmarkBaseline
Measures the raw performance of HyperServe with a minimal handler returning "OK". This establishes the baseline performance without any middleware.

### BenchmarkSecureAPI
Tests a real-world API configuration with a full security middleware stack:
- Request logging
- Trace ID generation
- Rate limiting
- Authentication
- Security headers
- JSON response

### BenchmarkIndividualMiddleware
Measures the isolated overhead of each middleware component:
- RequestLogger
- Trace
- Recovery
- RateLimit
- Auth

### BenchmarkStaticFile
Tests static file serving performance, including the os.Root security features in Go 1.24.

### BenchmarkJSON
Measures JSON encoding performance for API responses.

## Interpreting Results

Example output:
```
BenchmarkBaseline-8                      5000000       245 ns/op        96 B/op       2 allocs/op
BenchmarkSecureAPI-8                     1000000      1053 ns/op       512 B/op       8 allocs/op
BenchmarkIndividualMiddleware/RequestLogger-8   3000000       423 ns/op       128 B/op       3 allocs/op
```

- `ns/op`: Nanoseconds per operation (lower is better)
- `B/op`: Bytes allocated per operation (lower is better)
- `allocs/op`: Number of allocations per operation (lower is better)

### Performance Targets

- **Baseline**: < 300 ns/op (3.3M+ requests/sec)
- **Secure API**: < 1500 ns/op (650K+ requests/sec)
- **Static Files**: < 5000 ns/op for small files
- **Zero allocations** for static file serving path

## Middleware Overhead

From the individual middleware benchmarks, you can calculate the cost of each component:

| Middleware | Overhead | Allocations |
|------------|----------|-------------|
| RequestLogger | ~180 ns | 1 alloc |
| Trace | ~45 ns | 1 alloc |
| RateLimit | ~120 ns | 0 alloc |
| Auth | ~200 ns | 2 allocs |
| Headers | ~100 ns | 0 alloc |

Total secure stack overhead: ~650-800 ns (still sub-microsecond!)

## Comparison with Other Frameworks

To compare with other frameworks, you can run similar benchmarks:

```go
// Standard library
http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("OK"))
})

// Results: ~298 ns/op
```

HyperServe aims to be within 10-20% of the standard library while providing significantly more features out of the box.