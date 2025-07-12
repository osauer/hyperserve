# HyperServe

A lightweight, high-performance HTTP server framework for Go with zero external dependencies (except `golang.org/x/time/rate` for rate limiting).

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Quick Start

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/osauer/hyperserve"
)

func main() {
    // Create server with automatic defaults
    srv, _ := hyperserve.NewServer()
    
    // Add a route
    srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })
    
    // Run (includes graceful shutdown, health checks, and more)
    srv.Run()
}
```

## Building

HyperServe uses dynamic version detection at build time:

```bash
# Build with automatic version detection from git tags
make build

# Install globally with version
make install

# Manual build with specific version
go build -ldflags "-X github.com/osauer/hyperserve.Version=v1.0.0" .
```

## Features

### Zero Configuration Features

These features work automatically when you create a server:

| Feature | Description | Details |
|---------|-------------|---------|
| **Graceful Shutdown** | Clean shutdown on SIGINT/SIGTERM | Built into `srv.Run()` |
| **Request Logging** | Structured request logs | Via DefaultMiddleware |
| **Panic Recovery** | Automatic panic handling | Via DefaultMiddleware |
| **Metrics Collection** | Request count and timing | Via DefaultMiddleware |
| **Memory Leak Prevention** | Automatic cleanup | Rate limiter cleanup every 5 minutes |

### Opt-in Features

Add these features as needed:

| Feature | How to Enable | Example |
|---------|---------------|---------|
| **Health Checks** | Kubernetes-ready health endpoints | `hyperserve.WithHealthServer()` |
| **Security Headers** | Add HeadersMiddleware | `srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))` |
| **Web Worker Support** | Enable CSP for blob: URLs | `hyperserve.WithCSPWebWorkerSupport()` |
| **Rate Limiting** | Add RateLimitMiddleware | `srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))` |
| **Authentication** | Configure validator + middleware | See [Authentication](#authentication) |
| **TLS/HTTPS** | WithTLS option | `hyperserve.WithTLS("cert.pem", "key.pem")` |
| **Static Files** | HandleStatic | `srv.HandleStatic("/static/")` |
| **Templates** | HandleTemplate | See [Templates](#templates) |
| **SSE** | Custom handler | See [Server-Sent Events](#server-sent-events-sse) |
| **WebSocket** | Upgrader | See [WebSocket Support](#websocket-support) |
| **MCP Support** | WithMCPSupport | `hyperserve.WithMCPSupport()` |

## Common Patterns

### Basic Web Server

```go
srv, _ := hyperserve.NewServer()

// Serve static files
srv.Options.StaticDir = "./public"
srv.HandleStatic("/")

srv.Run()
```

### Secure API Server

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithHealthServer(),
    hyperserve.WithAuthTokenValidator(validateToken),
)

// Apply secure API middleware stack (auth + rate limiting)
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))

// API routes
srv.HandleFunc("/api/users", getUsersHandler)
srv.HandleFunc("/api/orders", getOrdersHandler)

srv.Run()
```

### Web Application with Security

```go
srv, _ := hyperserve.NewServer()

// Apply secure web middleware stack (security headers)
srv.AddMiddlewareStack("*", hyperserve.SecureWeb(srv.Options))

// Serve static files
srv.Options.StaticDir = "./static"
srv.HandleStatic("/static/")

// Dynamic routes
srv.HandleFunc("/", homeHandler)

srv.Run()
```

### Enterprise Server with FIPS

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
    hyperserve.WithTLS("cert.pem", "key.pem"),
    hyperserve.WithAuthTokenValidator(validateJWT),
)

// Apply security middleware
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))
srv.AddMiddlewareStack("/", hyperserve.SecureWeb(srv.Options))

srv.Run()
```

## Web Worker Support

Modern web applications often use Web Workers for performance optimization. Libraries like Tone.js, PDF.js, and others create Web Workers using `blob:` URLs, which are blocked by default by Content Security Policy (CSP).

### Enable Web Worker Support

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithCSPWebWorkerSupport(),
)

// Apply security headers with Web Worker support
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))
```

This adds `blob:` URLs to the CSP `worker-src` and `child-src` directives, enabling Web Workers while maintaining security.

### Environment Variable

```bash
export HS_CSP_WEB_WORKER_SUPPORT=true
```

### JSON Configuration

```json
{
    "csp_web_worker_support": true
}
```

### Use Cases

- **Audio Applications**: Tone.js, Web Audio API libraries
- **PDF Rendering**: PDF.js and similar libraries
- **Performance Optimization**: Any library using Web Workers with blob: URLs

**Security Note**: Web Worker support is disabled by default. Enable only when needed for your application.

## Configuration

### Configuration Methods (in precedence order)

1. **Functional Options** (recommended)
```go
hyperserve.NewServer(
    hyperserve.WithAddr(":3000"),
    hyperserve.WithRateLimit(200, 400),
    hyperserve.WithCSPWebWorkerSupport(),  // Enable Web Worker support for Tone.js, PDF.js, etc.
)
```

