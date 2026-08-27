# Performance and Benchmarking

HyperServe publishes benchmark methods, not a universal throughput number.
Results depend on the Go version, host, route shape, middleware, payload,
connection reuse, and scheduler.

## Maintained benchmark surfaces

- repository-root `benchmark_test.go` — routing, middleware dispatch, typed
  handlers, MCP wiring, and representative request paths;
- `ratelimit/benchmark_test.go` — quota lookup and entry-footprint evidence;
- `benchmarks/load` — loopback HTTP workload profiles.

Run in-process benchmarks:

```sh
go test -run '^$' -bench . -benchmem .
go test -run '^$' -bench . -benchmem ./ratelimit
```

Run the maintained loopback profiles:

```sh
make benchmark-load
```

## Comparing revisions

Record both revisions, the exact command, Go version, operating system, CPU,
and whether other load was present.

```sh
go test -run '^$' -bench . -benchmem -count=10 . > before.txt
# switch to the candidate revision
go test -run '^$' -bench . -benchmem -count=10 . > after.txt
benchstat before.txt after.txt
```

Use the same process for `./ratelimit`. For allocation or entry-footprint
changes, report both bytes per operation and the implied bound at the default
`MaxClients`; do not turn a lower-bound structure estimate into a whole-process
memory claim.

## Interpreting results

- A microbenchmark isolates one path. It does not prove end-to-end throughput.
- A loopback result includes networking, scheduling, client behavior, and
  handler work. It can move differently from a middleware microbenchmark.
- An improvement claim needs repeated A/B samples from comparable conditions.
- A regression needs a named workload and magnitude before adding caching,
  pooling, custom routers, assembly, or new configuration knobs.
- Race detection and correctness tests remain mandatory; benchmark speed does
  not waive behavior.

## Profiling

Capture profiles only after a stable comparison identifies a real bottleneck:

```sh
go test -run '^$' -bench BenchmarkName -cpuprofile cpu.out -memprofile mem.out .
go tool pprof cpu.out
go tool pprof mem.out
```

Keep generated profiles and benchmark output out of commits unless they are
intentional review fixtures.

## Claims in documentation

Documentation may state architectural facts such as the standard
`net/http.ServeMux` router or the single runtime dependency. It must not claim
that HyperServe is faster than another framework without a current,
reproducible comparison that names both revisions and workloads.
