# Benchmarks

HyperServe has two comparison tools. Neither is a production capacity claim.

## In-process benchmarks

The Go benchmarks in the root [`benchmark_test.go`](../benchmark_test.go) cover
serial and concurrent request paths, middleware, static files, JSON, and MCP
operations. The standalone limiter has its own middleware and entry-footprint
benchmarks:

```sh
go test -run '^$' -bench . -benchmem .
go test -run '^$' -bench . -benchmem ./ratelimit
```

Use `-count=5` when comparing revisions so one noisy sample does not drive a
conclusion.

## Loopback load profiles

The repository-native load command builds a maintained fixture and a small
standard-library load tool, runs two profiles, records the environment, and
then stops the fixture:

```sh
make benchmark-load
```

It requires `go`, `git`, and `curl`; no third-party load generator is needed.
The defaults are deliberately short and can be changed without editing files:

| Variable | Default | Meaning |
|---|---:|---|
| `BENCH_DURATION` | `5s` | Time spent on each profile |
| `BENCH_THREADS` | `4` | Go execution parallelism (`GOMAXPROCS`) |
| `BENCH_CONNECTIONS` | `32` | Concurrent request workers |
| `BENCH_PORT` | `18080` | Loopback port for the fixture |
| `BENCH_RESULTS_DIR` | timestamped directory | Where metadata, logs, and results are written |

For example:

```sh
BENCH_DURATION=30s BENCH_THREADS=8 BENCH_CONNECTIONS=100 make benchmark-load
```

The `minimal` profile uses a small handler with HyperServe's defaults. The
`middleware` profile adds security headers and bearer authentication. Results
go beneath `benchmarks/results/`, which Git ignores. Each run includes the exact
commit, clean/dirty state, Go version, platform, workload, and profile details.

Compare runs only when their metadata and inputs match. See the
[performance guide](../docs/PERFORMANCE.md) for the interpretation boundary.
