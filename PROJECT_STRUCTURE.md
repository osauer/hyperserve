# HyperServe Project Structure

## Overview

HyperServe is a high-performance HTTP server framework written in Go with native Model Context Protocol (MCP) support.

## Directory Structure

```
hyperserve/
├── .github/workflows/    # CI/CD workflows
├── benchmarks/          # Performance benchmarks
├── cmd/                 # Command-line applications
│   ├── example-server/ # Minimal benchmarking server
│   └── server/         # Reference CLI wrapper around the library
├── configs/            # Configuration examples
├── docs/               # Documentation
├── examples/           # Example implementations
│   ├── auth/               # Authentication examples
│   ├── enterprise/         # Enterprise features
│   ├── hello-world/        # Basic usage
│   ├── mcp-*/              # MCP integration patterns
│   └── websocket-*/        # WebSocket examples
├── internal/           # Private application code
│   └── ws/            # WebSocket internals
├── spec/               # API specifications
│   └── conformance/   # Conformance tests
└── *.go               # Core library files
```

## Root Files

### Configuration
- `go.mod` - Go module definition
- `go.sum` - Dependency checksums
- `.golangci.yml` - Linter configuration
- `Makefile` - Build automation

### Documentation
- `README.md` - Project overview and quick start
- `ARCHITECTURE.md` - System architecture and design decisions
- `PROJECT_STRUCTURE.md` - This file
- `CONTRIBUTING.md` - Contribution guidelines
- `CHANGELOG.md` - Release history
- `CLAUDE.md` - AI assistant instructions
- `LICENSE` - MIT license

## Core Components

### Source Files
- `server.go` - Main server implementation
- `middleware.go` - Middleware system
- `handlers.go` - HTTP handlers
- `websocket.go` - WebSocket support
- `websocket_pool.go` - Connection pooling
- `mcp.go` - MCP protocol implementation
- `mcp_transport.go` - MCP transport layers
- `mcp_builtin.go` - Built-in MCP tools
- `options.go` - Server configuration options
- `interceptor.go` - Request/response interceptors
- `jsonrpc.go` - JSON-RPC protocol support

### Test Files
- `*_test.go` - Unit tests for each component
- `integration_test.go` - Integration tests
- `benchmark_test.go` - Performance benchmarks

## Key Directories

### `/cmd`
Command-line applications built with HyperServe:
- `example-server/` - Lightweight binary for benchmarks and load testing
- `server/` - Feature-complete CLI wrapping the library for demos

### `/internal`
Private packages not exposed in the public API:
- `ws/` - WebSocket protocol implementation details

### `/examples`
Comprehensive examples demonstrating features such as authentication, MCP, WebSocket pooling, HTMX integrations, and configuration management. Directory names follow a `feature-name` convention (e.g., `mcp-basic`, `websocket-pool`).

### `/docs`
Technical documentation:
- API guides
- MCP integration guide
- WebSocket implementation
- Performance optimization
- Best practices

### `/spec`
Formal specifications:
- `api.md` - HTTP API specification
- `conformance/` - Conformance test suite

### `/benchmarks`
Performance testing:
- Benchmark scripts
- Performance comparison tools
- Results analysis

### `/configs`
Configuration examples:
- Development settings
- Production configurations
- Docker configurations

## Design Philosophy

1. **Flat structure** - Core library at root for easy imports
2. **Clear separation** - Public API vs internal implementation
3. **Comprehensive examples** - Real-world usage patterns
4. **Extensive testing** - Unit, integration, and benchmarks
5. **Rich documentation** - Guides for all skill levels

## Import Paths

```go
import "github.com/osauer/hyperserve"           // Core library
import "github.com/osauer/hyperserve/internal/ws" // Internal (not recommended)
```

## Building

```bash
# Build the library
go build .

# Build the reference CLI
go build ./cmd/server

# Run tests
go test ./...

# Run benchmarks
go test -bench=. ./...
```
