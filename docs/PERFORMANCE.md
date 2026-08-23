# Performance and benchmarking

HyperServe does not publish a universal throughput or allocation baseline.
Results depend on the Go version, host, handler, middleware stack, request shape,
and benchmark harness. A number without that context is not useful adoption
evidence.

## Supported benchmarks

The maintained benchmark surface is `pkg/server/benchmark_test.go`. It exercises:

- a minimal handler;
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

The previous `wrk` script targeted a removed `cmd/server` binary and could not run
from a current checkout, so it was removed together with its unqualified result
summary. [Issue #82](https://github.com/osauer/hyperserve/issues/82) tracks a
replacement concurrent harness with explicit workloads, reliable cleanup, and
reproducible result metadata.

Until that work lands, package microbenchmarks are the only supported performance
measurement in this repository. Do not infer production throughput from them.

## Optimization policy

- Measure a concrete workload before changing routing, logging, pooling, or
  synchronization.
- Prefer clear code when results do not show a meaningful bottleneck.
- Preserve security and protocol correctness when optimizing hot paths.
- Publish qualified measurements, not adjectives such as "high-performance."

See [`benchmarks/README.md`](../benchmarks/README.md) for the short command
reference and [CONTRIBUTING.md](../CONTRIBUTING.md) for the repository workflow.
