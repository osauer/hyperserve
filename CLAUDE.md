# CLAUDE.md

Repository guidance for coding agents working on HyperServe.

## Canonical layout

HyperServe is a Go HTTP library with optional Model Context Protocol support.
Its public packages are:

- repository root — `github.com/osauer/hyperserve/v2`, package
  `hyperserve`;
- `auth/` — authentication and stable request principals;
- `jsonrpc/` — JSON-RPC 2.0;
- `mcp/` and `mcp/builtin/` — MCP protocol and opt-in builtins;
- `ratelimit/` — bounded HTTP rate-limit middleware;
- `websocket/` — RFC 6455 server and client.

There are no public `pkg/...` packages or compatibility facades. The central
constructor is `hyperserve.New`; do not add `NewServer` aliases.

`cmd/hyperserve-init` is the supported repository binary. Generated
applications have their own `cmd/server` and may name an application-owned
factory `app.NewServer(cfg)`; that does not recreate a HyperServe constructor.

## Ownership boundaries

- Applications own process signals and the root context. HyperServe follows
  `Run(ctx)`; handlers follow `r.Context()`.
- The root package owns HTTP serving and optional MCP wiring. It must not import
  `mcp/builtin` or `ratelimit`.
- One `ratelimit.New` call creates one quota namespace. Reusing its middleware
  shares quotas; another call isolates them.
- Default limiter identity is the transport peer. Forwarding headers require an
  explicit `TrustedProxyClientKey` built from validated `netip.Prefix`
  ranges.
- `auth` establishes identity. Providers, sessions, roles, and resource
  authorization remain application policy.

See [ADR-0014](docs/0014-root-package-and-concern-subpackages.md) before moving
public APIs across packages.

## Talking to an MCP-enabled application

Start with discovery:

```sh
curl http://localhost:8080/.well-known/mcp.json
```

### Streamable HTTP (MCP 2026-07-28)

```sh
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/list" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":1}'
```

Ordinary requests return finite JSON. Accepted notifications return 202 with
no body. Browser origins must satisfy the default same-origin policy or the
application's `hyperserve.WithMCPOriginValidator`.

Subscribable resource templates use request-scoped SSE:

```sh
curl -N -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: subscriptions/listen" \
  -d '{"jsonrpc":"2.0","method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":["quotes://AAPL"]},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":"quotes"}'
```

Closing the response cancels the request. Server shutdown sends a final
`resultType: complete` response when possible.

Requests without 2026 per-request metadata use the initialize-era 2025-11-25
request/response path. It does not provide 2025 sessions or resumable SSE.

### Deprecated HyperServe routed SSE

This proprietary compatibility transport is not MCP Streamable HTTP. New
clients must not use it. Existing HyperServe-specific clients can temporarily
enable it with `hyperserve.WithMCPLegacyRoutedSSE(true)`.

```sh
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp
```

The initial event provides a `clientId` and `bindingToken`. Routed POSTs
must return both `X-SSE-Client-ID` and `X-SSE-Binding`; the client ID alone
is not authority.

## MCP construction

```go
import (
    "github.com/osauer/hyperserve/v2"
    "github.com/osauer/hyperserve/v2/mcp"
    _ "github.com/osauer/hyperserve/v2/mcp/builtin"
)

app, err := hyperserve.New(
    hyperserve.WithMCPSupport("MyApp", "1.0.0"),
    hyperserve.WithMCPBuiltinTools(true),
    hyperserve.WithMCPBuiltinResources(true),
    hyperserve.WithMCPFileToolRoot("/srv/data"),
    hyperserve.WithMCPDiscoveryPolicy(mcp.DiscoveryCount),
)
```

Builtins are off by default and require the explicit `mcp/builtin` import.
File tools never fall back to an unsandboxed root. Discovery policy controls
what metadata is listed; it is not endpoint authorization.

## Development conventions

- `make check` is the canonical formatting, analysis, vulnerability,
  documentation, example, MCP-conformance, and release-gate fixture check.
- `make test` adds the native unit suite. Use `make test-race` for races and
  `make fuzz-smoke` for fuzz targets.
- Public API changes require focused tests, current documentation, updated
  examples/scaffold output, and a disposable consumer witness.
- Keep the shipped dependency graph separate from `tools/go.mod`.
- Preserve historical CHANGELOG and ADR wording; supersede decisions with a
  new ADR instead of rewriting history.
- Do not claim a release from a commit, tag, or local test alone. Publication
  and fresh-module retrieval require separate evidence.

## References

- [README](README.md)
- [Architecture](ARCHITECTURE.md)
- [MCP guide](docs/MCP_GUIDE.md)
- [Production guide](docs/PRODUCTION.md)
- [API stability](docs/API_STABILITY.md)
- [Migrating to v2.1](docs/MIGRATING_V2_1.md)
