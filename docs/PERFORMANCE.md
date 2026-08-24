# Performance and benchmarking

HyperServe does not publish a universal throughput or allocation baseline.
Results depend on the Go version, host, handler, middleware stack, request shape,
and benchmark harness. A number without that context is not useful adoption
evidence.

## Supported benchmarks

The maintained benchmark surface is `pkg/server/benchmark_test.go`. It exercises:

- serial and concurrent minimal handlers;
- individual and combined middleware;
- static-file and JSON responses; and
- MCP request, tool, resource, handshake, middleware, and payload paths.

Run it with:

```sh
go test -run '^$' -bench . -benchmem ./pkg/server
```

These are in-process microbenchmarks built with `httptest`. They are useful for
comparing two revisions of the same code on the same machine. They do not model
network latency, concurrent clients, proxy behavior, TLS termination, or an
application's real handler work.

## Compare a change

Capture several samples before and after the change:

```sh
go test -run '^$' -bench . -benchmem -count=5 ./pkg/server > before.txt
# apply the change
go test -run '^$' -bench . -benchmem -count=5 ./pkg/server > after.txt
```

Keep the following with any reported result:

- exact Git commit;
- `go version` output;
- operating system, architecture, and CPU;
- full benchmark command;
- benchmark names and sample count; and
- any relevant application options or middleware.

Compare like with like. A faster result on a different toolchain or host is not
evidence that the code change caused the difference. Investigate allocations and
profiles only after a stable comparison identifies a regression or bottleneck.

## End-to-end load testing

Run the maintained loopback harness from the repository root:

```sh
make benchmark-load
```

The command builds temporary server and load-tool binaries, waits for the server
to become ready, runs a minimal profile and an authenticated security-middleware
profile, then stops the server on success, failure, or interruption. Its load
tool uses only the Go standard library. `go`, `git`, and `curl` are the explicit
prerequisites.

Each timestamped directory beneath `benchmarks/results/` contains:

- `metadata.txt` with the exact commit, clean/dirty state, Go version, platform,
  duration, execution parallelism, worker count, endpoints, and middleware;
- one result file per profile, including status counts, request rate, response
  bytes, and bounded latency percentiles; and
- `server.log` for startup or runtime diagnostics.

Tune a workload through `BENCH_DURATION`, `BENCH_THREADS`,
`BENCH_CONNECTIONS`, and `BENCH_PORT`. Keep those inputs identical for before
and after runs. The short defaults check that the harness works; longer repeated
runs are more useful when investigating a measured change.

Loopback load removes network variability but still shares one machine between
client and server. It does not model a reverse proxy, TLS termination, remote
clients, noisy neighbors, or application work. Treat it as regression evidence,
not production sizing advice.

## Optimization policy

- Measure a concrete workload before changing routing, logging, pooling, or
  synchronization.
- Prefer clear code when results do not show a meaningful bottleneck.
- Preserve security and protocol correctness when optimizing hot paths.
- Publish qualified measurements, not adjectives such as "high-performance."

See [`benchmarks/README.md`](../benchmarks/README.md) for the short command
reference and [CONTRIBUTING.md](../CONTRIBUTING.md) for the repository workflow.
