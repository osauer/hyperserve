# HyperServe Architecture

## Design Philosophy

HyperServe is built with a focus on simplicity, performance, and maintainability. By leveraging Go's excellent standard library and minimal external dependencies, we achieve a robust HTTP server framework that is both powerful and easy to understand.

## Core Principles

### 1. Minimal Dependencies
- Single external dependency: `golang.org/x/time` for rate limiting
- Maximizes use of Go's standard library
- Reduces security surface area and maintenance burden

### 2. Performance First
- Leverages Go's efficient goroutine model for concurrency
- Zero-allocation patterns where possible
- Optimized for both throughput and latency

### 3. Security by Default
- Built-in security headers
- Rate limiting out of the box
- Secure defaults for all configurations

### 4. Developer Experience
- Simple, intuitive API
- Comprehensive examples
- Clear error messages and logging

## Architecture Overview

### Core Components

#### Server
The main server struct (`Server`) handles:
- HTTP request routing and handling
- WebSocket connections
- Middleware chain execution
- Configuration management

#### Middleware System
Flexible middleware architecture supporting:
- Pre and post-processing
- Authentication and authorization
- Logging and metrics
- Security headers
- Rate limiting

#### MCP (Model Context Protocol)
Native MCP implementation providing:
- Tool registration and execution
- Resource management
- Multiple transport support (HTTP, SSE, stdio)
- Discovery endpoints
- Namespace isolation

#### WebSocket Support
Full WebSocket implementation featuring:
- Connection pooling
- Binary and text message support
- Automatic ping/pong handling
- Configurable timeouts
- Per-connection rate limiting

### Package Layout
- `pkg/server`: public HTTP server surface, middleware registry, interceptors, MCP runtime
- `pkg/websocket`: low-level WebSocket primitives, origin checks, pooling
- `pkg/jsonrpc`: standalone JSON-RPC engine reused by MCP
- Root facades (`*_facade.go`) keep `github.com/osauer/hyperserve` imports source-compatible

### Directory Structure

```
/
├── cmd/              # Command-line applications
├── internal/         # Private application code (non-exported)
├── pkg/              # Public Go packages
│   ├── server/       # HTTP server, middleware, MCP
│   ├── websocket/    # WebSocket primitives & pooling
│   └── jsonrpc/      # JSON-RPC 2.0 engine
├── examples/         # Example implementations
├── docs/             # Documentation
├── benchmarks/       # Performance benchmarks
├── spec/             # API specifications
├── configs/          # Configuration examples
└── go.{mod,sum}
```

## Key Design Decisions

### Go Standard Library First
We prioritize Go's standard library over external packages. This provides:
- Better long-term stability
- Reduced dependency management
- More predictable behavior
- Easier debugging

### Interface-Based Design
Heavy use of interfaces enables:
- Easy testing with mocks
- Flexible implementations
- Clean separation of concerns
- Plugin-style extensibility

### Context-Aware
All handlers and middleware use Go's context pattern for:
- Request-scoped values
- Cancellation propagation
- Timeout management
- Tracing and correlation

### Error Handling
Consistent error handling approach:
- Errors are always returned, never panicked
- Structured logging for debugging
- User-friendly error messages
- Proper HTTP status codes

## Performance Characteristics

### Concurrency Model
- One goroutine per connection
- Efficient goroutine pooling
- Non-blocking I/O operations
- Careful resource management

### Memory Management
- Minimal allocations in hot paths
- Buffer pooling for large operations
- Careful string handling
- Efficient JSON encoding/decoding

### Network Optimization
- HTTP/2 support via standard library
- Keep-alive connections
- Configurable timeouts
- Graceful shutdown

## Security Architecture

### Defense in Depth
Multiple layers of security:
1. Input validation
2. Rate limiting
3. Security headers
4. Authentication middleware
5. Authorization checks

### Secure Defaults
- TLS 1.2+ only
- Secure cookie flags
- CORS protection
- XSS prevention headers

## Future Roadmap

### Near Term
- Enhanced metrics and observability
- Additional MCP tool implementations
- Performance optimizations
- Extended authentication providers

### Long Term
- HTTP/3 support
- Advanced caching strategies
- Distributed tracing
- Service mesh integration

## Contributing

When contributing to HyperServe:
1. Follow Go best practices and idioms
2. Maintain minimal dependency philosophy
3. Write comprehensive tests
4. Update documentation
5. Consider performance implications

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed guidelines.