# Contributing to HyperServe

HyperServe is maintained as a small Go library. Keep changes focused, preserve
standard-library handler shapes, and prove public behavior at the boundary that
changed.

## Setup

```sh
git clone https://github.com/osauer/hyperserve.git
cd hyperserve
go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
make check
make test-race
make fuzz-smoke
```

The required Go version is recorded in `go.mod` (currently 1.27).
`modernize` is pinned in `tools/go.mod` and runs from that module, so it
does not enter HyperServe's shipped dependency graph.

## Check gate

| Check | Purpose |
|---|---|
| `gofmt` | Canonical Go formatting |
| `go vet` and `staticcheck` | Correctness, simplification, and redundancy diagnostics |
| `govulncheck` | Reachable vulnerabilities in runtime, example, and tool graphs |
| `modernize` | Current Go idioms without changing public behavior |
| documentation and example checks | Runnable imports, links, scaffold output, and stale-surface prevention |
| MCP conformance | Protocol behavior against the official SDK |
| release-gate fixtures | Exact-SHA CI selection and failure handling |

`make test` runs the repository gate before the unit suite. Race and fuzz
checks are separate because of cost; run both before pushing a public API,
protocol, concurrency, or lifecycle change.

## Code map

| Concern | Location |
|---|---|
| Root HTTP server, middleware, options, binding, lifecycle | repository-root `*.go` |
| Authentication and principals | `auth/` |
| JSON-RPC engine | `jsonrpc/` |
| MCP protocol, transports, discovery, namespaces | `mcp/` |
| Opt-in MCP tools and resources | `mcp/builtin/` |
| Bounded rate-limit middleware | `ratelimit/` |
| WebSocket server and outbound client | `websocket/` |
| Scaffold generator | `internal/scaffold/`, `cmd/hyperserve-init/` |
| Runnable examples | `examples/` |

The main library import is `github.com/osauer/hyperserve/v2`. The old
`.../pkg/...` public paths have no forwarding packages.

## Design boundaries

Read the relevant ADR before changing a load-bearing boundary:

- [ADR-0001](docs/0001-minimal-external-dependencies.md) — runtime dependency policy.
- [ADR-0008](docs/0008-graceful-shutdown-design.md) — application-owned lifecycle.
- [ADR-0011](docs/0011-mcp-protocol-support.md) and
  [ADR-0012](docs/0012-mcp-streamable-http-2026.md) — MCP transports.
- [ADR-0014](docs/0014-root-package-and-concern-subpackages.md) — canonical
  root and concern-package architecture.

The application owns signals, its root context, identity-provider setup,
sessions, and authorization. HyperServe follows `Run(ctx)`; request work
follows `r.Context()`. The standalone `ratelimit` package owns quota state;
the root `Server` does not.

If a change contradicts an accepted ADR, propose a superseding ADR in the same
pull request. Do not silently revise historical decision prose.

## Pull requests

1. Branch from `main`.
2. Keep one concern per branch and avoid unrelated cleanup.
3. Add focused tests at the changed risk surface.
4. Update current documentation and examples when the public surface changes.
5. Run `make check`, `make test-race`, and `make fuzz-smoke`.
6. Use an imperative commit subject of at most 72 characters; explain why in
   the body when the reason is not obvious.

Changes to exported APIs need a disposable consumer witness in addition to
in-repository tests. A local replacement is useful before publication, but it
must never be committed as release evidence.

## Release notes

User-visible changes need a `CHANGELOG.md` entry:

```sh
make changelog-stub RELEASE_VERSION=vX.Y.Z
make changelog-lint RELEASE_VERSION=vX.Y.Z
```

`make release-publish` promotes the `What's new` section into the GitHub
Release body. Breaking releases must put the warning, migration guide, and
rollback pin before any upgrade command.
Later compatible releases link to the migration guide instead of repeating
the historical warning. Only the latest stable release receives fixes; see
[the support policy](docs/API_STABILITY.md).

The canonical release path is
`make release RELEASE_VERSION=vX.Y.Z`. It verifies the pushed candidate's
exact-SHA `push` CI run and every required job before creating a tag. Local
`make release-smoke` is source and scaffold evidence; it is not CI or
public-module evidence.

If an annotated tag is pushed but GitHub Release creation fails, never move or
replace the tag. Verify that local and remote tags resolve to the same commit
and that exact-SHA CI succeeded, then recover only the publication step with
`make release-publish RELEASE_VERSION=vX.Y.Z`.

## Reporting issues

Open a [GitHub Issue](https://github.com/osauer/hyperserve/issues) with the Go
version, operating system, a minimal reproduction, and expected versus actual
behavior. Report security issues privately as described in
[SECURITY.md](SECURITY.md).

[GitHub Discussions](https://github.com/osauer/hyperserve/discussions) is for
usage and fit questions.
