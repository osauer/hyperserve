# HyperServe 🚀

A lightweight, high-performance HTTP server framework for Go with zero external dependencies (except `golang.org/x/time/rate` for rate limiting).

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Features

### 🚀 Performance & Simplicity
- **Zero external dependencies** - Pure Go implementation (only uses `golang.org/x/time/rate`)
- **Flexible middleware system** - Route-specific and global middleware chains
- **Built-in static file serving** - Efficient static content delivery
- **Template engine integration** - Dynamic HTML rendering with Go templates
- **Graceful shutdown** - Proper cleanup and connection draining

### 🔒 Security & Reliability
- **TLS support** - Easy HTTPS configuration with modern cipher suites
- **Rate limiting** - Token bucket algorithm per client IP
- **Security headers** - CORS, CSP, HSTS, and more built-in
- **Health checks** - Kubernetes-ready health endpoints (`/healthz`, `/readyz`, `/livez`)
- **Request tracing** - Built-in trace ID generation for distributed systems
- **Panic recovery** - Automatic recovery from handler panics

### 🎯 Developer Experience
- **Multiple configuration methods** - Environment variables, JSON files, or code
- **Structured logging** - Using Go's `slog` package
- **Metrics collection** - Request count and latency tracking
- **Server-Sent Events (SSE)** - Real-time streaming support
- **Chaos engineering** - Built-in chaos mode for resilience testing

## Installation

```bash
go get github.com/osauer/hyperserve
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    
    "github.com/osauer/hyperserve"
)

func main() {
    // Create a new server
    srv, err := hyperserve.NewServer(
        hyperserve.WithAddr(":8080"),
        hyperserve.WithHealthServer(),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Add middleware
    srv.AddMiddleware("*", hyperserve.MetricsMiddleware(srv))
    srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv.Options))

    // Add routes
    srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "Welcome to HyperServe!")
    })

    // Start the server
    if err := srv.Run(); err != nil {
        log.Fatal(err)
    }
}
```

## Configuration

HyperServe supports three configuration methods (in order of precedence):

1. **Environment Variables** (prefix: `HS_`)
2. **JSON Configuration File**
3. **Default Values**

### Environment Variables

```bash
export HS_PORT=8080
export HS_RATE_LIMIT=100
export HS_BURST_LIMIT=200
export HS_LOG_LEVEL=info
export HS_CHAOS_MODE=false
export HS_TLS_CERT_FILE=/path/to/cert.pem
export HS_TLS_KEY_FILE=/path/to/key.pem
```

### JSON Configuration

```json
{
    "port": 8080,
    "rateLimit": 100,
    "burstLimit": 200,
    "logLevel": "info",
    "enableTLS": true,
    "certFile": "/path/to/cert.pem",
    "keyFile": "/path/to/key.pem"
}
```

### Programmatic Configuration

```go
srv, err := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithTLS("cert.pem", "key.pem"),
    hyperserve.WithRateLimit(100, 200),
    hyperserve.WithTimeouts(30*time.Second, 30*time.Second, 120*time.Second),
    hyperserve.WithHealthServer(),
)
```

## Middleware

### Built-in Middleware

```go
// Logging and metrics
srv.AddMiddleware("*", hyperserve.RequestLoggerMiddleware)
srv.AddMiddleware("*", hyperserve.MetricsMiddleware(srv))

// Security
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))

// Rate limiting
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv.Options))

// Recovery and tracing
srv.AddMiddleware("*", hyperserve.RecoveryMiddleware)
srv.AddMiddleware("*", hyperserve.TraceMiddleware)
```

### Custom Middleware

```go
func CustomMiddleware(next http.Handler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Before request
        log.Printf("Request: %s %s", r.Method, r.URL.Path)
        
        // Call next handler
        next.ServeHTTP(w, r)
        
        // After request
        log.Printf("Request completed")
    }
}

srv.AddMiddleware("/api", CustomMiddleware)
```

### Middleware Stacks

```go
// Pre-configured stacks
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv.Options))
srv.AddMiddlewareStack("/", hyperserve.SecureWeb(srv.Options))
```

## Templates

```go
// Configure template directory
srv.Options.TemplateDir = "./templates"

// Serve static template
srv.HandleTemplate("/about", "about.html", map[string]string{
    "title": "About Us",
    "content": "Welcome to our site",
})

// Dynamic template with data function
srv.HandleFuncDynamic("/user", "user.html", func(r *http.Request) interface{} {
    return map[string]interface{}{
        "username": r.URL.Query().Get("name"),
        "timestamp": time.Now().Format(time.RFC3339),
    }
})
```

## Static Files

```go
// Configure static directory
srv.Options.StaticDir = "./static"

// Serve static files
srv.HandleStatic("/static/")
```

## Server-Sent Events (SSE)

```go
srv.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Send events
    for i := 0; i < 10; i++ {
        msg := hyperserve.NewSSEMessage(map[string]interface{}{
            "count": i,
            "time": time.Now().Format(time.RFC3339),
        })
        
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
        w.(http.Flusher).Flush()
        
        time.Sleep(time.Second)
    }
})
```

## Authentication

```go
// Configure token validator
srv, _ := hyperserve.NewServer(
    hyperserve.WithAuthTokenValidator(func(token string) (bool, error) {
        // Implement your token validation logic
        return token == "valid-secret-token", nil
    }),
)

// Apply auth middleware to protected routes
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))
```

## Health Checks

When health server is enabled, the following endpoints are available on a separate port (default: `:8081`):

- `/healthz` - Overall health status
- `/readyz` - Readiness probe
- `/livez` - Liveness probe

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithHealthServer(),
)
```

## Examples

Check out the `examples` directory for complete examples:

- **[htmx-dynamic](examples/htmx-dynamic)** - Dynamic content with HTMX 2.x
- **[htmx-stream](examples/htmx-stream)** - Server-Sent Events with HTMX
- **[chaos](examples/chaos)** - Chaos engineering demonstration
- **[auth](examples/auth)** - Authentication example

## Testing

```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run with coverage
go test -cover ./...
```

## Chaos Mode

Enable chaos mode for testing application resilience:

```bash
export HS_CHAOS_MODE=true
export HS_CHAOS_ERROR_RATE=0.1
export HS_CHAOS_THROTTLE_RATE=0.05
export HS_CHAOS_MIN_LATENCY=100ms
export HS_CHAOS_MAX_LATENCY=500ms
```

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

## License

HyperServe is released under the [MIT License](LICENSE).