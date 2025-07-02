# Middleware Basics Example

This interactive example demonstrates HyperServe's middleware system by building up a middleware stack step by step. You'll see how each middleware affects the server's behavior and performance.

## What This Example Shows

- How middleware wraps HTTP handlers
- The order of middleware execution
- Global vs route-specific middleware
- Common middleware patterns
- Performance impact of different middleware

## Running the Example

```bash
go run main.go
```

The example runs interactively, guiding you through 5 steps:

1. **No Middleware** - Baseline server
2. **Request Logging** - Add logging
3. **Request Metrics** - Add metrics collection  
4. **Rate Limiting** - Add rate limiting
5. **Full Stack** - Complete middleware setup with route-specific rules

## Interactive Demo

When you run the example, it will:
1. Explain what each step demonstrates
2. Wait for you to press Enter to start
3. Run the server with that configuration
4. Let you test it with curl
5. Wait for Enter to continue to the next step

## Testing Each Step

### Step 1: No Middleware
```bash
curl http://localhost:8080/api/data
# Fast response, no logging
```

### Step 2: With Logging
```bash
curl http://localhost:8080/api/data
# See request details in console
```

### Step 3: With Metrics
```bash
curl http://localhost:8080/api/data
curl http://localhost:8080/metrics
# See request count and timing
```

### Step 4: With Rate Limiting
```bash
# Make rapid requests
for i in {1..20}; do curl http://localhost:8080/api/data; done
# See 429 errors after limit exceeded
```

### Step 5: Full Stack
```bash
# Public route (no rate limit)
curl http://localhost:8080/

# API route (rate limited)
curl http://localhost:8080/api/data

# Crash test (recovery middleware)
curl http://localhost:8080/api/crash

# Metrics
curl http://localhost:8080/metrics
```

## Key Concepts

### 1. Middleware Order Matters

```go
server.AddMiddleware("*", logging)    // Runs first
server.AddMiddleware("*", metrics)    // Runs second
server.AddMiddleware("*", rateLimit)  // Runs third
```

Middleware executes in the order it's added. The first middleware sees the request first and the response last.

### 2. Global vs Route-Specific

```go
// Global - applies to all routes
server.AddMiddleware("*", middleware)

// Route-specific - only for paths starting with /api
server.AddMiddleware("/api", middleware)
```

### 3. Middleware Signature

HyperServe uses this middleware function type:
```go
type MiddlewareFunc func(http.Handler) http.HandlerFunc
```

Standard middleware pattern:
```go
func MyMiddleware(next http.Handler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Before handler
        next.ServeHTTP(w, r)
        // After handler
    }
}
```

### 4. Performance Impact

Each middleware adds overhead:
- **Logging**: ~250% overhead (I/O bound)
- **Metrics**: ~50% overhead (memory/CPU)
- **Rate Limiting**: ~50% overhead (map lookups)
- **Recovery**: ~-9% (actually improves performance!)

## Common Middleware Patterns

### Timing Requests

```go
start := time.Now()
next.ServeHTTP(w, r)
duration := time.Since(start)
```

### Modifying Requests

```go
// Add header before processing
r.Header.Set("X-Request-ID", generateID())
next.ServeHTTP(w, r)
```

### Short-Circuit Responses

```go
if !authorized {
    http.Error(w, "Unauthorized", 401)
    return // Don't call next
}
next.ServeHTTP(w, r)
```

### Wrapping Response Writer

```go
wrapped := &responseWriter{ResponseWriter: w}
next.ServeHTTP(wrapped, r)
log.Printf("Status: %d", wrapped.statusCode)
```

## Try These Modifications

1. **Add Custom Middleware**: Create a middleware that adds a custom header
2. **Conditional Middleware**: Only apply middleware based on request headers
3. **Chain Middleware**: Create a middleware that combines multiple middlewares
4. **Error Handling**: Add middleware that catches and formats errors

## Writing Your Own Middleware

Here's a template for custom middleware compatible with HyperServe:

```go
func MyMiddleware(srv *hyperserve.Server) hyperserve.MiddlewareFunc {
    return func(next http.Handler) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // Pre-processing
            log.Println("Before request")
            
            // Call the next handler
            next.ServeHTTP(w, r)
            
            // Post-processing
            log.Println("After request")
        }
    }
}
```

## Best Practices

1. **Order Carefully**: Place authentication before rate limiting
2. **Minimize Overhead**: Avoid expensive operations in middleware
3. **Use Context**: Pass data between middleware using `r.Context()`
4. **Handle Errors**: Don't let middleware panic
5. **Document Effects**: Clearly state what your middleware does

## What's Next?

Now that you understand middleware, move on to [configuration](../configuration/) to learn about HyperServe's configuration system.