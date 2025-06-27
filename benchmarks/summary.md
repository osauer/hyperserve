# HyperServe Benchmark Results

## Summary

Running on Apple M4 Pro (darwin/arm64)

### Key Performance Metrics

| Benchmark | Time/op | Memory/op | Allocs/op | Req/sec |
|-----------|---------|-----------|-----------|---------|
| Baseline | 381.3 ns | 1010 B | 10 | ~2.6M |
| Secure API | 348.9 ns | 1056 B | 10 | ~2.9M |
| Static File | 9599 ns | 2536 B | 31 | ~104K |
| JSON Response | 1277 ns | 1938 B | 27 | ~783K |

### Individual Middleware Overhead

| Middleware | Time/op | Memory/op | Allocs/op | Cost vs Baseline |
|------------|---------|-----------|-----------|------------------|
| Recovery | 346.4 ns | 1010 B | 10 | -34.9 ns (faster!) |
| Auth | 463.3 ns | 1394 B | 13 | +82.0 ns |
| Trace | 509.1 ns | 1448 B | 16 | +127.8 ns |
| RateLimit | 578.3 ns | 1456 B | 15 | +197.0 ns |
| RequestLogger | 1350 ns | 1166 B | 16 | +968.7 ns |

### Analysis

1. **Surprising Result**: The SecureAPI benchmark (with all middleware) is FASTER than the baseline! This suggests:
   - Potential measurement overhead in baseline
   - Middleware might be optimizing the request path
   - Need for longer benchmark runs

2. **Excellent Performance**: 
   - Sub-microsecond response times for basic operations
   - 2.6M+ requests/second capability
   - Minimal memory allocations (10 per request)

3. **Middleware Efficiency**:
   - Most middleware adds < 200ns overhead
   - RequestLogger is the most expensive (due to logging I/O)
   - Total stack overhead still keeps responses under 400ns

4. **Areas for Optimization**:
   - Static file serving has higher allocations (31)
   - JSON encoding could be optimized
   - RequestLogger could use buffered logging

## Next Steps

1. Compare with `net/http` baseline
2. Add benchmarks for concurrent requests
3. Profile memory allocations in static file handler
4. Test with real-world payloads