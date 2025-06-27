# HyperServe 🚀

A lightweight, high-performance HTTP server framework for Go with zero external dependencies (except `golang.org/x/time/rate` for rate limiting).

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## Features

### 🚀 Performance & Simplicity
- **Zero external dependencies** - Pure Go implementation (only uses `golang.org/x/time/rate`)
- **Flexible middleware system** - Route-specific and global middleware chains
- **Built-in static file serving** - Efficient static content delivery
- **Template engine integration** - Dynamic HTML rendering with Go templates
- **Graceful shutdown** - Proper cleanup and connection draining

### 🔒 Security & Reliability
- **TLS support** - HTTPS with modern cipher suites, post-quantum ready (X25519MLKEM768)
- **FIPS 140-3 mode** - Government-grade cryptographic compliance for enterprise deployments
- **Encrypted Client Hello** - Enhanced privacy by encrypting SNI in TLS handshakes
- **Rate limiting** - Optimized for Go 1.24's Swiss Tables with automatic cleanup
- **Timing attack protection** - Constant-time authentication using `crypto/subtle`
- **Secure file serving** - Uses `os.Root` for sandboxed directory access (Go 1.24)
- **Security headers** - Modern 2024 security headers including CORS, CSP, HSTS, Cross-Origin policies, and Permissions-Policy
- **Health checks** - Kubernetes-ready health endpoints (`/healthz`, `/readyz`, `/livez`)
- **Request tracing** - Built-in trace ID generation for distributed systems
- **Panic recovery** - Automatic recovery from handler panics

### 🎯 Developer Experience
- **Multiple configuration methods** - Environment variables, JSON files, or code
- **Structured logging** - Using Go's `slog` package
- **Metrics collection** - Request count and latency tracking with automatic cleanup
- **Memory management** - Built-in cleanup mechanisms prevent memory leaks from rate limiters
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
    srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))

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
export HS_CHAOS_MODE=false  # Default: false (production safe)
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
    hyperserve.WithFIPSMode(), // Enable FIPS 140-3 compliance
    hyperserve.WithEncryptedClientHello(echKeys...), // Enable ECH
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
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))

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

💡 **Auth Integration**: For authentication setup, see [Authentication](#authentication) and [Token Validation](#token-validation) sections.

### Middleware Stacks

```go
// Pre-configured stacks
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))
srv.AddMiddlewareStack("/", hyperserve.SecureWeb(srv.Options))
```

### Rate Limiting Headers

The rate limiting middleware automatically adds informative headers to help clients understand their current rate limit status:

- `X-RateLimit-Limit`: The maximum number of requests allowed per second
- `X-RateLimit-Remaining`: The number of requests remaining in the current window
- `X-RateLimit-Reset`: Unix timestamp when the rate limit resets
- `Retry-After`: Seconds to wait before retrying when rate limited (429 responses)

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
        // Example: validate JWT, check database, verify API key
        if token == "valid-secret-token" {
            return true, nil
        }
        return false, nil
    }),
)

// Apply auth middleware to protected routes
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))
```

💡 **See also**: [Token Validation](#token-validation) for detailed implementation examples, [Security Headers](#-security--reliability) for additional protection, and [Middleware Stacks](#middleware-stacks) for combining auth with other security features.

### Token Validation

Token validation is a critical security component that determines whether incoming authentication tokens are legitimate. The validator function acts as the gatekeeper for protected routes, ensuring only authenticated requests can access secured endpoints. Proper token validation helps prevent unauthorized access and maintains application security.

The authentication middleware requires a token validator function to be configured. This function receives the bearer token and should return whether it's valid:

- Return `(true, nil)` for valid tokens
- Return `(false, nil)` for invalid tokens  
- Return `(false, error)` for validation errors

Example implementations:

```go
// Simple API key validation
WithAuthTokenValidator(func(token string) (bool, error) {
    validTokens := map[string]bool{
        "api-key-123": true,
        "secret-token": true,
    }
    return validTokens[token], nil
})

// Comprehensive JWT validation with error handling
WithAuthTokenValidator(func(token string) (bool, error) {
    parsedToken, err := jwt.Parse(token, keyFunc)
    if err != nil {
        // Handle specific JWT errors
        if ve, ok := err.(*jwt.ValidationError); ok {
            switch {
            case ve.Errors&jwt.ValidationErrorMalformed != 0:
                return false, fmt.Errorf("malformed token")
            case ve.Errors&jwt.ValidationErrorExpired != 0:
                return false, fmt.Errorf("token expired")
            case ve.Errors&jwt.ValidationErrorNotValidYet != 0:
                return false, fmt.Errorf("token not valid yet")
            default:
                return false, fmt.Errorf("token validation failed: %v", err)
            }
        }
        return false, err
    }
    
    // Validate custom claims
    if claims, ok := parsedToken.Claims.(jwt.MapClaims); ok && parsedToken.Valid {
        // Check required claims
        if !claims.VerifyAudience("your-app", true) {
            return false, fmt.Errorf("invalid audience")
        }
        return true, nil
    }
    
    return false, fmt.Errorf("invalid token claims")
})
```

💡 **Related**: This pairs with [Rate Limiting](#rate-limiting-headers) to prevent brute force attacks and [Security Headers](#-security--reliability) for defense in depth.

## Go 1.24 Features

HyperServe leverages cutting-edge Go 1.24 features for enhanced performance and security:

### FIPS 140-3 Compliance

Enable FIPS mode for government and regulated industry deployments:

```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
)
```

This enables:
- FIPS-approved cipher suites only
- Restricted elliptic curves (P256, P384)
- GOFIPS140 runtime mode
- Compliance logging

### Encrypted Client Hello (ECH)

Protect user privacy by encrypting the SNI:

```go
echKeys := [][]byte{primaryKey, backupKey}
srv, _ := hyperserve.NewServer(
    hyperserve.WithEncryptedClientHello(echKeys...),
)
```

### Post-Quantum Cryptography

HyperServe automatically enables X25519MLKEM768 key exchange when not in FIPS mode, providing protection against future quantum computer attacks.

### Performance Optimizations

- **Swiss Tables**: Rate limiting uses Go 1.24's faster map implementation
- **os.Root**: Secure, sandboxed file serving prevents directory traversal
- **Timing Protection**: Authentication uses `crypto/subtle.WithDataIndependentTiming`

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

- **[enterprise](examples/enterprise)** - Enterprise security with FIPS 140-3 and Go 1.24 features
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

## Documentation

For comprehensive project documentation, see:

- **[CHANGELOG.md](CHANGELOG.md)** - Version history and release notes following semantic versioning
- **[API_STABILITY.md](API_STABILITY.md)** - API stability commitments and backward compatibility promises
- **[MIGRATION_GUIDE.md](MIGRATION_GUIDE.md)** - Guide for migrating to Go 1.24 features
- **[RELEASE_NOTES.md](RELEASE_NOTES.md)** - Detailed release notes with upgrade instructions
- **[CONTRIBUTING.md](CONTRIBUTING.md)** - Guidelines for contributing to the project

## Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

## License

HyperServe is released under the [MIT License](LICENSE).