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

Implement two explicit middleware scopes:

1. **Global middleware**: applied to every request with `Server.Use`.
2. **Prefix middleware**: applied at a URL path boundary with
   `Server.UsePrefix`.

```go
srv.Use(RequestIDMiddleware())
srv.UsePrefix("/api", requireIdentity, RateLimitMiddleware(srv))
```

Middleware execution order follows registration sequence.

## Consequences

### Positive
- **Flexibility**: Global and nested path policy without a custom router
- **Performance**: Routes only run necessary middleware
- **Clarity**: Clear which middleware applies where
- **Simplicity**: No complex router groups needed
- **Composable**: Mix and match middleware as needed

### Negative
- **Complexity**: More complex than one global middleware chain
- **Ordering confusion**: Execution order matters
- **Memory overhead**: Multiple middleware chains stored
- **Pattern matching cost**: Must check patterns on each request

### Mitigation
- Clear documentation on execution order
- Efficient pattern matching implementation
- Examples of common middleware patterns

## Implementation Details

- Middleware has the standard `func(http.Handler) http.Handler` shape.
- Prefixes match at path-segment boundaries: `/api` matches `/api/users` but
  not `/apiv2`.
- Prefixes are ordered at registration time; deeper matching prefixes wrap
  closer to the route handler.
- Global middleware remains outermost.
- Middleware must be registered before serving requests concurrently.

## Examples

```go
verifier := auth.TokenVerifierFunc(verifyToken)
apiIdentity := auth.Bearer(verifier)
requireIdentity := auth.Require(apiIdentity)

srv.Use(RequestIDMiddleware())
srv.Use(server.SecureWeb(srv.Options()))
srv.UsePrefix("/api", requireIdentity, server.RateLimitMiddleware(srv))
srv.UsePrefix("/api/admin", requireAdmin)

// Keep public routes outside the authenticated prefix.
srv.GET("/status", statusHandler)
```
