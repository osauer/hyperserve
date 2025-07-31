# HyperServe Architecture

## Why Two Implementations?

HyperServe maintains both Go and Rust implementations to leverage the unique strengths of each language while maintaining feature parity. This approach provides:

1. **Choice** - Users can select based on their ecosystem
2. **Learning** - Compare idiomatic approaches in both languages
3. **Innovation** - Features can be prototyped in either language
4. **Validation** - Cross-implementation testing ensures correctness

## Design Principles

### 1. Minimal Dependencies
- **Go**: Single dependency (`golang.org/x/time` for rate limiting)
- **Rust**: Zero dependencies - everything built from scratch

### 2. API Compatibility
Both implementations expose the same HTTP API and configuration options, allowing them to be used interchangeably.

### 3. Feature Parity
Core features are implemented in both:
- HTTP/1.1 server
- WebSocket support
- Model Context Protocol (MCP)
- Security middleware
- Health checks
- Static file serving

## Architectural Decisions

### Go Implementation
- Leverages Go's excellent standard library
- Uses goroutines for concurrency
- Simple, readable code that follows Go idioms
- Ideal for cloud-native deployments

### Rust Implementation
- Built from first principles with zero dependencies
- Memory safe without garbage collection
- Uses thread pool for concurrency
- Ideal for embedded systems and edge computing

## Shared Components

### Specifications (`/spec`)
Language-agnostic API and protocol specifications ensure both implementations behave identically.

### Tests (`/tests/conformance`)
Cross-implementation tests validate that both versions conform to the same behavior.

### Documentation (`/docs`)
Shared documentation covers common concepts, deployment, and usage patterns.

## Trade-offs

### Go Advantages
- Faster development iteration
- Battle-tested HTTP/2 support
- Rich standard library
- Simple deployment (single binary)

### Rust Advantages
- No garbage collection pauses
- Lower memory footprint
- Predictable performance
- Can target no_std environments

## Future Directions

1. **Performance Benchmarks** - Detailed comparisons under various workloads
2. **WASM Support** - Rust implementation compiled to WebAssembly
3. **Embedded Targets** - Rust on microcontrollers
4. **Advanced MCP Features** - Leverage each language's strengths

## Contributing

When adding features:
1. Update the specification first
2. Implement in both languages
3. Add conformance tests
4. Update documentation

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed guidelines.