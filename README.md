# Hyperserve 🚀

A high-performance, dependency-free, and secure HTTP server for the modern web, built with Go.

## Key Features

### Performance
- **Optimized HTTP Handling**: Lightweight server using Go's standard library.
- **Middleware Support**: Easily extend server functionality with a middleware stack.
- **Configurable Rate Limiting**: Per-second burst configuration to avoid overloads.

### Security
- **Built-in TLS Support**: Enables HTTPS with ease, with customizable key and cert paths.
- **Health Checks**: Pre-configured endpoints (`/healthz`, `/readyz`, `/livez`) for monitoring.
- **Secure Defaults**: Designed with best practices, including CSRF protection and request validation.

### Simplicity and Flexibility
- **Dynamic Templates**: Support for dynamic HTML rendering using Go templates.
- **Static File Serving**: Serve static files efficiently from a configurable directory.
- **Zero External Dependencies**: Pure Go implementation to ensure lightweight and portable builds.

### Optional Features
- **HTMX 2.x Compatibility**: Fully compatible with HTMX for seamless hypermedia-driven development.
- **Server-Sent Events (SSE)**: Native SSE support for real-time updates without external dependencies.

## Getting Started

1. **Install Hyperserve**
   Ensure you have Go 1.23+ installed. Then clone the repository:
   ```bash
   git clone https://github.com/osauer/hyperserve.git
   cd hyperserve
   ```

2. **Run the Server**
   Execute the sample server:
   ```bash
   go run server.go
   ```

3. **Customize**
   Edit `server.go` to customize middleware, routes, and templates.

## Example Usage

### Simple Route
```go
server.Handle("/hello", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, "Hello, World!")
})
```

### Enable TLS
```go
server.WithTLS(certFile: "path/to/cert.pem", keyFile: "path/to/key.pem")
```

### Middleware Example
```go
server.AddMiddleware("/api", func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println("Processing request...")
        next.ServeHTTP(w, r)
    })
})
```

## HTMX and SSE Integration
Leverage HTMX 2.x and the SSE extension for real-time applications:
```html
<div hx-ext="sse" sse-connect="/events" sse-swap="message">
    Real-time updates here
</div>
```

## Contributing

We welcome contributions to improve Hyperserve. Please follow our [Contribution Guidelines](CONTRIBUTING.md).

## License

Hyperserve is released under the MIT License.
