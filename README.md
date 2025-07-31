# HyperServe

A lightweight, high-performance HTTP server framework with built-in Model Context Protocol (MCP) support. Available in both Go and Rust implementations with feature parity.

## Features

- 🚀 **Minimal Dependencies** - Go version has only 1 dependency, Rust version is zero-dependency
- 🤖 **MCP Support** - Built-in Model Context Protocol for AI assistants
- 🔌 **WebSocket Support** - Real-time bidirectional communication
- 🛡️ **Secure by Default** - Built-in security headers and rate limiting
- 📊 **Observable** - Metrics, health checks, and structured logging
- ⚡ **High Performance** - Optimized for throughput and low latency

## Choose Your Implementation

### [Go Implementation](./go/)
- Single dependency (`golang.org/x/time`)
- Leverages Go's excellent standard library
- Simple concurrency with goroutines
- Battle-tested HTTP/2 support

### [Rust Implementation](./rust/)
- Zero dependencies - everything built from scratch
- Memory safe without garbage collection
- Ideal for embedded and resource-constrained environments
- Rust 2024 Edition with latest language features

Both implementations provide the same API and features. Choose based on your ecosystem and requirements.

## Quick Start

### Go
```go
srv, _ := hyperserve.NewServer()
srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintln(w, "Hello, World!")
})
srv.Run()
```

### Rust
```rust
HyperServe::new("127.0.0.1:8080")?
    .handle_func("/", |_req| {
        Response::new(Status::Ok).body("Hello, World!")
    })
    .run()
```

## MCP (Model Context Protocol)

Both implementations support MCP, enabling AI assistants to:
- Execute tools and access resources
- Connect via HTTP or Server-Sent Events (SSE)
- Discover capabilities automatically

Enable MCP with environment variables:
```bash
HS_MCP_ENABLED=true
HS_MCP_SERVER_NAME=MyServer
HS_MCP_SERVER_VERSION=1.0.0
```

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md) for design decisions and rationale behind maintaining two implementations.

## API Specification

Both implementations conform to the same [API specification](./spec/api.md) ensuring feature parity and compatibility.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on contributing to either implementation.

## License

MIT License - see [LICENSE](./LICENSE) for details.

## Benchmarks

See [benchmarks documentation](./docs/benchmarks.md) for performance comparisons between implementations.