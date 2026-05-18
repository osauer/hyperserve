# Benchmarks

HyperServe has two benchmark surfaces:

## 1. Go micro-benchmarks

Live under `pkg/server/benchmark_test.go`. Standard Go benchmarks; measure
handler/middleware overhead and JSON encoding paths.

```bash
go test -bench=. -benchmem ./pkg/server
```

For CPU/memory profiling:

```bash
go test -bench=. -cpuprofile=cpu.out ./pkg/server
go test -bench=. -memprofile=mem.out ./pkg/server
go tool pprof cpu.out
```

## 2. End-to-end load tests

`./run_benchmarks.sh` runs an HTTP load test against a built `cmd/server`
binary using `wrk`. Requires `wrk` on PATH.

```bash
./benchmarks/run_benchmarks.sh
```

The script reports requests/sec, latency percentiles, and transfer rates.

## What you get

There are no aspirational performance targets in this repo: actual numbers
are workload-dependent (handler complexity, middleware stack, hardware).
Use these benchmarks as a relative comparison tool — before/after a change —
not as absolute claims.
