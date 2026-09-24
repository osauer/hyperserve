# HyperServe

- Read [ARCHITECTURE.md](ARCHITECTURE.md) and
  [API stability](docs/API_STABILITY.md); use the [MCP guide](docs/MCP_GUIDE.md)
  for protocol work.
- Applications own signals, root context and authorization. Keep optional
  builtins and rate limiting out of the root package's imports.
- Run `make check` before committing; `make test` adds unit tests.
  Package/lifecycle changes also require `make test-race`, `make fuzz-smoke`
  and an ADR. Public API changes need updated docs/examples/scaffolds and a
  disposable consumer witness.
- Keep developer dependencies in `tools/go.mod`. Preserve historical changelog
  and ADR text; supersede decisions with new ADRs.
- A commit or tag does not prove publication or fresh-module retrieval.
