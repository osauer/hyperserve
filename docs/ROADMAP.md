# HyperServe Roadmap

_Last updated: 2026-08-24 (v2 line)._

HyperServe is the Go API server its maintainer wanted to own: standard
`net/http` shapes, an explicit lifecycle and configuration boundary, and the
operational pieces that otherwise accumulate around each service. WebSocket,
JSON-RPC, and optional MCP remain in-tree so adopters can choose an integrated
server instead of assembling a framework from many packages.

## Product Thesis

A service should be able to add lifecycle, request binding, security middleware,
observability, WebSocket, and MCP without replacing ordinary Go handlers. Each
integrated capability must remove repeated application work and justify the
protocol and compatibility surface HyperServe then owns.

MCP is first-class but optional. When enabled, it can expose tools and resources
from the same process that serves HTTP traffic. The application still owns
authorization, logging boundaries, and which capabilities are reachable.

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
| Production MCP observability | Keep resources live, route inspection truthful, logs server-owned, and discovery cache-safe. | An in-process observability surface must not capture unrelated application state or weaken caller authority. |
| Scaffold reliability | Generated projects should build outside the monorepo, include the right module requirement, and use current Go/tooling defaults. | The generated service is many users' first executable contract with the library. |
| Protocol conformance | Continue tightening JSON-RPC, SSE, and WebSocket behavior against their specs. | Agent clients are strict; protocol drift becomes integration pain. |
| Benchmark discipline | Keep concurrent workloads reproducible and publish only environment-qualified results. | Performance claims should follow evidence rather than drive speculative optimization. |
| Authentication adapters | Keep `pkg/auth` provider-neutral while maintaining one real OIDC example and clear authorization boundaries. | Identity-provider choice should not inflate the core module or blur application policy. |

## Release Discipline

- Keep `cmd/hyperserve-init` as the supported command; avoid checked-in demo binaries.
- Keep v2 semver clean. Breaking exported APIs require a future `/v3` module path.
- Use `make release RELEASE_VERSION=vX.Y.Z`; it checks the changelog, local
  gates, scaffold smoke, clean tree, synced `origin/main`, tag uniqueness, and
  then publishes GitHub release notes derived from `CHANGELOG.md`.
- Run `make changelog-stub RELEASE_VERSION=vX.Y.Z` at the start of release
  prep, then fill the `### What's new` section in reader-facing language.
- Update docs and examples in the same change as API or behavior changes.
