# HyperServe Roadmap

_Last updated: 2026-05-24 07:21 CEST (v1 line)._

HyperServe is a library-first Go HTTP framework with in-process MCP for
agentic workloads. The near-term roadmap is about making that story sharp:
production MCP observability, SSE transport correctness, and a small set of
canonical examples that stay release-gated.

## Product Thesis

A small `net/http`-shaped server with first-class MCP, JSON-RPC, SSE,
WebSocket, request binding, and production middleware. AI assistants should be
able to inspect a live service through the same binary that serves traffic,
without an out-of-process bridge.

## Canonical Examples

These three examples define the release story:

- `examples/devops`: production MCP observability.
- `examples/mcp-sse`: MCP over the unified SSE/HTTP endpoint.
- `examples/json-api`: a normal JSON API server using method-aware routes and typed binding.

Other examples are supplemental. They should not dilute the main README or
release gate unless they protect a specific production contract.

## Near-Term Work

| Theme | Description | Why It Matters |
|---|---|---|
| Production MCP observability | Keep resources live, route inspection truthful, logs wired, and discovery cache-safe. | This is the project differentiator and must be trustworthy in production. |
| Scaffold reliability | Generated projects should build outside the monorepo, include the right module requirement, and use current Go/tooling defaults. | A broken first generated app reflects poorly on the framework. |
| Protocol conformance | Continue tightening JSON-RPC, SSE, and WebSocket behavior against their specs. | Agent clients are strict; protocol drift becomes integration pain. |
| Observability exports | Explore lightweight OTLP-compatible metrics/trace export without pulling a full SDK into the runtime. | Connects HyperServe to existing production stacks while keeping the core small. |
| Runtime safeguards | Design privileged MCP controls with policy hooks, auditing, and narrow scopes. | Makes agent-assisted operations useful without turning MCP into an unsafe control plane. |

## Release Discipline

- Keep `cmd/hyperserve-init` as the supported command; avoid checked-in demo binaries.
- Keep v1 semver clean. Breaking exported APIs require a future `/v2` module path.
- Run `make check`, `go test ./...`, and the canonical example gate before tagging.
- Update docs and examples in the same change as API or behavior changes.
