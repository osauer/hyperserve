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

Binaries: `cmd/server` (example bundle), `cmd/hyperserve-init` (scaffold).

Import the library as `github.com/osauer/hyperserve/pkg/server` — there are
no `.go` files at the repository root.

## Talking to an MCP-enabled HyperServe

If the server logs "MCP ENABLED" at startup, try the discovery endpoint
first:

```bash
curl http://localhost:8080/.well-known/mcp.json
```

It returns transport info and (per policy) tool/resource lists.

### HTTP transport

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

### SSE transport

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

- `make check` is the gate: gofmt + vet + staticcheck + govulncheck + modernize.
- `make test` runs unit + race-detected tests.
- New features ship with: tests, doc comments, example update where relevant.
- Honor library design practices (functional options, narrow exports).

## Reference docs

- [MCP_GUIDE.md](docs/MCP_GUIDE.md) — full MCP reference with namespaces and presets.
- [API_STABILITY.md](docs/API_STABILITY.md) — pre-1.0 deprecation policy.
- [PERFORMANCE.md](docs/PERFORMANCE.md) — benchmark methodology.
- [examples/auth](examples/auth/) — JWT / API-key / Basic auth, role and
  permission gating sourced from validator SessionInfo (not from raw headers).
