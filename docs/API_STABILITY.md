# API and release policy

HyperServe has one owner and one maintained API. The module path is
`github.com/osauer/hyperserve`; it does not acquire a new `/vN` suffix for each
breaking change. Applications pin a release or commit and migrate together.
This policy was adopted in [ADR-0018](0018-single-owner-module.md).

## Versions and compatibility

Unsuffixed Go modules use v0 or v1 versions. The existing unsuffixed history
ends at `v1.6.1`; the next release is `v1.7.0`. Old `/v2` tags remain available
for reproducible builds. Only the current release line receives fixes; there
are no backports or parallel compatibility implementations.

Release numbers identify snapshots. They do not promise that every minor
release is source-compatible. A breaking change requires migration instructions,
consumer checks and a clear **Breaking (Go library)** release note. Do not silently
change a caller's dependency pin. See [migration from /v2](MIGRATING.md).

The public packages are listed in [Architecture](../ARCHITECTURE.md#public-package-graph).
The root package owns HTTP integration; concern packages remain independently
usable. `internal`, examples, benchmarks and generated layouts are not public
import contracts. Avoid compatibility aliases without a present consumer need.

## Ownership contracts

- Construction is deterministic unless the application explicitly binds files
  or environment.
- The application owns signals, root context, data and authorization.
- `Run(ctx)` follows application cancellation; handlers follow request context.
- Authentication and discovery visibility do not grant resource authorization.
- Rate-limit policy belongs to the application; default identity does not
  trust forwarding headers.
- Stream writes, cancellation and shutdown follow their documented contracts.

Changing one of these boundaries requires an ADR and tests at the affected
boundary. The Go toolchain floor in `go.mod` and any increase are part of the
consumer migration contract.

The proprietary routed-SSE transport is retained, deprecated and disabled by
default. It is isolated from current MCP Streamable HTTP and ordinary HTTP SSE.
