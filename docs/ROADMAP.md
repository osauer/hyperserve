# HyperServe Roadmap

_Last updated: 2026-05-24 07:43 CEST (v1 line)._

HyperServe is a library-first Go HTTP framework with in-process MCP for
agentic workloads. The near-term roadmap is about making that story sharp:
production MCP observability, Streamable HTTP correctness, and a small set of
canonical examples that stay release-gated.

## Product Thesis

A small `net/http`-shaped server with first-class MCP, JSON-RPC, SSE,
WebSocket, request binding, and production middleware. AI assistants should be
able to inspect a live service through the same binary that serves traffic,
without an out-of-process bridge.

## Canonical Examples

These three examples define the release story:

- `examples/devops`: production MCP observability.
- `examples/mcp-extensions`: current Streamable HTTP subscriptions over SSE.
- `examples/json-api`: a normal JSON API server using method-aware routes and typed binding.

Other examples are supplemental. They should not dilute the main README or
release gate unless they protect a specific production contract.
`examples/mcp-sse` remains release-gated separately as a deprecated routed-SSE
compatibility regression, not as the primary transport story.

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
- Use `make release RELEASE_VERSION=vX.Y.Z`; it checks the changelog, local
  gates, scaffold smoke, clean tree, synced `origin/main`, tag uniqueness, and
  then publishes GitHub release notes derived from `CHANGELOG.md`.
- Run `make changelog-stub RELEASE_VERSION=vX.Y.Z` at the start of release
  prep, then fill the `### What's new` section in reader-facing language.
- Update docs and examples in the same change as API or behavior changes.
