# ADR-0002: Functional Options Pattern for Configuration

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

HTTP servers require extensive configuration options:
- Port, timeouts, TLS settings
- Middleware configuration
- Rate limiting parameters
- Template directories
- Logging and protocol settings

Traditional approaches include:
1. **Config structs**: `NewServer(config ServerConfig)` - breaks on new fields
2. **Multiple constructors**: `NewServer()`, `NewTLSServer()` - explosion of variants
3. **Builder pattern**: `ServerBuilder.WithPort().Build()` - verbose and stateful
4. **Variadic interfaces**: Type assertions needed

We need a pattern that supports optional configuration while maintaining backward compatibility.

## Decision

Use the functional options pattern with:
```go
type Option func(*Server) error

func WithAddr(addr string) Option {
	return func(srv *Server) error {
		srv.options.Addr = addr
		return nil
	}
}

func NewServer(options ...Option) (*Server, error)
```

## Consequences

### Positive
- **Backward compatibility**: New options don't break existing code
- **Self-documenting**: Each `WithX()` function clearly states its purpose
- **Type safety**: Compile-time checking of option types
- **Sensible defaults**: Unspecified options use reasonable defaults
- **Validation**: Each option can validate its input
- **Composable**: Options can be grouped and reused

### Negative
- **Verbosity**: More verbose than struct literals
- **Runtime errors**: Invalid options only discovered at runtime
- **Separate serialization path**: Function options are not serializable;
  `Options` plus `WithConfigFile` covers reviewed JSON configuration
- **Discovery**: Users must browse documentation to find options

### Mitigation
- Provide common option combinations as examples
- Clear error messages for invalid configurations
- Comprehensive documentation of all options
- IDE autocomplete helps with discovery

## Examples

```go
// Simple usage with defaults
srv, _ := server.NewServer()

// Custom configuration
srv, _ := server.NewServer(
	server.WithAddr(":8080"),
    server.WithRateLimit(100, 200),
    server.WithTLS("cert.pem", "key.pem"),
)

// Reusable option sets
productionOpts := []server.Option{
    server.WithFIPSMode(),
	server.WithTimeouts(30*time.Second, 30*time.Second, 2*time.Minute),
    server.WithRateLimit(1000, 2000),
}
srv, _ := server.NewServer(productionOpts...)
```
