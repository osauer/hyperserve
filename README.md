# HyperServe

HyperServe lets humans and AI assistants co-manage the same production-grade Go services. A zero-dependency, high-performance core ships with native Model Context Protocol (MCP) control surfaces and hardened defaults so you can automate safely from laptop to regulated environments.

## Features

- 🧱 **Project Scaffold** - Generate secure, MCP-ready services via `hyperserve-init`
- 🚀 **Minimal Dependencies** - Only 1 dependency (`golang.org/x/time`)
- 🤖 **MCP Support** - Built-in Model Context Protocol for AI assistants
- 🔌 **WebSocket Support** - Real-time bidirectional communication
- 🛡️ **Security Middleware** - Hardened headers, auth, and rate limiting ready to enable
- 📊 **Observable** - Metrics, health checks, and structured logging
- ⚡ **High Performance** - Optimized for throughput and low latency
- 🔧 **Battle-tested HTTP/2** - Leverages Go's excellent standard library

## Quick Start

> All code samples import the server package with `server "github.com/osauer/hyperserve/pkg/server"`.

```go
import (
    "fmt"
    "net/http"

    server "github.com/osauer/hyperserve/pkg/server"
)

func main() {
    srv, _ := server.NewServer()

    srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })

    srv.Run()
}
```

## Scaffold a New Service

```bash
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

Flags include `--name` (display name), `--out` (output directory), `--with-mcp=false` to opt out of MCP, and `--local-replace` for working against a local HyperServe checkout during development.

## Installation

```bash
go get github.com/osauer/hyperserve/pkg/server
```

## MCP (Model Context Protocol)

HyperServe includes native MCP support, enabling AI assistants to:
- Execute tools and access resources
- Connect via HTTP or Server-Sent Events (SSE)
- Discover capabilities automatically

Enable MCP with environment variables:
```bash
HS_MCP_ENABLED=true
HS_MCP_SERVER_NAME=MyServer
HS_MCP_SERVER_VERSION=1.0.0
```

Or programmatically:
```go
srv, _ := server.NewServer(
    server.WithMCPSupport("MyServer", "1.0.0"),
    server.WithMCPBuiltinTools(true),
    server.WithMCPBuiltinResources(true),
)
```

## Common Middleware

`NewServer` wires in recovery, request logging, and metrics collectors.
Security middleware (headers, auth, rate limiting) can be enabled per route:

```go
srv, _ := server.NewServer()
srv.AddMiddleware("/api", server.RateLimitMiddleware(srv))
srv.AddMiddlewareStack("/web", server.SecureWeb(srv.Options))
```

## Deferred Initialization

Bring the listener up immediately while long-running bootstrap work executes in the background. The server keeps `/healthz` live, returns 503 for application routes, and flips to ready once deferred tasks (and any `WithOnReady` hooks) finish successfully.

```go
srv, _ := server.NewServer(
    server.WithDeferredInit(func(ctx context.Context, app *server.Server) error {
        return warmCaches(ctx) // hydrate databases, load configs, etc.
    }),
    server.WithOnReady(func(ctx context.Context, app *server.Server) error {
        app.HandleFunc("/api/users", usersHandler)
        return nil
    }),
    server.WithBannerColor(true), // optional ANSI color output for the startup banner
)
```

Use `server.WithDeferredInitStopOnFailure(false)` to keep serving health checks when a bootstrap failure should not terminate the process, and `server.CompleteDeferredInit(ctx, nil)` once the issue is resolved to flip the server to ready.

## Examples

See the [examples](./examples) directory for comprehensive examples including:
- Basic HTTP server
- WebSocket implementation
- MCP integration
- Authentication and RBAC
- Enterprise features
- Best practices

## Documentation

- [Architecture](./ARCHITECTURE.md) - Design decisions and system architecture
- [API Specification](./spec/api.md) - Complete API documentation
- [MCP Guide](./docs/MCP_GUIDE.md) - Model Context Protocol integration
- [WebSocket Guide](./docs/WEBSOCKET_GUIDE.md) - WebSocket implementation details
- [Scaffolding Guide](./docs/SCAFFOLDING.md) - `hyperserve-init` usage and templates
- [Bundle Exploration](./docs/BUNDLE_EXPLORATION.md) - Strategy for one-click Regime deployment

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on contributing.

## License

MIT License - see [LICENSE](./LICENSE) for details.
