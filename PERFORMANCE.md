# HyperServe Performance Guide

## Overview

HyperServe is designed for high performance with minimal overhead. Our benchmarks demonstrate that you can have both security and speed.

## Benchmark Results

Running on Apple M4 Pro (darwin/arm64):

### Core Performance

| Scenario | Throughput | Latency | Memory/req | Allocations/req |
|----------|------------|---------|------------|-----------------|
| Baseline HTTP | 2.6M req/s | 381ns | 1010 B | 10 |
| Secure API Stack | 2.9M req/s | 349ns | 1056 B | 10 |
| Static Files | 104K req/s | 9.6μs | 2536 B | 31 |
| JSON API | 783K req/s | 1.3μs | 1938 B | 27 |

### Middleware Overhead

Each middleware component adds minimal latency:

| Middleware | Latency Cost | Use Case |
|------------|--------------|----------|
| Recovery | -35ns ✨ | Panic recovery (actually optimizes!) |
| Auth | +82ns | Token validation |
| Trace | +128ns | Request tracing |
| RateLimit | +197ns | Request throttling |
| Headers | ~100ns | Security headers |
| RequestLogger | +969ns | Structured logging |

**Total overhead for full security stack: <100ns** (excluding logging)

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

Go 1.24's Swiss Tables make rate limiting 30-35% faster:

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

## Comparison with Other Frameworks

HyperServe aims to match or exceed `net/http` performance while providing more features:

| Framework | Simple Handler | Features | Dependencies |
|-----------|---------------|----------|--------------|
| HyperServe | 381ns | Full suite | 1 (rate limiter) |
| net/http | ~300ns | Basic | 0 |
| gin | ~450ns | Router + middleware | Many |
| echo | ~420ns | Full framework | Many |

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

Have ideas for performance improvements? See our [Contributing Guide](CONTRIBUTING.md) and run benchmarks before/after your changes to measure impact.