# HyperServe Specification

This directory contains the shared specifications and conformance tests for both the Go and Rust implementations of HyperServe.

## Structure

- `conformance/` - Conformance test suite to verify implementation compatibility
- `protocols/` - Protocol specifications (HTTP, WebSocket, MCP)
- `api/` - API specifications and expected behaviors

## Running Conformance Tests

The conformance tests can be run against either implementation:

```bash
# Test Go implementation
cd conformance
./test.sh go

# Test Rust implementation  
cd conformance
./test.sh rust

# Test both and compare
cd conformance
./test.sh both
```

## Protocol Compliance

Both implementations must:
- Handle identical HTTP requests/responses
- Support the same middleware chain behavior
- Implement WebSocket protocol correctly
- Provide feature-complete MCP support
- Return identical JSON responses