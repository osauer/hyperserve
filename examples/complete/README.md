# HyperServe Complete Example

This example demonstrates correct usage of HyperServe features, including zero-configuration defaults and opt-in middleware stacks.

## Features Demonstrated

### Automatic Features (Zero Configuration)

These work out of the box with `hyperserve.NewServer()`:

- **Graceful Shutdown** - Automatic on Ctrl+C via `srv.Run()`
- **Health Checks** - Available on :8081 when enabled with WithHealthServer()
- **Request Logging** - Via DefaultMiddleware
- **Panic Recovery** - Via DefaultMiddleware  
- **Metrics Collection** - Via DefaultMiddleware
- **Memory Leak Prevention** - Rate limiter cleanup every 5 minutes

### Configured Features

These are explicitly enabled in this example:

- **Security Headers** - Applied via `SecureWeb` middleware stack
- **Authentication** - Token validation + `SecureAPI` middleware stack
- **Rate Limiting** - Applied to /api/* via `SecureAPI` middleware stack
- **Server-Sent Events** - Custom SSE handler
- **Static Files** - Via `HandleStatic()`
- **Templates** - Dynamic HTML generation
- **MCP Support** - AI assistant integration
- **File Upload** - Multipart form handling

## Running the Example

```bash
# From the examples/complete directory
go run main.go

# Or from hyperserve root
go run examples/complete/main.go
```

## Understanding Middleware Stacks

This example correctly uses hyperserve's middleware system:

1. **DefaultMiddleware** (automatic):
   - MetricsMiddleware
   - RequestLoggerMiddleware
   - RecoveryMiddleware

2. **SecureWeb** (applied to all routes):
   - HeadersMiddleware (security headers)

3. **SecureAPI** (applied to /api/*):
   - AuthMiddleware (Bearer token validation)
   - RateLimitMiddleware (per-IP rate limiting)

## Endpoints

### Public Endpoints
- `GET /` - Home page showing all features
- `GET /static/*` - Static assets (CSS, JS)
- `GET /api/status` - Public API (has security headers)

### Protected Endpoints (Require Bearer Token)
- `GET /api/user` - Get user info
- `GET /api/stream` - SSE real-time updates
- `POST /api/upload` - File upload demo
- `GET /api/error` - Error recovery demo
- `GET /api/metrics` - Metrics information

### Health Check
- `GET http://localhost:8081/healthz` - Kubernetes-ready health check
- `GET http://localhost:8081/readyz` - Readiness probe
- `GET http://localhost:8081/livez` - Liveness probe

### MCP Endpoint
- `POST /mcp` - Model Context Protocol for AI assistants

## Authentication

The example includes two test tokens:
- `demo-token-123` - Authenticates as user "alice"
- `demo-token-456` - Authenticates as user "bob"

Example:
```bash
curl -H "Authorization: Bearer demo-token-123" http://localhost:8080/api/user
```

## Key Patterns Demonstrated

### 1. Correct Middleware Stack Usage
```go
// Middleware stacks use AddMiddlewareStack, not AddMiddleware
srv.AddMiddlewareStack("/", hs.SecureWeb(srv.Options))
srv.AddMiddlewareStack("/api", hs.SecureAPI(srv))
```

### 2. Zero-Config Features
```go
// These features work automatically:
// - Graceful shutdown (in srv.Run())
// - Health checks (on :8081 when enabled with WithHealthServer())
// - Request logging (DefaultMiddleware)
// - Panic recovery (DefaultMiddleware)
// - Metrics (DefaultMiddleware)
```

### 3. SSE with Context Cancellation
```go
for {
    select {
    case <-r.Context().Done():
        return // Clean shutdown
    case <-ticker.C:
        // Send update
    }
}
```

### 4. Proper Error Handling
The recovery middleware (included in DefaultMiddleware) catches panics and returns 500 errors instead of crashing the server.

## Interactive Web Interface

Open http://localhost:8080 to see the interactive web interface that demonstrates:

1. **Feature Status** - Shows which features are automatic vs configured
2. **Authentication Test** - Try different tokens
3. **Real-time SSE Stream** - Live CPU/Memory chart
4. **File Upload** - Multipart form handling
5. **Error Recovery** - Test panic recovery

## What This Example Teaches

- Difference between automatic and opt-in features
- Correct usage of middleware stacks vs individual middleware
- How DefaultMiddleware provides core functionality
- Proper patterns for authentication, SSE, and file handling
- How graceful shutdown and health checks work automatically