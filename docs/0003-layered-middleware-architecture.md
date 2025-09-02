# ADR-0003: Layered Middleware Architecture

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

HTTP middleware is essential for cross-cutting concerns like logging, authentication, and rate limiting. However, different routes often need different middleware combinations:
- Public endpoints may not need authentication
- Health checks shouldn't be rate limited
- API routes need different middleware than web pages
- Some middleware should apply globally

Traditional middleware approaches:
1. **Global chain**: All middleware applies to all routes
2. **Per-route middleware**: Lots of duplication
3. **Router groups**: Requires complex router implementation

## Decision

Implement a three-tier middleware system:
1. **Global middleware**: Applied to all routes via `"*"` pattern
2. **Route-specific middleware**: Applied to specific route patterns
3. **Exclusion mechanism**: Ability to exclude middleware from specific routes

```go
// Global middleware
srv.AddMiddleware("*", LoggingMiddleware(srv.Options))

// API-specific middleware  
srv.AddMiddleware("/api", RateLimitMiddleware(srv))
srv.AddMiddleware("/api", AuthMiddleware(srv.Options))

// Exclude rate limiting from health checks
srv.ExcludeMiddleware("/healthz", RateLimitMiddleware)
```

Middleware execution order follows registration sequence.

## Consequences

### Positive
- **Flexibility**: Fine-grained control over middleware application
- **Performance**: Routes only run necessary middleware
- **Clarity**: Clear which middleware applies where
- **Simplicity**: No complex router groups needed
- **Composable**: Mix and match middleware as needed

### Negative
- **Complexity**: More complex than single middleware stack
- **Ordering confusion**: Execution order matters
- **Memory overhead**: Multiple middleware chains stored
- **Pattern matching cost**: Must check patterns on each request

### Mitigation
- Clear documentation on execution order
- Logging to show which middleware runs
- Efficient pattern matching implementation
- Examples of common middleware patterns

## Implementation Details

- Patterns use Go's standard `path.Match` function
- Middleware chains are pre-computed at registration time
- Route-specific middleware runs after global middleware
- Exclusions are processed during chain building

## Examples

```go
// Typical API server setup
srv.AddMiddleware("*", RequestIDMiddleware())
srv.AddMiddleware("*", LoggingMiddleware(srv.Options))
srv.AddMiddleware("/api", CORSMiddleware(corsConfig))
srv.AddMiddleware("/api", AuthMiddleware(srv.Options))
srv.AddMiddleware("/api", RateLimitMiddleware(srv))

// Admin routes need additional auth
srv.AddMiddleware("/api/admin", AdminAuthMiddleware())

// Public routes skip auth
srv.ExcludeMiddleware("/api/public", AuthMiddleware)
```