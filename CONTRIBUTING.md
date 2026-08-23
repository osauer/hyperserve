# Contributing to HyperServe

Thanks for your interest. HyperServe is a small v1 Go library (single
maintainer). This doc tells you exactly what CI gates on so the first PR comes in
green.

## Setup

```bash
git clone https://github.com/osauer/hyperserve.git
cd hyperserve
go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
make check && make test-race && make fuzz-smoke
```

If those three commands pass locally, CI will pass. `modernize` is pinned in
`tools/go.mod` and invoked via `go -C tools tool modernize`, so it downloads
on first use without entering the shipped module graph.

Go version: see `go.mod` (currently 1.27).

## The check gate

CI runs `make test`, which invokes `make check` first. The gate is:

| Tool                                    | What it catches                                                       |
|-----------------------------------------|-----------------------------------------------------------------------|
| `gofmt`                                 | unformatted Go — fix with `make fmt`                                  |
| `go vet`                                | suspicious constructs                                                 |
| `staticcheck`                           | bugs, simplifications, redundancy                                     |
| `govulncheck`                           | known CVEs in shipped, example, and developer-tool module graphs       |
| `go fix -diff` + tools-module modernize | current Go idioms (`any`, `for i := range N`, `wg.Go`, `b.Loop`, …)   |

Apply idiom fixes in place with `make modernize`. Race detector and fuzz
smoke run as separate CI steps — run `make test-race` and `make fuzz-smoke`
locally before pushing.

A `make check` failure on your machine is the same failure CI will report;
fix it locally rather than relying on CI to surface it.

## Code map

| What                                              | Where                          |
|---------------------------------------------------|--------------------------------|
| HTTP server, middleware, deferred-init lifecycle  | `pkg/server/`                  |
| MCP protocol, JSON-RPC dispatch, namespaces       | `pkg/mcp/`                     |
| SSE transport (binding tokens, connection events) | `pkg/mcp/transport_sse.go`     |
| Built-in MCP tools and resources (opt-in)         | `pkg/mcp/builtin/`             |
| WebSocket server and outbound client (RFC 6455)  | `pkg/websocket/`               |
| JSON-RPC 2.0 engine                               | `pkg/jsonrpc/`                 |
| Scaffold generator                                | `internal/scaffold/`, `cmd/hyperserve-init/` |
| Self-contained `go run .` examples                | `examples/`                    |

The library imports as `github.com/osauer/hyperserve/pkg/server`. There is
no Go code at the repository root.

## Architecture decisions

Before changing load-bearing design — transports, dependency policy,
lifecycle — read the relevant ADR in [`docs/`](docs/). They are short and
record the *why*. Notable ones:

- [ADR 0001](docs/0001-minimal-external-dependencies.md) — minimal external
  dependencies. This is load-bearing for the project's pitch; new transitive
  deps need a strong case.
- [ADR 0002](docs/0002-functional-options-pattern.md) — functional options
  (`WithFoo(...)`) over config structs.
- [ADR 0008](docs/0008-graceful-shutdown-design.md) — graceful shutdown.
- [ADR 0011](docs/0011-mcp-protocol-support.md) — MCP protocol support.

If a change contradicts an ADR, propose superseding it in the same PR.

## Submitting changes

1. Branch from `main`.
2. Keep the PR focused — one concern per branch.
3. Locally: `make check && make test-race && make fuzz-smoke`.
4. Commit subject in the imperative, ≤72 chars. Body explains *why* (the
   diff shows *what*). New features ship with tests and updated docs.
5. Push and open a PR. CI runs the same gate you just ran.

## Release notes

User-visible changes need a `CHANGELOG.md` entry. Start one with:

```bash
make changelog-stub RELEASE_VERSION=vX.Y.Z
```

Fill `### What's new` in plain English; that section is promoted directly into
the GitHub Release body by `make release-publish`. Before a release, run
`make changelog-lint RELEASE_VERSION=vX.Y.Z` to catch malformed headings,
missing user-facing highlights, and public notes that leak internal review IDs.

## Reporting issues

Open a [GitHub Issue](https://github.com/osauer/hyperserve/issues) with:

- Go version (`go version`) and OS
- Minimal reproduction (a failing test is best)
- Expected vs actual behavior

For security issues, see [SECURITY.md](SECURITY.md) — do **not** open a
public issue.

## Questions

[GitHub Discussions](https://github.com/osauer/hyperserve/discussions) is
the right place for "is this the right fit for X?" or "how should I approach
Y?" questions. Bug reports and feature proposals go in Issues.
