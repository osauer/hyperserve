# Benchmarks

HyperServe currently supports in-process Go microbenchmarks in
`pkg/server/benchmark_test.go`:

```sh
go test -run '^$' -bench . -benchmem ./pkg/server
```

Use them to compare revisions on the same machine. They do not establish
production throughput or model concurrent network clients.

The previous `wrk` harness was removed because it built a server command that no
longer exists. [Issue #82](https://github.com/osauer/hyperserve/issues/82) tracks
its replacement. See the [performance guide](../docs/PERFORMANCE.md) for scope,
comparison procedure, and the metadata required with reported results.
