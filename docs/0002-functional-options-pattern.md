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
- Authentication settings

Traditional approaches include:
1. **Config structs**: `NewServer(config ServerConfig)` - breaks on new fields
2. **Multiple constructors**: `NewServer()`, `NewTLSServer()` - explosion of variants
3. **Builder pattern**: `ServerBuilder.WithPort().Build()` - verbose and stateful
4. **Variadic interfaces**: Type assertions needed

We need a pattern that supports optional configuration while maintaining backward compatibility.

## Decision

Use the functional options pattern with:
```go
type ServerOptionFunc func(*ServerOptions) error

func WithPort(port int) ServerOptionFunc {
    return func(opts *ServerOptions) error {
        opts.Port = port
        return nil
    }
}

func NewServer(options ...ServerOptionFunc) (*Server, error)
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
- **No serialization**: Can't easily save/load configurations
- **Discovery**: Users must browse documentation to find options

### Mitigation
- Provide common option combinations as examples
- Clear error messages for invalid configurations
- Comprehensive documentation of all options
- IDE autocomplete helps with discovery

## Examples

```go
// Simple usage with defaults
srv, _ := hyperserve.NewServer()

// Custom configuration
srv, _ := hyperserve.NewServer(
    hyperserve.WithPort(8080),
    hyperserve.WithRateLimit(100, 200),
    hyperserve.WithTLS("cert.pem", "key.pem"),
)

// Reusable option sets
productionOpts := []hyperserve.ServerOptionFunc{
    hyperserve.WithFIPSMode(),
    hyperserve.WithTimeouts(30*time.Second, 30*time.Second),
    hyperserve.WithRateLimit(1000, 2000),
}
srv, _ := hyperserve.NewServer(productionOpts...)
```