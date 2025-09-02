# HyperServe

A lightweight, high-performance HTTP server framework with built-in Model Context Protocol (MCP) support, written in Go.

## Features

- 🚀 **Minimal Dependencies** - Only 1 dependency (`golang.org/x/time`)
- 🤖 **MCP Support** - Built-in Model Context Protocol for AI assistants
- 🔌 **WebSocket Support** - Real-time bidirectional communication
- 🛡️ **Secure by Default** - Built-in security headers and rate limiting
- 📊 **Observable** - Metrics, health checks, and structured logging
- ⚡ **High Performance** - Optimized for throughput and low latency
- 🔧 **Battle-tested HTTP/2** - Leverages Go's excellent standard library

## Quick Start

```go
srv, _ := hyperserve.NewServer()
srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, "Hello, World!")
})
srv.Run()
```

## Installation

```bash
go get github.com/osauer/hyperserve
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
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPBuiltinTools(true),
    hyperserve.WithMCPBuiltinResources(true),
)
```

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

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on contributing.

## License

MIT License - see [LICENSE](./LICENSE) for details.