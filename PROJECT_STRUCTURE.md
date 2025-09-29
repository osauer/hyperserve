# HyperServe Project Structure

## Overview

HyperServe is a high-performance HTTP server framework written in Go with native Model Context Protocol (MCP) support.

## Directory Structure

```
hyperserve/
├── .github/workflows/    # CI/CD workflows
├── benchmarks/          # Performance benchmarks
├── cmd/                 # Command-line applications
│   ├── example-server/  # Minimal benchmarking server
│   └── server/          # Reference CLI wrapper around the library
├── configs/             # Configuration examples
├── docs/                # Documentation
├── examples/            # Example implementations
│   ├── auth/                # Authentication examples
│   ├── enterprise/
│   ├── mcp-*/
│   └── websocket-*/
├── internal/            # Private application code
│   ├── scaffold/        # Project generator internals
│   └── ws/              # WebSocket protocol primitives
├── pkg/                 # Public Go packages
│   ├── jsonrpc/         # JSON-RPC 2.0 engine
│   ├── server/          # HTTP/MCP server core
│   └── websocket/       # WebSocket primitives & pooling
├── spec/                # API specifications
│   └── conformance/     # Conformance tests
├── server/              # Legacy generated assets (to be archived)
├── jsonrpc_facade.go    # Backwards compatibility shim
├── mcp_facade.go        # Backwards compatibility shim
├── server_facade.go     # Backwards compatibility shim
├── websocket_facade.go  # Backwards compatibility shim
└── go.{mod,sum}
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
Key packages:
- `pkg/server` – HTTP server, middleware registry, interceptor chain, and MCP implementation
- `pkg/websocket` – WebSocket upgrader, connection pool, and security helpers
- `pkg/jsonrpc` – Stand-alone JSON-RPC 2.0 processing engine
- Root shims re-export the stable API for existing imports

### Test Files
- Package-local `*_test.go` alongside implementations
- `pkg/server` owns integration, security, and benchmark suites
- Fixtures live under package-specific `testdata/`

## Key Directories

### `/cmd`
Command-line applications built with HyperServe:
- `example-server/` - Lightweight binary for benchmarks and load testing
- `server/` - Feature-complete CLI wrapping the library for demos
- `hyperserve-init/` - Project scaffolding CLI that generates secure, MCP-ready services

### `/internal`
Private packages not exposed in the public API:
- `ws/` - WebSocket protocol implementation details
- `scaffold/` - Project generator and templates backing `hyperserve-init`

### `/examples`
Comprehensive examples demonstrating features such as authentication, MCP, WebSocket pooling, HTMX integrations, and configuration management. Directory names follow a `feature-name` convention (e.g., `mcp-basic`, `websocket-pool`).

### `/docs`
Technical documentation:
- API guides (`docs/guides/`)
- Design decision logs (`docs/000*-*.md`)
- MCP integration guide
- WebSocket implementation
- [Performance guide](docs/PERFORMANCE.md)
- [Scaffolding guide](docs/SCAFFOLDING.md)
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

1. **Layered packages** – Core logic in `pkg/`, with root shims for compatibility
2. **Clear separation** – Public API vs. internal tooling
3. **Comprehensive examples** – Real-world usage patterns
4. **Extensive testing** – Unit, integration, and benchmarks
5. **Rich documentation** – Guides for all skill levels

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
