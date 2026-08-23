# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in the HyperServe repo.

## Layout

HyperServe is a Go HTTP framework with built-in Model Context Protocol (MCP)
support. The library lives under `pkg/`:

- `pkg/server` — `net/http`-shaped HTTP server, middleware, options, lifecycle.
- `pkg/mcp` — MCP protocol handler, JSON-RPC dispatch, SSE manager, transports.
- `pkg/mcp/builtin` — opt-in built-in tools (calculator, sandboxed file tools)
  and resources (config, metrics, system, logs). Activated via blank import
  plus `WithMCPBuiltinTools(true)` / `WithMCPBuiltinResources(true)`.
- `pkg/jsonrpc` — JSON-RPC 2.0 engine reused by MCP.
- `pkg/websocket` — RFC 6455 WebSocket implementation.

Supported binary: `cmd/hyperserve-init` (project scaffold). Generated projects
contain their own `cmd/server`; the HyperServe repository does not.

Import the library as `github.com/osauer/hyperserve/pkg/server` — there are
no `.go` files at the repository root.

## Talking to an MCP-enabled HyperServe

If the server logs "MCP ENABLED" at startup, try the discovery endpoint
first:

```bash
curl http://localhost:8080/.well-known/mcp.json
```

It returns transport info and (per policy) tool/resource lists.

### Streamable HTTP (MCP 2026-07-28)

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/list" \
  -d '{"jsonrpc":"2.0","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":1}'
```

The endpoint returns JSON for ordinary requests. It validates the mirrored
protocol/method/name headers, returns 202 with no body for accepted
notifications, and rejects browser Origins that do not match the request Host.
Use `server.WithMCPOriginValidator` for an authenticated cross-origin client.

Resource templates implementing `mcp.SubscribableResourceTemplate` are
available through a request-scoped SSE POST:

```bash
curl -N -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: subscriptions/listen" \
  -d '{"jsonrpc":"2.0","method":"subscriptions/listen","params":{"notifications":{"resourceSubscriptions":["quotes://AAPL"]},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":"quotes"}'
```

The acknowledgement is always first. Close the response to cancel the
subscription; server shutdown sends a final `resultType: complete` response.

Requests without 2026 per-request metadata, or with the configured legacy
protocol header, use the initialize-era 2025-11-25 request/response
compatibility path. It does not implement 2025 sessions or resumable SSE.

### Legacy HyperServe routed SSE

The following is a proprietary compatibility transport, not MCP Streamable
HTTP. New clients must not use it. It is disabled by default; an existing
HyperServe-specific client may temporarily enable it with
`server.WithMCPLegacyRoutedSSE(true)`.

Connect with the SSE Accept header:

```bash
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp
```

The initial `connection` event carries `clientId` and `bindingToken`. POST
JSON-RPC requests to the same `/mcp` endpoint with **both** headers set:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "X-SSE-Client-ID: <clientId>" \
  -H "X-SSE-Binding: <bindingToken>" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

Missing or wrong binding → 403. The binding token is the capability — the
client ID alone is **not** sufficient to inject requests into another
client's stream.

## MCP option summary

```go
srv, _ := server.NewServer(
    server.WithMCPSupport("MyServer", "1.0.0"),
    server.WithMCPBuiltinTools(true),       // off by default
    server.WithMCPBuiltinResources(true),   // off by default
    server.WithMCPFileToolRoot("/srv/data"), // required for file tools
)
```

Built-in tools/resources are off by default. File tools refuse to construct
without a sandbox root; the unsandboxed fallback was deleted.

## Discovery policies

```go
server.WithMCPDiscoveryPolicy(server.DiscoveryCount)         // counts only
server.WithMCPDiscoveryPolicy(server.DiscoveryAuthenticated) // requires auth header
server.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool { ... })
```

## Development conventions

- `make check` is the gate: formatting, vet, staticcheck, vulnerability scans
  for root/tools/examples, modernization, canonical/compatibility examples,
  and official MCP SDK conformance.
- `make test` runs the native check gate plus the unit suite; use
  `make test-race` for the race detector.
- New features ship with: tests, doc comments, example update where relevant.
- Honor library design practices (functional options, narrow exports).

## Reference docs

- [MCP_GUIDE.md](docs/MCP_GUIDE.md) — full MCP reference with namespaces and presets.
- [API_STABILITY.md](docs/API_STABILITY.md) — pre-1.0 deprecation policy.
- [PERFORMANCE.md](docs/PERFORMANCE.md) — benchmark methodology.
- [examples/auth](examples/auth/) — JWT / API-key / Basic auth, role and
  permission gating sourced from validator SessionInfo (not from raw headers).
