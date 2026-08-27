# API Stability

HyperServe normally follows semantic versioning on the
`github.com/osauer/hyperserve/v2` module line:

- patch releases fix defects without intentionally breaking exported behavior;
- minor releases add compatible APIs and behavior;
- future breaking exported API changes require a new major module path.

## The v2.1.0 exception

On 2026-08-27, v2.1.0 makes one explicitly controlled compatibility reset
inside `/v2`. It replaces the intermediate `pkg/...` public layout with the
branded root package and concern-specific subpackages, renames `NewServer` to
`New`, and moves rate limiting out of `Server`.

That is not ordinary SemVer compatibility. It was accepted as a narrow,
one-time correction before the intermediate shape accumulated more consumers.
It does not establish permission for another breaking minor release.

Before upgrading an existing v2 application, read
[Migrating to v2.1](./MIGRATING_V2_1.md). To roll back:

```sh
go get github.com/osauer/hyperserve/v2@v2.0.3
```

After v2.1.0, the normal rule resumes: a breaking public API change requires a
future major version and corresponding module path.

## Stable public packages

The compatibility promise covers exported APIs in:

- `github.com/osauer/hyperserve/v2`
- `github.com/osauer/hyperserve/v2/auth`
- `github.com/osauer/hyperserve/v2/jsonrpc`
- `github.com/osauer/hyperserve/v2/mcp`
- `github.com/osauer/hyperserve/v2/mcp/builtin`
- `github.com/osauer/hyperserve/v2/ratelimit`
- `github.com/osauer/hyperserve/v2/websocket`

There are no compatibility packages under `pkg/...` and no `NewServer`
constructor alias.

The following are maintained and release-gated but are not stable Go import
surfaces:

- `cmd/hyperserve-init` command flags and generated project layout;
- `examples/` and `benchmarks/`;
- `internal/`, `scripts/`, and `tools/`;
- Make targets and repository-only test helpers.

Changes there should still be documented when they affect users, but they do
not carry the same source-compatibility promise as exported package APIs.

## Behavior and ownership

Compatibility includes documented behavior, not only exported names. In
particular:

- `hyperserve.New` is deterministic unless the application explicitly binds
  a file or process environment;
- the application owns signals and the root context, while `Run(ctx)` follows
  cancellation and handlers use `r.Context()`;
- authentication does not imply application authorization;
- the root server does not own limiter policy or quota state;
- default rate-limit identity does not trust forwarding headers;
- MCP discovery visibility does not authorize endpoint access;
- WebSocket and streaming cancellation follow the documented request and
  connection contracts.

Changing one of these boundaries incompatibly requires the same major-version
treatment as changing an exported signature.

## Deprecation

When practical, a supported replacement is documented before removal and the
old surface remains through a major-version boundary. Security-sensitive
behavior may be disabled or removed sooner when retaining it would preserve an
unsafe authority path; such a change must be called out prominently.

The proprietary HyperServe routed-SSE transport is deprecated and disabled by
default. It is not the current MCP Streamable HTTP transport.

## Toolchain floor

The `go` directive in `go.mod` is the supported compiler floor. Raising it
is a compatibility event and must be documented in release notes.

For the concrete v1-to-current mapping, see
[Migrating from v1](./MIGRATING_V2.md).
