# ADR-0001: Minimal External Dependencies

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Most Go HTTP server libraries and frameworks pull in numerous external dependencies. For example:
- Popular web frameworks often have 20-50+ direct and indirect dependencies
- Each dependency increases the security attack surface
- Dependencies can introduce version conflicts
- Build times and binary sizes increase with each dependency
- Supply chain attacks are a growing concern

The hyperserve project needs to balance functionality with simplicity, security, and maintainability.

## Decision

We will minimize external dependencies, using only `golang.org/x/time/rate` for rate limiting functionality. All other features will be implemented using Go's standard library.

## Consequences

### Positive
- **Smaller binary size**: Typical binaries are under 10MB
- **Faster compilation**: Build times remain under 5 seconds
- **Easier security auditing**: Only one external dependency to review
- **Better control**: We own the implementation of all features
- **Simpler debugging**: No need to trace through multiple libraries
- **Reduced supply chain risk**: Minimal exposure to compromised dependencies

### Negative
- **More code to maintain**: We must implement features that exist in other libraries
- **Potential for bugs**: Can't leverage battle-tested community solutions
- **Slower feature development**: Must build everything from scratch
- **Limited ecosystem integration**: Can't easily plug in popular middleware

### Mitigation
- Focus on core HTTP server features that are well-understood
- Extensive testing to ensure reliability
- Clear documentation of limitations
- Encourage users to wrap hyperserve for additional functionality

## Notes
The only exception is `golang.org/x/time/rate` because:
1. It's maintained by the Go team (trusted source)
2. Rate limiting algorithms are complex and easy to get wrong
3. The implementation is mature and well-tested