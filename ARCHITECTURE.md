# HyperServe Architecture

## Design Philosophy

HyperServe is an integrated server boundary built on `net/http`. It preserves
standard routes and handler shapes while keeping lifecycle, request binding,
security middleware, observability, WebSocket, JSON-RPC, and optional MCP in one
module.

The runtime module has one external dependency, `golang.org/x/time`. WebSocket,
JSON-RPC, and MCP are implemented in-tree. Callers therefore assemble fewer
packages, while HyperServe takes responsibility for more protocol conformance
and security work. That trade-off is deliberate and must remain visible.

## Core Principles

### 1. Small Runtime Graph

- `golang.org/x/time` for the rate-limiter token bucket.
- Everything else uses the Go standard library.
- `tools/go.mod` owns `golang.org/x/tools`, the official MCP SDK conformance
  dependency, and their transitive graphs, keeping developer tooling out of
  the shipped module graph.

### 2. Standard Library Shapes

- `net/http` owns routing and handler contracts; `crypto/tls` owns TLS; `os.Root`
  confines static-file access.
- WebSocket and JSON-RPC are implemented in-tree against the standard library.
- Applications can use ordinary `http.Handler` middleware without an adapter.

### 3. Caller-Owned Authority

- `NewServer` is deterministic. File and environment configuration require
  explicit `WithConfigFile` or `WithEnvironment` options.
- The application owns process signals and passes cancellation to `Run(ctx)`.
- Security headers middleware is available but remains an application choice.
- `pkg/auth` establishes issuer/subject identity; providers, sessions, roles,
  and resource authorization remain application choices.
- Rate limiting is opt-in per route, with a periodic cleanup ticker.
- TLS defaults to 1.2+; `WithFIPSMode()` restricts to the FIPS-approved cipher list.

### 4. MCP Is First-Class and Optional

- The MCP implementation lives in `pkg/mcp`; HTTP and WebSocket users do not
  need to enable it.
- Services that do enable MCP can mount it in the same process without changing
  their existing `net/http` handlers.
- Current Streamable HTTP uses finite JSON responses and request-scoped SSE
  for `subscriptions/listen`; stdio shares the same handler. Proprietary routed
  SSE is default-off deprecated compatibility behavior.
- Built-in tools and resources are opt-in (`WithMCPBuiltinTools(true)`); they are
  demonstrations, not an authorization policy.

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
- Authentication and application-owned authorization
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
- `pkg/mcp/builtin` — Opt-in built-in MCP tools and resources. Depends on both
  `pkg/server` (for `*Server` access) and `pkg/mcp`.
- `pkg/websocket` — WebSocket server upgrader, outbound client, low-level framing, origin checks.
- `pkg/jsonrpc` — Standalone JSON-RPC 2.0 engine used by `pkg/mcp`.

Dependency direction is one-way: `pkg/mcp/builtin` → `pkg/server` + `pkg/mcp`;
`pkg/server` → `pkg/mcp`; `pkg/mcp` → `pkg/jsonrpc`. No cycles.

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
├── benchmarks/       # Benchmark methodology and supporting material
└── go.{mod,sum}
```

## Key Design Decisions

- **Standard library shapes.** Routing is `net/http.ServeMux`, handlers remain
  `http.Handler` values, TLS uses `crypto/tls`, and static-file access uses
  `os.Root`. WebSocket and JSON-RPC are maintained in-tree.
- **Interfaces only where they earn it.** `auth.Authenticator`, `mcp.Tool`,
  `mcp.Resource`, `mcp.Transport`, and `mcp.Extension` have multiple
  implementations or are extension points. Single-implementation interfaces are
  avoided.
- **Context-aware end-to-end.** Handlers, middleware, deferred initialization,
  shutdown hooks, and MCP tool execution all thread `context.Context`.
- **Errors returned, not panicked.** Recovery middleware catches caller panics;
  HyperServe does not use panics for control flow.

## Security

- TLS 1.2+ default; `WithFIPSMode()` restricts to the FIPS-approved cipher list.
- Security headers, bearer extraction, and identity requirements are separate
  opt-in middleware; application authorization stays outside the library.
- Rate limiting is per-route and per-client, with a periodic cleanup ticker.
- Static-file serving uses `os.Root` and fails closed if the configured root
  cannot be opened.
- The MCP discovery filter (`WithMCPDiscoveryFilter`) gates only discovery
  visibility. Authorization middleware must protect the MCP endpoint itself.

## Performance Evidence

Repository microbenchmarks are comparison tools, not universal throughput
claims. Results must identify the commit, Go version, host, command, and workload
that produced them. See [Performance and benchmarking](./docs/PERFORMANCE.md).

## Roadmap

See [ROADMAP.md](./docs/ROADMAP.md) and the issue tracker.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the local workflow. Run `make check`
before opening a pull request.