2. **Environment Variables**
```bash
export HS_PORT=3000
export HS_RATE_LIMIT=200
export HS_LOG_LEVEL=debug
export HS_CSP_WEB_WORKER_SUPPORT=true  # Enable Web Worker support
```

3. **JSON File** (`options.json`)
```json
{
    "port": 3000,
    "rateLimit": 200,
    "logLevel": "debug",
    "csp_web_worker_support": true
}
```

### Default Configuration

- Port: `:8080`
- Health server: disabled by default (use `WithHealthServer()` to enable on `:8081`)
- Rate limit: 1 req/s (burst: 10)
- Timeouts: 5s read, 10s write, 120s idle
- Log level: Info

## Middleware

### Default Middleware

Every server automatically includes:
- `MetricsMiddleware` - Request counting and timing
- `RequestLoggerMiddleware` - Structured request logs
- `RecoveryMiddleware` - Panic recovery

### Middleware Stacks

Pre-configured middleware combinations:

**SecureAPI** - For API endpoints:
- `AuthMiddleware` - Bearer token validation
- `RateLimitMiddleware` - Rate limiting per IP

**SecureWeb** - For web applications:
- `HeadersMiddleware` - Security headers (CSP, HSTS, etc.)

**FileServer** - For static file serving:
- `HeadersMiddleware` - Appropriate cache headers

Usage:
```go
// Apply middleware stack to specific routes
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))       // Auth + rate limiting for /api/*
srv.AddMiddlewareStack("*", hyperserve.SecureWeb(srv.Options))  // Security headers for all routes
```

### Individual Middleware

```go
// Add specific middleware to routes
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))      // Global - all routes
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))      // Only /api/* routes
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))         // Only /api/* routes
srv.AddMiddleware("/static", hyperserve.HeadersMiddleware(srv.Options)) // Only /static/* routes
```

**Route-specific middleware**:
- Uses prefix matching: `"/api"` matches `/api`, `/api/users`, `/api/v1/orders`, etc.
- Global middleware uses `"*"` and runs before route-specific middleware
- Multiple middleware for the same route are executed in registration order

### Custom Middleware

```go
func TimingMiddleware(next http.Handler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("Request took %v", time.Since(start))
    }
}

srv.AddMiddleware("/api", TimingMiddleware)
```

## Authentication

```go
// Configure token validator
srv, _ := hyperserve.NewServer(
    hyperserve.WithAuthTokenValidator(func(token string) (bool, error) {
        // Validate token (JWT, database lookup, etc.)
        return isValidToken(token), nil
    }),
)

// Apply auth middleware to protected routes
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))
```

The auth middleware:
- Requires Bearer token in Authorization header
- Uses timing-safe comparison
- Returns 401 for invalid tokens

## Templates

```go
// Configure template directory
srv.Options.TemplateDir = "./templates"

// Static template data
srv.HandleTemplate("/about", "about.html", map[string]string{
    "title": "About Us",
})

// Dynamic template data
srv.HandleFuncDynamic("/user", "user.html", func(r *http.Request) interface{} {
    return map[string]interface{}{
        "username": r.URL.Query().Get("name"),
        "timestamp": time.Now(),
    }
})
```

## Static Files

```go
// Configure static directory
srv.Options.StaticDir = "./static"

// Serve files from /static/
srv.HandleStatic("/static/")
```

Uses Go 1.24's `os.Root` for secure file serving when available.

## Server-Sent Events (SSE)

```go
srv.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-r.Context().Done():
            return
        case <-ticker.C:
            msg := hyperserve.NewSSEMessage(map[string]interface{}{
                "time": time.Now(),
                "data": "update",
            })
            fmt.Fprintf(w, "%s", msg)
            w.(http.Flusher).Flush()
        }
    }
})
```

## WebSocket Support

hyperserve provides secure, RFC 6455-compliant WebSocket support with zero dependencies.

```go
upgrader := hyperserve.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // Configure based on your security needs
    },
}

srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print("upgrade error:", err)
        return
    }
    defer conn.Close()
    
    // Handle WebSocket messages
    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            break
        }
        // Echo message back
        conn.WriteMessage(messageType, p)
    }
})
```

See the [WebSocket Implementation Guide](docs/WEBSOCKET_GUIDE.md) for security features, middleware compatibility, and best practices.

## Go 1.24 Features

### FIPS 140-3 Mode

For government and regulated industries:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
)
```

### Encrypted Client Hello (ECH)

Protect user privacy:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithEncryptedClientHello(echKeys...),
)
```

### Performance Features

- **Swiss Tables**: Faster map implementation for rate limiting
- **os.Root**: Secure file serving with automatic sandboxing
- **Post-Quantum**: X25519MLKEM768 enabled by default (non-FIPS)

