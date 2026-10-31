# API and release policy

HyperServe follows semantic versioning. The current module is
`github.com/osauer/hyperserve/v2`. Version `v2.2.2` is a maintenance release;
existing v2 applications keep their imports. See
[ADR-0019](0019-v2-maintenance.md) and [upgrade notes](MIGRATING.md).

## Versions and compatibility

- Patch releases fix defects without deliberately breaking the public contract.
- Minor releases add backward-compatible functionality.
- Breaking public API or behavior changes require a new major version, a
  matching Go module suffix, and migration instructions.

Public exported Go APIs and documented behavior are compatibility commitments.
Correcting a defect still requires checking observable effects on callers.
Only the current release line receives fixes; old tags remain available for
reproducible builds. Do not silently change a consumer's dependency pin.

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
