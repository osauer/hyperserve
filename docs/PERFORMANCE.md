# HyperServe Performance Guide

## Overview

HyperServe is designed for minimal overhead while providing production-ready features. Performance characteristics are hardware-dependent, but the relative costs and efficiency metrics provide useful guidance.

## Performance Characteristics

### Memory Efficiency

HyperServe maintains a small memory footprint:

| Component | Memory per Request | Allocations | Notes |
|-----------|-------------------|-------------|--------|
| Baseline Handler | ~1 KB | 10 | Minimal HTTP processing |
| With Security Stack | ~1 KB | 10 | No additional allocations |
| Static File (small) | ~2.5 KB | 31 | Room for optimization |
| JSON Response | ~2 KB | 27 | Includes encoding buffer |

### Relative Performance

Middleware overhead as percentage of baseline:

| Middleware | Overhead | Impact |
|------------|----------|---------|
| Recovery | -9% | Actually improves performance via optimized path |
| Auth | +21% | Token validation with timing-safe comparison |
| Trace | +33% | UUID generation and context propagation |
| RateLimit | +52% | Map lookup and token bucket algorithm |
| RequestLogger | +254% | I/O bound, structured logging |

**Key insight**: Full security stack (excluding logging) adds only 10-30% overhead compared to baseline.

### Architectural Efficiency

What makes HyperServe efficient:

1. **Zero-copy operations** where possible
2. **Pre-allocated buffers** for common paths
3. **Minimal interface conversions**
4. **Efficient middleware chaining** without reflection
5. **Swiss Tables (Go 1.24+)** for O(1) rate limiter lookups

### Measurement Notes

> **Keep metrics current**: The tables in this section are directional and depend on hardware, workload, and Go version. Before publishing new numbers, rerun `go test -bench=. -benchmem ./...`, capture the hardware/Go version used, and update the tables accordingly.

## Performance Tips

### 1. Minimize Allocations

HyperServe is already optimized for minimal allocations (10 per request baseline). To maintain this:

```go
// ❌ Avoid creating new objects in hot paths
func handler(w http.ResponseWriter, r *http.Request) {
    data := make(map[string]interface{}) // allocation
    data["status"] = "ok"
    json.NewEncoder(w).Encode(data)
}

// ✅ Reuse structures or use static responses
var okResponse = []byte(`{"status":"ok"}`)
func handler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Write(okResponse)
}
```

### 2. Middleware Selection

Only use the middleware you need:

```go
// ❌ Don't apply all middleware globally
srv.AddMiddleware("*", RequestLoggerMiddleware) // +969ns everywhere

// ✅ Apply middleware selectively
srv.AddMiddleware("/api", RateLimitMiddleware(srv))
srv.AddMiddleware("/debug", RequestLoggerMiddleware)
```

### 3. Static File Optimization

For best static file performance:

```go
// ✅ HyperServe uses os.Root for security without sacrificing speed
srv.HandleStatic("/static/")

// Consider CDN for large static assets
// HyperServe is optimized for dynamic content
```

### 4. Rate Limiting Configuration

Go 1.24's Swiss Tables (or newer) make rate limiting 30-35% faster. On earlier toolchains the improvement will be smaller:

```go
// Configure appropriate limits
srv, _ := hyperserve.NewServer(
    hyperserve.WithRateLimit(1000, 2000), // 1000 req/s, burst 2000
)
```

### 5. Logging Performance

RequestLogger adds ~1μs due to I/O. For high-performance scenarios:

```go
// ❌ Don't log every request in production
srv.AddMiddleware("*", RequestLoggerMiddleware)

// ✅ Log selectively or asynchronously
if debug {
    srv.AddMiddleware("/debug", RequestLoggerMiddleware)
}
```

## Design Philosophy

HyperServe prioritizes:

1. **Predictable performance** - No hidden allocations or surprising costs
2. **Linear scaling** - Performance degrades gracefully under load
3. **Memory stability** - No leaks, automatic cleanup of resources
4. **Fair comparison** - We measure relative overhead, not absolute numbers

### Trade-offs

- **Simplicity over micro-optimizations**: Readable code that's fast enough
- **Safety over speed**: Timing-safe auth, proper bounds checking
- **Features when needed**: Middleware is pay-for-what-you-use

## Running Benchmarks

To run benchmarks yourself:

```bash
# Run all benchmarks
go test -bench=. -benchmem

# Run specific benchmark
go test -bench=BenchmarkBaseline -benchmem

# Profile CPU usage
go test -bench=BenchmarkSecureAPI -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Profile memory
go test -bench=BenchmarkSecureAPI -memprofile=mem.prof
go tool pprof mem.prof
```

## Future Optimizations

Areas we're exploring for even better performance:

1. **Zero-allocation static file serving** (currently 31 allocs)
2. **Buffered logging** for RequestLogger middleware
3. **Connection pooling** for keep-alive optimization
4. **SIMD optimizations** for header parsing (Go 1.25+)

## Contributing

Have ideas for performance improvements? See our [Contributing Guide](../CONTRIBUTING.md) and run benchmarks before/after your changes to measure impact.
