# HyperServe Specification

This directory contains the specifications and conformance tests for HyperServe.

## Structure

- `conformance/` - Conformance test suite to verify implementation
- `api.md` - API specifications and expected behaviors

## Running Conformance Tests

```bash
# Run conformance tests
cd conformance
./test.sh
```

## Protocol Compliance

The implementation must:
- Handle HTTP requests/responses according to RFC standards
- Support middleware chain behavior as specified
- Implement WebSocket protocol correctly
- Provide feature-complete MCP support
- Return properly formatted JSON responses

## API Specification

See [api.md](./api.md) for the complete API specification including:
- Endpoint definitions
- Request/response formats
- Status codes
- Error handling