# HyperServe

A secure, high-performance HTTP server framework for Go with native Model Context Protocol (MCP) support.

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Features

- **Zero Dependencies** - Uses only `golang.org/x/time/rate` for rate limiting
- **MCP Native** - Built-in support for AI assistant integration via Model Context Protocol
- **Secure by Default** - FIPS 140-3 mode, TLS 1.3, security headers, origin validation
- **Production Ready** - Graceful shutdown, health checks, structured logging, metrics
- **Developer Friendly** - Hot reload, route inspection, request debugging with MCP tools

## Quick Start

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/osauer/hyperserve"
)

func main() {
    srv, _ := hyperserve.NewServer()
    
    srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })
    
    srv.Run() // Graceful shutdown on SIGINT/SIGTERM
}
```

## MCP Integration

HyperServe provides first-class support for the Model Context Protocol, enabling AI assistants to interact with your server.

### Production Monitoring (DevOps)

Monitor your production servers with minimal overhead:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPDevOpsPreset()),
)
```

Provides secure access to:
- Server configuration (sanitized)
- Health metrics and uptime
- Recent logs (circular buffer)

### Development Mode

Enable AI-assisted development for rapid iteration:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPDeveloperPreset()),
)
```

⚠️ **Development only!** Enables:
- Server restart and reload
- Dynamic log level changes
- Route inspection
- Request capture and replay

### Custom Extensions

Expose your application's functionality through MCP:

```go
extension := hyperserve.NewMCPExtension("blog").
    WithTool(
        hyperserve.NewTool("publish_post").
            WithParameter("title", "string", "Post title", true).
            WithParameter("content", "string", "Post content", true).
            WithExecute(publishPost).
            Build(),
    ).
    WithResource(
        hyperserve.NewResource("blog://posts/recent").
            WithRead(getRecentPosts).
            Build(),
    ).
    Build()

srv.RegisterMCPExtension(extension)
```

## Core Features

### Security

```go
// FIPS 140-3 compliant mode
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
    hyperserve.WithTLS("cert.pem", "key.pem"),
)

// Automatic security headers
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))
```

### Performance

- **Request pooling** - ~10 allocations per request
- **Zero-copy** upgrades for WebSocket
- **os.Root** sandboxing for static files (Go 1.24)
- **Swiss map** implementation for rate limiting

### Middleware

```go
// Built-in middleware stacks
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))     // Auth + rate limiting
srv.AddMiddlewareStack("*", hyperserve.SecureWeb(srv.Options)) // Security headers

// Custom middleware
srv.AddMiddleware("/admin", RequireAdminAuth)
```

### WebSocket Support

```go
upgrader := hyperserve.Upgrader{
    CheckOrigin: hyperserve.SameOriginCheck(),
}

srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, _ := upgrader.Upgrade(w, r, nil)
    defer conn.Close()
    
    // Handle WebSocket connection
})
```

## Configuration

### Options

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithAddr(":3000"),
    hyperserve.WithHealthServer(),         // Kubernetes health checks
    hyperserve.WithRateLimit(100, 1000),   // 100 req/s, burst 1000
    hyperserve.WithTimeouts(5*time.Second, 10*time.Second, 120*time.Second),
)
```

### Environment Variables

```bash
export HS_PORT=3000
export HS_LOG_LEVEL=DEBUG
export HS_HARDENED_MODE=true
```

### Configuration File

```json
{
  "addr": ":3000",
  "tls": true,
  "hardened_mode": true,
  "log_level": "INFO"
}
```

## Examples

- [Hello World](examples/hello-world) - Minimal server
- [JSON API](examples/json-api) - RESTful API with authentication
- [MCP Development](examples/mcp-development) - AI-assisted development
- [MCP Extensions](examples/mcp-extensions) - Custom tools and resources
- [WebSocket Chat](examples/websocket-chat) - Real-time communication
- [Enterprise](examples/enterprise) - FIPS mode with full security

## Performance

| Metric | Value |
|--------|-------|
| Requests/sec | 150,000+ |
| Latency (p99) | <1ms |
| Memory/request | ~1KB |
| Allocations/request | 10 |

Benchmarked on Apple M1, 16GB RAM. See [benchmarks](PERFORMANCE.md) for details.

## Documentation

- [API Reference](https://pkg.go.dev/github.com/osauer/hyperserve)
- [WebSocket Guide](docs/WEBSOCKET_GUIDE.md)
- [Performance Guide](PERFORMANCE.md)
- [Migration Guide](MIGRATION_GUIDE.md)

## License

MIT License - see [LICENSE](LICENSE) for details.