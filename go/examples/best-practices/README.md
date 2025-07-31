# Hyperserve Best Practices Example

This example demonstrates the correct way to use hyperserve's built-in features without reimplementing functionality.

## What This Example Shows

### ✅ DO: Use Built-in Features

1. **Graceful Shutdown** - No custom signal handling needed
2. **Request Logging** - Automatic structured logging with slog
3. **Rate Limiting** - Built-in token bucket rate limiting
4. **Health Checks** - Separate health server on :8081
5. **MCP Support** - Native Model Context Protocol integration
6. **SSE Support** - Proper Server-Sent Events with helpers
7. **Security Headers** - Pre-configured middleware stacks
8. **Configuration** - Environment variables and defaults

### ❌ DON'T: Common Anti-Patterns to Avoid

1. **Custom shutdown handling** - hyperserve handles SIGINT/SIGTERM
2. **Custom logging middleware** - RequestLoggerMiddleware is applied by default
3. **Manual MCP implementation** - Use WithMCPSupport()
4. **Manual SSE formatting** - Use NewSSEMessage() helper
5. **Hardcoded configuration** - Use HS_* environment variables

## Running the Example

```bash
# Basic usage
go run main.go

# With debug logging
HS_LOG_LEVEL=debug go run main.go

# With custom configuration
HS_PORT=9090 HS_RATE_LIMIT=50 go run main.go

# Test protected endpoint
curl -H "Authorization: Bearer secret-token-123" http://localhost:8080/api/data

# Test MCP endpoint
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 1
  }'

# Watch SSE stream
curl -N http://localhost:8080/api/stream
```

## Key Takeaways

1. **Hyperserve is batteries-included** - Most common server needs are built-in
2. **Use functional options** - Configure with WithX() functions
3. **Leverage middleware stacks** - SecureAPI() and SecureWeb() for common patterns
4. **Trust the defaults** - Sensible defaults that work for most applications
5. **Progressive complexity** - Start simple, add features as needed

## Configuration Options

All configuration can be set via environment variables:

- `HS_PORT` - Server port (default: 8080)
- `HS_HEALTH_PORT` - Health check port (default: 8081)
- `HS_RATE_LIMIT` - Requests per second (default: 100)
- `HS_BURST_LIMIT` - Burst capacity (default: 200)
- `HS_LOG_LEVEL` - Log level: debug, info, warn, error (default: info)
- `HS_SHUTDOWN_TIMEOUT` - Graceful shutdown timeout (default: 30s)
- `HS_MCP_ENABLED` - Enable MCP support (default: false)
- `HS_MCP_FILE_TOOL_ROOT` - Root directory for MCP file tools

## Comparison: Wrong Way vs Right Way

### Logging
```go
// ❌ WRONG: Custom logging middleware
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        // ... custom logging implementation
    })
}

// ✅ RIGHT: Use built-in logging (applied automatically)
srv, _ := hyperserve.NewServer() // RequestLoggerMiddleware included by default
```

### Shutdown
```go
// ❌ WRONG: Custom signal handling
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt)
go func() {
    <-sigChan
    os.Exit(0)
}()

// ✅ RIGHT: Let hyperserve handle it
srv.Run() // Handles SIGINT/SIGTERM automatically
```

### MCP
```go
// ❌ WRONG: Custom MCP handler
type MCPHandler struct{}
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // ... manual JSON-RPC implementation
}

// ✅ RIGHT: Use built-in MCP
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
)
```

### SSE
```go
// ❌ WRONG: Manual SSE formatting
fmt.Fprintf(w, "event: %s\n", event)
fmt.Fprintf(w, "data: %s\n\n", data)

// ✅ RIGHT: Use SSE helper
msg := hyperserve.NewSSEMessage(event, data)
fmt.Fprint(w, msg)
```