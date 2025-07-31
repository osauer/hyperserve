# HyperServe Project Structure

## Overview

HyperServe is organized as a dual-implementation project with Go and Rust versions sharing common specifications and tests.

## Directory Structure

```
hyperserve/
├── .github/workflows/    # CI/CD for both implementations
├── docs/                 # Shared documentation
├── go/                   # Go implementation
├── rust/                 # Rust implementation  
├── spec/                 # API specifications
├── scripts/              # Build and test scripts
└── tests/conformance/    # Cross-implementation tests
```

## Root Files

- `README.md` - Project overview
- `ARCHITECTURE.md` - Dual implementation rationale
- `LICENSE` - MIT license
- `CONTRIBUTING.md` - Contribution guidelines
- `CHANGELOG.md` - Release history
- `CLAUDE.md` - AI assistant instructions
- `.mcp` - MCP marker for AI tools

## Implementation Directories

### `/go`
- Complete Go implementation
- Single dependency: `golang.org/x/time`
- Includes benchmarks, configs, examples
- Go-specific docs in `go/docs/`

### `/rust`
- Zero-dependency Rust implementation
- Built from scratch using Rust 2024
- Examples in `rust/examples/`

## Shared Resources

### `/spec`
- `api.md` - HTTP API specification
- `mcp-protocol.md` - MCP protocol details

### `/docs`
- Guides applicable to both implementations
- MCP usage documentation
- WebSocket implementation guide

### `/tests/conformance`
- Tests that both implementations must pass
- Ensures API compatibility
- Language-agnostic test specifications

## Design Philosophy

1. **Minimal root** - Only cross-cutting concerns
2. **Self-contained implementations** - Each language directory is complete
3. **Shared specifications** - Single source of truth for behavior
4. **Conformance testing** - Automated verification of compatibility