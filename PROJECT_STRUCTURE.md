# HyperServe Project Structure

## Directory layout

```text
hyperserve/
├── *.go                   # Root hyperserve package and tests
├── auth/                  # Provider-neutral authentication and principals
├── jsonrpc/               # JSON-RPC 2.0 engine
├── mcp/                   # MCP protocol surface
│   └── builtin/           # Opt-in built-in MCP tools and resources
├── ratelimit/             # Bounded HTTP rate-limit middleware
├── websocket/             # RFC 6455 server and client
├── cmd/
│   └── hyperserve-init/   # Project scaffolding CLI
├── internal/
│   ├── scaffold/          # Templates and generator behind hyperserve-init
│   ├── doccheck/          # Current-document and example assertions
│   └── validate/          # Struct-tag validator shared by root and MCP
├── examples/              # Self-contained runnable examples
├── benchmarks/            # Loopback load harness
├── docs/                  # ADRs, migration guides, and focused references
├── scripts/               # Release and repository checks
├── tools/                 # Separate developer-tool module
└── go.{mod,sum}
```

## Public packages

| Directory | Import path | Responsibility |
|---|---|---|
| repository root | `github.com/osauer/hyperserve/v2` | HTTP server, middleware, lifecycle, typed input, pages, SSE, and MCP wiring |
| `auth/` | `github.com/osauer/hyperserve/v2/auth` | Authentication boundary and stable principals |
| `jsonrpc/` | `github.com/osauer/hyperserve/v2/jsonrpc` | Standalone JSON-RPC 2.0 |
| `mcp/` | `github.com/osauer/hyperserve/v2/mcp` | MCP handler, transports, discovery, tools, and resources |
| `mcp/builtin/` | `github.com/osauer/hyperserve/v2/mcp/builtin` | Opt-in built-in tools and resources |
| `ratelimit/` | `github.com/osauer/hyperserve/v2/ratelimit` | Bounded rate-limit middleware and trusted-proxy client keys |
| `websocket/` | `github.com/osauer/hyperserve/v2/websocket` | WebSocket upgrade, framing, connections, and outbound dialing |

Canonical imports:

```go
import (
    "github.com/osauer/hyperserve/v2"
    "github.com/osauer/hyperserve/v2/auth"
    "github.com/osauer/hyperserve/v2/jsonrpc"
    "github.com/osauer/hyperserve/v2/mcp"
    _ "github.com/osauer/hyperserve/v2/mcp/builtin" // only when builtin presets are enabled
    "github.com/osauer/hyperserve/v2/ratelimit"
    "github.com/osauer/hyperserve/v2/websocket"
)
```

There are no public `pkg/...` forwarding packages. The root package does not
import `mcp/builtin` or `ratelimit`; see
[Architecture](./ARCHITECTURE.md) for the cycle-free dependency graph.

## Repository authority

- `go.mod` and `go.sum` define the shipped module and its single external
  runtime dependency.
- `tools/go.mod` and `tools/go.sum` isolate modernization and MCP
  conformance dependencies from applications.
- `Makefile` is the canonical local check, build, and release entry point.
- `README.md` is the adoption and first-run path.
- `docs/API_STABILITY.md` defines the compatibility promise.
- `CHANGELOG.md` records release history; historical entries retain the API
  names that were correct when published.

## Building and testing

```sh
make build
make test
make test-race
make fuzz-smoke
make check
go test -run '^$' -bench . -benchmem .
make benchmark-load
```

`make check` covers formatting, vetting, static analysis, vulnerability
checks, documentation/example assertions, MCP conformance, and the exact-SHA
release-gate fixtures.
