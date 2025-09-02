# Request/Response Interceptors Example

This example demonstrates HyperServe's powerful interceptor system for implementing cross-cutting concerns like authentication, logging, rate limiting, and response transformation.

## Features

- **Request Interception**: Modify requests before they reach handlers
- **Response Transformation**: Modify responses after handler execution
- **Early Response**: Return responses directly from interceptors
- **Chain Management**: Add/remove interceptors dynamically
- **Metadata Passing**: Share data between interceptors and handlers

## Running the Example

```bash
cd examples/interceptors
go run main.go
```

Open http://localhost:8086 in your browser for an interactive demo.

## Implemented Interceptors

### 1. API Key Validator
Validates API keys and enriches requests with user information.

```go
validator := NewAPIKeyValidator()
chain.Add(validator)
```

**Features:**
- Validates `X-API-Key` header
- Returns 401 for invalid/missing keys
- Adds user ID to request metadata
- Skips validation for public endpoints

### 2. Rate Limiter
Enforces per-client request limits.

```go
rateLimiter := NewSimpleRateLimiter(10, 20) // 10 RPS, burst 20
chain.Add(hyperserve.NewRateLimitInterceptor(rateLimiter))
```

**Features:**
- Per-client IP rate limiting
- Configurable requests per second and burst
- Returns 429 when limit exceeded
- Includes `Retry-After` header

### 3. Request Logger
Logs all requests and responses with timing.

```go
chain.Add(hyperserve.NewRequestLogger(log.Printf))
```

**Features:**
- Logs request method and path
- Records response status and duration
- Stores timing in request metadata

### 4. JSON Transformer
Adds metadata wrapper to JSON responses.

```go
chain.Add(&JSONTransformer{})
```

**Features:**
- Wraps JSON responses with metadata
- Adds timestamp and version info
- Includes user ID when available
- Only processes JSON content types

### 5. CORS Handler
Manages cross-origin resource sharing.

```go
chain.Add(NewCORSInterceptor([]string{"*"}))
```

**Features:**
- Handles preflight OPTIONS requests
- Sets appropriate CORS headers
- Configurable allowed origins
- Supports credentials

## Usage Pattern

```go
// Create interceptor chain
chain := hyperserve.NewInterceptorChain()

// Add interceptors in desired order
chain.Add(corsInterceptor)
chain.Add(logger)
chain.Add(rateLimiter) 
chain.Add(authValidator)
chain.Add(responseTransformer)

// Wrap handlers
srv.HandleFunc("/api/data", chain.WrapHandler(handler).ServeHTTP)
```

## API Endpoints

### Public Endpoint (No Auth)
```
GET /public/health
```
Returns health status without authentication.

### Protected API (Auth Required)  
```
GET /api/data
Header: X-API-Key: demo-key-123
```
Returns user-specific data with authentication.

### Response Transformation
```
GET /api/transform  
Header: X-API-Key: demo-key-456
```
Demonstrates JSON response wrapping with metadata.

## Valid API Keys

- `demo-key-123` → `user-1`
- `demo-key-456` → `user-2` 
- `admin-key` → `admin`

## Creating Custom Interceptors

Implement the `Interceptor` interface:

```go
type MyInterceptor struct{}

func (mi *MyInterceptor) Name() string {
    return "MyInterceptor"
}

func (mi *MyInterceptor) InterceptRequest(ctx context.Context, req *hyperserve.InterceptableRequest) (*hyperserve.InterceptorResponse, error) {
    // Modify request or return early response
    return nil, nil // Continue to next interceptor
}

func (mi *MyInterceptor) InterceptResponse(ctx context.Context, req *hyperserve.InterceptableRequest, resp *hyperserve.InterceptableResponse) error {
    // Modify response
    return nil
}
```

## Common Use Cases

1. **Authentication/Authorization**: Validate tokens, inject user context
2. **Rate Limiting**: Prevent abuse, implement quotas
3. **Audit Logging**: Track all requests for compliance
4. **Response Transformation**: Add metadata, format responses
5. **CORS Management**: Handle cross-origin requests
6. **Request Validation**: Validate input before processing
7. **Caching**: Implement response caching logic
8. **A/B Testing**: Route requests to different handlers
9. **Multi-tenancy**: Inject tenant context
10. **Compression**: Compress responses based on client support

## Best Practices

1. **Order Matters**: Place interceptors in logical execution order
2. **Error Handling**: Always handle errors gracefully
3. **Performance**: Keep interceptors lightweight
4. **Metadata Usage**: Use request metadata to pass data between interceptors
5. **Early Returns**: Use early responses judiciously to avoid handler execution