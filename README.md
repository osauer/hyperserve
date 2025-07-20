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

### Development with Claude Code

Enable AI-assisted development without hardcoding dev tools:

```bash
# Using command-line flags
./myapp --mcp --mcp-dev

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_DEV=true ./myapp
```

#### Claude Code Integration (HTTP)
```json
{
  "mcpServers": {
    "myapp-local": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

#### Discovery Endpoints

HyperServe implements MCP discovery endpoints for automatic configuration:

```bash
# Standard discovery endpoint
curl http://localhost:8080/.well-known/mcp.json

# Alternative discovery endpoint
curl http://localhost:8080/mcp/discover
```

These endpoints return:
- Available transports (HTTP, SSE)
- Server capabilities
- Dynamic tool/resource lists (based on policy)

##### Discovery Security

Control what tools are exposed through discovery:

```go
// Production: Only show counts
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPDiscoveryPolicy(hyperserve.DiscoveryCount),
)

// With RBAC: Custom filter
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
        token := r.Header.Get("Authorization")
        return isAuthorized(token, toolName)
    }),
)
```

#### Server-Sent Events (SSE) Support

HyperServe's MCP endpoint supports both regular HTTP and SSE connections using the **same endpoint**:

```bash
# Regular HTTP requests
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# SSE connection (same endpoint, different header)
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp
```

SSE enables real-time bidirectional communication for advanced use cases like live debugging and monitoring.

#### Claude Desktop Integration (STDIO)
```bash
# Build your app with MCP support
go build -o myapp
```

Configure Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/myapp",
      "args": ["--mcp", "--mcp-dev", "--mcp-transport=stdio"]
    }
  }
}
```

Now Claude can help you:
- "Set the log level to DEBUG to see what's happening"
- "Show me all the registered routes"
- "Capture the next request to /api/users for debugging"
- "List recent error logs"

⚠️ **Development only!** You'll see this warning in logs:
```
⚠️  MCP DEVELOPER MODE ENABLED ⚠️
```

### Production Observability

Monitor your production servers with safe, read-only access:

```bash
./myapp --mcp --mcp-observability
```

Configure Claude for remote monitoring:
```json
{
  "mcpServers": {
    "myapp-prod": {
      "command": "ssh",
      "args": ["prod-server", "curl", "-s", "http://localhost:8080/mcp"],
      "env": {}
    }
  }
}
```

Provides secure access to:
- Server configuration (sanitized, no secrets)
- Health metrics and uptime
- Recent logs (circular buffer)

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

Now Claude can interact with your app:
- "Publish a new blog post about Go generics"
- "Show me the recent blog posts"
- "Update the post titled 'Getting Started'"

## Core Features

### Security

HyperServe is designed with security as a top priority, providing multiple layers of protection:

#### Protection Against Common Attacks

- **Slowloris Protection**: Automatic `ReadHeaderTimeout` prevents slow HTTP attacks
- **Integer Overflow Protection**: Safe WebSocket frame size handling
- **Origin Validation**: Configurable CORS and WebSocket origin checking
- **Rate Limiting**: Built-in protection against DoS attacks

#### Secure Configuration

```go
// FIPS 140-3 compliant mode with comprehensive security settings
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
    hyperserve.WithTLS("cert.pem", "key.pem"),
    // Timeout configurations for attack prevention
    hyperserve.WithReadTimeout(5*time.Second),     // Prevents slow-read attacks
    hyperserve.WithWriteTimeout(10*time.Second),   // Prevents slow-write attacks
    hyperserve.WithIdleTimeout(120*time.Second),   // Closes idle connections
    hyperserve.WithReadHeaderTimeout(5*time.Second), // Prevents Slowloris
)

// Automatic security headers for all routes
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))

// WebSocket with secure origin validation
upgrader := hyperserve.Upgrader{
    CheckOrigin: hyperserve.SameOriginCheck(), // Only allow same-origin connections
    // Or use custom validation:
    // CheckOrigin: func(r *http.Request) bool {
    //     origin := r.Header.Get("Origin")
    //     return origin == "https://trusted-domain.com"
    // },
}
```

#### Security Headers

HyperServe automatically sets security headers when using `HeadersMiddleware`:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security` (when TLS is enabled)
- `Content-Security-Policy` (configurable)

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
    
    // Timeout configurations (all timeouts are optional with sensible defaults)
    hyperserve.WithTimeouts(5*time.Second, 10*time.Second, 120*time.Second),
    // Or configure individually:
    hyperserve.WithReadTimeout(5*time.Second),      // Max time to read request
    hyperserve.WithWriteTimeout(10*time.Second),    // Max time to write response
    hyperserve.WithIdleTimeout(120*time.Second),    // Max time between requests
    hyperserve.WithReadHeaderTimeout(5*time.Second), // Max time to read headers (Slowloris protection)
)
```

#### Timeout Configuration Guide

| Timeout | Default | Purpose | Recommendation |
|---------|---------|---------|----------------|
| ReadTimeout | 30s | Maximum duration for reading entire request | 5-30s depending on expected request size |
| WriteTimeout | 30s | Maximum duration for writing response | 10-60s depending on response size |
| IdleTimeout | 120s | Maximum time to wait for next request | 60-180s for keep-alive connections |
| ReadHeaderTimeout | ReadTimeout | Maximum duration for reading request headers | 5-10s (prevents Slowloris attacks) |

**Note**: Health check endpoints automatically use the same timeouts as the main server for consistency.

### Environment Variables

```bash
export HS_PORT=3000
export HS_LOG_LEVEL=DEBUG
export HS_HARDENED_MODE=true

# MCP Configuration
export HS_MCP_ENABLED=true
export HS_MCP_SERVER_NAME="MyApp"
export HS_MCP_DEV=true              # Development tools
export HS_MCP_OBSERVABILITY=true    # Production monitoring
export HS_MCP_TRANSPORT=http        # or stdio for Claude Desktop
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
- [JSON API](examples/json-api) - RESTful API with JSON handling
- [Auth](examples/auth) - Production-ready authentication (JWT, API Keys, RBAC)
- [MCP Flags](examples/mcp-flags) - Configure MCP via flags/environment
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
- [MCP Integration Guide](docs/MCP_GUIDE.md)
- [Performance Guide](PERFORMANCE.md)
- [Migration Guide](MIGRATION_GUIDE.md)

## License

MIT License - see [LICENSE](LICENSE) for details.