# HyperServe Architecture

## Design Philosophy

HyperServe is a thin layer over `net/http` with one runtime dependency
(`golang.org/x/time`) and a built-in MCP server in the same binary. Everything
else — middleware, WebSocket, JSON-RPC — is in-tree to keep the dependency tree
flat.

## Core Principles

### 1. Single Runtime Dependency
- `golang.org/x/time` for the rate-limiter token bucket.
- Everything else uses the Go standard library.
- `tools/go.mod` owns `golang.org/x/tools`, the official MCP SDK conformance
  dependency, and their transitive graphs, keeping developer tooling out of
  the shipped module graph.

### 2. Standard Library First
- `net/http` for the server, `crypto/tls` for TLS, `os.Root` for the static-file sandbox.
- WebSocket and JSON-RPC are implemented in-tree against the standard library, not pulled from third parties.

### 3. Secure Defaults
- Security headers middleware is available out of the box.
- Rate limiting is opt-in per route, with a periodic cleanup ticker.
- TLS defaults to 1.2+; `WithFIPSMode()` restricts to the FIPS-approved cipher list.

### 4. AI / MCP as a First-Class Surface
- The MCP server lives in `pkg/mcp` and is the differentiator vs `net/http` + a third-party router.
- Current Streamable HTTP uses finite JSON responses and request-scoped SSE
  for `subscriptions/listen`; stdio shares the same handler. Proprietary routed
  SSE is default-off deprecated compatibility behavior.
- Built-in tools and resources are opt-in (`WithMCPBuiltinTools(true)`); they are demos, not production wiring.

## Architecture Overview

### Core Components

#### Server
The main server struct (`Server`) handles:
- HTTP request routing and handling
- WebSocket connections
- Middleware chain execution
- Configuration management

#### Middleware System
Flexible middleware architecture supporting:
- Pre and post-processing
- Authentication and authorization
- Logging and metrics
- Security headers
- Rate limiting

#### MCP (Model Context Protocol)
Native MCP implementation providing:
- Tool registration and execution
- Resource management
- Streamable HTTP (JSON plus request-scoped SSE) and stdio transports
- Discovery endpoints
- Namespace isolation

#### WebSocket Support
RFC 6455 implementation featuring:
- Outbound `ws` and `wss` client dialing
- Binary and text message support
- Automatic ping/pong handling
- Configurable timeouts

### Package Layout

- `pkg/server` — HTTP server, middleware registry, deferred-init lifecycle, MCP wiring options.
- `pkg/mcp` — MCP protocol surface. Standalone — no dependency on `pkg/server`.
- `pkg/mcp/builtin` — Opt-in built-in MCP tools and resources. Depends on both `pkg/server` (for `*Server` access) and `pkg/mcp`.
- `pkg/websocket` — WebSocket server upgrader, outbound client, low-level framing, origin checks.
- `pkg/jsonrpc` — Standalone JSON-RPC 2.0 engine used by `pkg/mcp`.

Dependency direction is one-way: `pkg/mcp/builtin` → `pkg/server` + `pkg/mcp`; `pkg/server` → `pkg/mcp`; `pkg/mcp` → `pkg/jsonrpc`. No cycles.

### Directory Structure

```
/
├── cmd/              # Command-line applications
├── internal/scaffold # Templates backing hyperserve-init
├── pkg/              # Public Go packages
│   ├── server/       # HTTP server, middleware, deferred-init, MCP wiring
│   ├── mcp/          # MCP protocol (handler, transports, discovery, namespaces)
│   │   └── builtin/  # Opt-in built-in MCP tools and resources
│   ├── websocket/    # RFC 6455 WebSocket implementation
│   └── jsonrpc/      # JSON-RPC 2.0 engine
├── examples/         # Self-contained `go run .` examples
├── docs/             # ADRs and guides
├── benchmarks/       # Go micro-benchmarks + wrk load script
└── go.{mod,sum}
```

## Key Design Decisions

- **Standard library first.** Routing is `net/http.ServeMux`. TLS is `crypto/tls`. The static-file sandbox is `os.Root`. WebSocket and JSON-RPC are in-tree against the standard library, not pulled from third parties.
- **Interfaces only where they earn it.** `MiddlewareFunc`, `mcp.Tool`, `mcp.Resource`, `mcp.Transport`, `mcp.Extension` exist because they have multiple implementations or are extension points. Single-implementation interfaces are avoided.
- **Context-aware end-to-end.** Handlers, middleware, deferred-init, shutdown hooks, and MCP tool execution all thread `context.Context`.
- **Errors returned, not panicked.** The recovery middleware exists to catch caller panics, not as a control-flow mechanism.

## Security

- TLS 1.2+ default; `WithFIPSMode()` restricts to the FIPS-approved cipher list.
- Security-header middleware (`SecureWeb`, `SecureAPI`) available out of the box; off by default.
- Rate limiting is per-route and per-client, with a periodic cleanup ticker.
- Static file serving is sandboxed via `os.Root`.
- The MCP discovery filter (`WithMCPDiscoveryFilter`) gates only discovery
  visibility. Authorization middleware must protect the MCP endpoint itself.

## Performance

- One goroutine per connection (stdlib `net/http`).
- Atomic counters for request/latency totals.
- Rate-limiter map uses Swiss Tables.

## Roadmap

Tracked in the issue tracker and CHANGELOG. The next milestone bumps the minimum Go version once `encoding/json/v2` graduates from experimental.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). Run `make check` (vet + staticcheck + modernize + govulncheck) before opening a PR.

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed guidelines.
