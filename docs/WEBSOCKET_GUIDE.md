# WebSocket Implementation Guide

This guide provides comprehensive documentation for hyperserve's WebSocket implementation, including security considerations, middleware compatibility, and best practices.

## Overview

hyperserve provides a secure, RFC 6455-compliant WebSocket implementation with zero external dependencies. The implementation is split into:

- **Public API** (`websocket.go`): High-level interface for applications
- **Internal Implementation** (`internal/ws/`): Low-level protocol handling

## Security Features

### Origin Checking

By default, hyperserve enforces same-origin policy for WebSocket connections. You can configure origin checking in three ways:

```go
// 1. Default: Same-origin only (safest)
upgrader := hyperserve.Upgrader{}

// 2. Allow specific origins
upgrader := hyperserve.Upgrader{
    AllowedOrigins: []string{
        "https://example.com",
        "https://app.example.com",
        "*.trusted-domain.com", // Wildcard subdomains
    },
}

// 3. Custom origin check function
upgrader := hyperserve.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        // Your custom logic here
        origin := r.Header.Get("Origin")
        return isOriginAllowed(origin)
    },
}
```

### Pre-Upgrade Security Hooks

Use the `BeforeUpgrade` hook for authentication, rate limiting, or other security checks:

```go
upgrader := hyperserve.Upgrader{
    BeforeUpgrade: func(w http.ResponseWriter, r *http.Request) error {
        // Example: Require authentication
        token := r.Header.Get("Authorization")
        if !isValidToken(token) {
            return errors.New("unauthorized")
        }
        
        // Example: Rate limiting
        if isRateLimited(r.RemoteAddr) {
            return errors.New("rate limit exceeded")
        }
        
        return nil
    },
}
```

### Subprotocol Enforcement

Require clients to negotiate a specific subprotocol:

```go
upgrader := hyperserve.Upgrader{
    Subprotocols:    []string{"chat.v1", "chat.v2"},
    RequireProtocol: true, // Reject if no protocol is negotiated
}
```

## Middleware Compatibility

### ResponseWriter Interface Preservation

hyperserve's middleware properly preserves WebSocket-required interfaces:

- `http.Hijacker`: Required for WebSocket upgrade
- `http.Flusher`: For real-time data streaming
- `io.ReaderFrom`: Optimizes static file serving
- `http.Pusher`: HTTP/2 server push support

Example middleware that preserves interfaces:

```go
type customResponseWriter struct {
    http.ResponseWriter
    // your fields
}

// Required for WebSocket support
func (w *customResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
    hijacker, ok := w.ResponseWriter.(http.Hijacker)
    if !ok {
        return nil, nil, fmt.Errorf("hijacking not supported")
    }
    return hijacker.Hijack()
}

// Preserve other interfaces as needed
func (w *customResponseWriter) Flush() {
    if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
        flusher.Flush()
    }
}
```

### Working with Logging Middleware

The built-in `loggingResponseWriter` automatically supports WebSocket upgrades:

```go
srv.AddMiddleware("*", hyperserve.RequestLoggerMiddleware)
// WebSocket upgrades will work through the logging middleware
```

## Best Practices

### 1. Message Size Limits

Always set appropriate message size limits:

```go
upgrader := hyperserve.Upgrader{
    MaxMessageSize: 512 * 1024, // 512KB limit
}
```

### 2. Timeouts

Configure appropriate timeouts:

```go
upgrader := hyperserve.Upgrader{
    HandshakeTimeout: 10 * time.Second,
}

// In your handler
conn.SetReadDeadline(time.Now().Add(60 * time.Second))
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(60 * time.Second))
    return nil
})
```

### 3. Error Handling

Provide custom error responses:

```go
upgrader := hyperserve.Upgrader{
    Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
        // Log the error
        logger.Error("WebSocket upgrade failed", 
            "status", status,
            "error", reason,
            "remote", r.RemoteAddr,
        )
        
        // Send appropriate response
        http.Error(w, reason.Error(), status)
    },
}
```

### 4. Graceful Shutdown

Handle connection cleanup properly:

```go
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()
    
    // Register connection for graceful shutdown
    srv.RegisterConnection(conn)
    defer srv.UnregisterConnection(conn)
    
    // Your WebSocket logic here
}
```

## Frame Parser Details

The internal frame parser (`internal/ws/frame.go`) implements:

- **RFC 6455 Compliance**: Full protocol support
- **Fragmentation**: Handles fragmented messages
- **Control Frames**: Ping/Pong/Close handling
- **Masking**: Client-to-server masking validation
- **Extensions**: RSV bits for future extensions

## Testing WebSocket Endpoints

Example test for WebSocket functionality:

```go
func TestWebSocketEndpoint(t *testing.T) {
    srv, _ := hyperserve.NewServer()
    
    upgrader := hyperserve.Upgrader{
        CheckOrigin: func(r *http.Request) bool {
            return true // Allow all origins in tests
        },
    }
    
    srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            t.Errorf("Upgrade failed: %v", err)
            return
        }
        defer conn.Close()
        
        // Echo server
        for {
            mt, msg, err := conn.ReadMessage()
            if err != nil {
                break
            }
            if err := conn.WriteMessage(mt, msg); err != nil {
                break
            }
        }
    })
    
    // Test the upgrade
    ts := httptest.NewServer(srv.mux)
    defer ts.Close()
    
    // Make WebSocket request
    req, _ := http.NewRequest("GET", ts.URL+"/ws", nil)
    req.Header.Set("Upgrade", "websocket")
    req.Header.Set("Connection", "Upgrade")
    req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
    req.Header.Set("Sec-WebSocket-Version", "13")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("Request failed: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 101 {
        t.Errorf("Expected status 101, got %d", resp.StatusCode)
    }
}
```

## Common Issues and Solutions

### Issue: "response writer does not support hijacking"

**Cause**: Custom middleware not preserving the Hijacker interface

**Solution**: Ensure your middleware implements the Hijack method:

```go
func (w *yourResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
    return w.ResponseWriter.(http.Hijacker).Hijack()
}
```

### Issue: Origin check failures

**Cause**: Browser sending Origin header that doesn't match expectations

**Solution**: Log origins during development to understand the pattern:

```go
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    log.Printf("WebSocket origin: %s", origin)
    // Your check logic
}
```

### Issue: Message size errors

**Cause**: Client sending messages larger than MaxMessageSize

**Solution**: Configure appropriate limits on both client and server:

```go
upgrader := hyperserve.Upgrader{
    MaxMessageSize: 10 * 1024 * 1024, // 10MB
}
```

## Performance Considerations

1. **Buffer Sizes**: Configure based on your message patterns
2. **Concurrent Connections**: Each WebSocket uses one goroutine minimum
3. **Message Broadcasting**: Use efficient fan-out patterns for multi-client scenarios
4. **Memory Usage**: Be mindful of message buffering with many connections

## Security Checklist

- [ ] Origin validation configured appropriately
- [ ] Authentication implemented (if required)
- [ ] Message size limits set
- [ ] Timeout handling implemented
- [ ] Rate limiting considered
- [ ] Input validation on messages
- [ ] TLS/WSS used in production
- [ ] Subprotocol validation (if using)
- [ ] Error messages don't leak sensitive information
- [ ] Connection limits implemented