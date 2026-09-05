# API Stability

HyperServe maintains one release line: only the latest stable release receives
bug fixes and security updates. Development happens on `main`, and fixes ship
in the next release. Older tags remain available for reproducible builds, with
no backports or parallel maintenance branches.

The current module path is `github.com/osauer/hyperserve/v2`. Releases from
the current API follow semantic versioning:

- patch releases fix defects without intentionally breaking exported behavior;
- minor releases add compatible APIs and behavior;
- future breaking exported API changes require a new major module path.

The earlier v2.1.0 package migration is documented in
[Migrating from v2.0.x](./MIGRATING_V2_1.md). Historical changes and rollback
instructions live in that guide and the changelog.

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