## Model Context Protocol (MCP)

Enable AI assistant integration with multiple transport options:

### HTTP Transport (Default)

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport("MyApp", "1.0.0"),  // Enable MCP with server info
    hyperserve.WithMCPBuiltinTools(true),      // Enable built-in tools (disabled by default)
    hyperserve.WithMCPBuiltinResources(true),  // Enable built-in resources (disabled by default)
    hyperserve.WithMCPFileToolRoot("/safe/path"),
)
```

### STDIO Transport

For CLI tools and Claude Desktop integration:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(hyperserve.MCPOverStdio()),
    hyperserve.WithMCPBuiltinTools(true),      // Enable built-in tools
    hyperserve.WithMCPFileToolRoot("/safe/path"),
)
```

See [mcp-stdio example](examples/mcp-stdio) for Claude Desktop integration.

### Built-in Tools and Resources

**Important**: Built-in tools and resources are **disabled by default** for security. You must explicitly enable them:

```go
// Enable only MCP protocol (no built-in tools/resources)
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(hyperserve.MCPServerInfo("MyApp", "1.0.0")),
)

// Alternative: Use the convenience function
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPServer("MyApp", "1.0.0"),  // Same as WithMCPSupport(MCPServerInfo(...))
)

// Enable MCP with built-in tools
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(hyperserve.MCPServerInfo("MyApp", "1.0.0")),
    hyperserve.WithMCPBuiltinTools(true),
)

// Enable MCP with built-in resources
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(hyperserve.MCPServerInfo("MyApp", "1.0.0")),
    hyperserve.WithMCPBuiltinResources(true),
)
```

#### Available Built-in Tools (when enabled)

- `read_file` - Read files (sandboxed)
- `list_directory` - List directories (sandboxed)
- `http_request` - Make HTTP requests
- `calculator` - Basic math operations

All tools support context-based cancellation and have a 30-second timeout by default.

#### Available Built-in Resources (when enabled)

- `config://server/options` - Server configuration
- `metrics://server/stats` - Performance metrics
- `system://runtime/info` - System information
- `logs://server/recent` - Recent log entries

Resources are automatically cached with a 5-minute TTL to improve performance.

### Custom MCP Tools and Resources

Register your own tools and resources:

```go
// Define a custom tool
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Custom tool" }
func (t *MyTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{"type": "string"},
        },
    }
}
func (t *MyTool) Execute(params map[string]interface{}) (interface{}, error) {
    // Implementation
    return map[string]interface{}{"result": "success"}, nil
}

// Register after server creation
srv, _ := hyperserve.NewServer(hyperserve.WithMCPSupport())
srv.RegisterMCPTool(&MyTool{})
srv.Run()
```

### MCP Performance Features

HyperServe's MCP implementation includes several performance optimizations:

- **Context Support**: All tools support cancellation with 30-second timeout
- **Resource Caching**: Automatic 5-minute cache for resource reads
- **Metrics Collection**: Detailed performance metrics for monitoring
- **Concurrent Execution**: Tools can execute concurrently for better throughput

Access metrics programmatically:
```go
if srv.MCPEnabled() {
    metrics := srv.mcpHandler.GetMetrics()
    // Returns request counts, latencies, error rates, cache stats
}
```

## Performance

Baseline performance characteristics:

| Metric | Value |
|--------|-------|
| Allocations per request | 10 |
| Memory per request | ~1KB |
| Baseline latency | 180ns |
| With security middleware | +30% |

See [PERFORMANCE.md](PERFORMANCE.md) for detailed benchmarks.

## Examples

Complete example applications:

- [hello-world](examples/hello-world) - Minimal server
- [best-practices](examples/best-practices) - Demonstrates proper usage patterns and anti-patterns to avoid
- [middleware-basics](examples/middleware-basics) - Middleware patterns
- [static-files](examples/static-files) - Static file serving
- [json-api](examples/json-api) - REST API example
- [htmx-stream](examples/htmx-stream) - SSE with HTMX
- [enterprise](examples/enterprise) - FIPS and security features
- [mcp](examples/mcp) - AI assistant integration (HTTP transport)
- [mcp-stdio](examples/mcp-stdio) - MCP with STDIO transport for Claude Desktop
- [chaos](examples/chaos) - Chaos engineering

## Testing

```bash
# Run all tests
go test ./...

# With race detection
go test -race ./...

# With coverage
go test -cover ./...

# Benchmarks
go test -bench=. -benchmem
```

## Documentation

- [PERFORMANCE.md](PERFORMANCE.md) - Performance analysis
- [CHANGELOG.md](CHANGELOG.md) - Version history
- [API_STABILITY.md](API_STABILITY.md) - API guarantees
- [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) - Go 1.24 migration
- [Architecture Decision Records](docs/adr/) - Design decisions
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines

## License

HyperServe is released under the [MIT License](LICENSE).