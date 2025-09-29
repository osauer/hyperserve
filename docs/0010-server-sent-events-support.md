# ADR-0010: Server-Sent Events as First-Class Feature

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Modern web applications increasingly need real-time server-to-client communication:
- Live dashboards and monitoring
- Progress updates for long operations  
- Real-time notifications
- Streaming data feeds

Common approaches include:
- **WebSockets**: Bidirectional but complex, requires upgrade
- **Long polling**: Simple but inefficient
- **Server-Sent Events (SSE)**: Unidirectional, simple, works over HTTP

SSE is often overlooked despite being simpler than WebSockets for many use cases.

## Decision

Provide Server-Sent Events as a first-class feature:
- Built-in SSE handler support
- Helper functions for SSE message formatting
- Automatic connection management
- Proper Content-Type and headers

```go
// Simple SSE endpoint
srv.HandleFunc("/events", server.SSEHandler(func(w http.ResponseWriter, r *http.Request, send chan<- server.SSEMessage) {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            send <- server.NewSSEMessage("time", time.Now().String())
        case <-r.Context().Done():
            return
        }
    }
}))
```

## Consequences

### Positive
- **Simplicity**: Easier than WebSockets for server-to-client
- **HTTP compatible**: Works through proxies and firewalls
- **Browser support**: Native EventSource API
- **Auto-reconnect**: Browsers handle reconnection
- **Standards-based**: W3C standard protocol

### Negative
- **Unidirectional**: Server-to-client only
- **Text-only**: Binary data needs encoding
- **Connection limits**: Browser limits concurrent connections
- **No IE support**: Internet Explorer doesn't support SSE

### Mitigation
- Document WebSocket alternative for bidirectional needs
- Provide base64 encoding helpers for binary data
- Connection pooling best practices
- Polyfill recommendations for old browsers

## Implementation Details

SSE protocol requirements:
```
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

data: First message\n\n
event: custom-event\n
data: {"json": "data"}\n\n
id: msg-123\n
data: Message with ID\n\n
```

Helper functions:
```go
type SSEMessage struct {
    Event string
    Data  string
    ID    string
}

func NewSSEMessage(event, data string) SSEMessage
func (m SSEMessage) String() string  // Formats as SSE protocol
```

## Examples

```go
// Time ticker example
srv.HandleFunc("/time", server.SSEHandler(func(w http.ResponseWriter, r *http.Request, send chan<- server.SSEMessage) {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            send <- server.NewSSEMessage("tick", time.Now().Format(time.RFC3339))
        case <-r.Context().Done():
            return
        }
    }
}))

// Progress updates
srv.HandleFunc("/upload/progress", server.SSEHandler(func(w http.ResponseWriter, r *http.Request, send chan<- server.SSEMessage) {
    uploadID := r.URL.Query().Get("id")
    
    for progress := range getUploadProgress(uploadID) {
        send <- server.NewSSEMessage("progress", fmt.Sprintf("%d", progress))
        if progress >= 100 {
            break
        }
    }
}))

// Client-side JavaScript
const events = new EventSource('/time');
events.addEventListener('tick', (e) => {
    console.log('Time:', e.data);
});
```

## Best Practices

1. Always handle client disconnection via `Context.Done()`
2. Use event types for different message categories
3. Include message IDs for resumption after reconnect
4. Send periodic keep-alive comments to prevent timeout
5. Limit concurrent SSE connections per client