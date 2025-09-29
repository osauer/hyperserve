# ADR-0006: Go 1.24 Minimum Version Requirement

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Go 1.24 introduces several significant features:
- **os.Root**: Secure file serving with automatic sandboxing
- **Swiss Tables**: Faster map implementation (up to 30% improvement)
- **FIPS 140-3 mode**: Required for government compliance
- **Encrypted Client Hello (ECH)**: Enhanced privacy
- **X25519MLKEM768**: Post-quantum cryptography by default

These features offer substantial security and performance benefits but require dropping support for older Go versions.

## Decision

Require Go 1.24 as the minimum supported version and actively leverage its new features:
- Use `os.Root` for secure static file serving
- Rely on Swiss Tables for rate limiter performance
- Provide `WithFIPSMode()` option for compliance
- Enable ECH and post-quantum crypto by default

## Consequences

### Positive
- **Better performance**: 10-30% faster map operations with Swiss Tables
- **Enhanced security**: Automatic sandboxing, FIPS compliance, post-quantum ready
- **Modern features**: Access to latest Go improvements
- **Simpler code**: Can use new APIs without compatibility shims
- **Future-proof**: Ready for post-quantum threats

### Negative
- **Limited adoption**: Go 1.24 is very new (hypothetical future version)
- **No compatibility**: Can't use with older Go versions
- **Enterprise friction**: Slow-moving organizations may resist
- **CI/CD updates**: Build systems need updates

### Mitigation
- Clear documentation of version requirement
- Explain benefits to justify the requirement
- Provide migration guide from older versions
- Use build tags to fail gracefully on older versions

## Implementation Details

Key features leveraged:
```go
srv, _ := server.NewServer(server.WithFIPSMode())

// os.Root for secure file serving (HandleStatic uses it automatically)
srv.HandleStatic("/static/")

// Swiss Tables (automatic with regular maps)
rateLimiters := make(map[string]*rate.Limiter)

// Post-quantum crypto enabled by default in Go 1.24's TLS stack
```

## Examples

```go
// go.mod
module github.com/example/hyperserve
go 1.24

// Build constraint for clear error
//go:build go1.24

// Using Go 1.24 features
srv, _ := server.NewServer(
    server.WithFIPSMode(),        // FIPS 140-3 compliance
)

// os.Root sandboxing is enabled internally when available
srv.HandleStatic("/static/")
```

## Notes

This decision locks us into modern Go but provides significant benefits:
- Performance gains are measurable (10-30% for map-heavy operations)
- Security improvements are critical for enterprise adoption
- Post-quantum readiness is forward-thinking
- FIPS compliance opens government market
