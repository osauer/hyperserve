# WebSocket Examples

This directory contains examples demonstrating WebSocket functionality with hyperserve.

## Examples

### 1. Echo Server (`echo.go`)
A simple WebSocket echo server that reflects messages back to the client.

### 2. Chat Demo (`chat.go`)
A simple chat application showcasing real-time messaging capabilities.

### 3. Demo HTML (`demo.html`)
An HTML page that demonstrates WebSocket connectivity and functionality.

## Running the Examples

```bash
# Run the echo server
go run echo.go

# Run the chat server
go run chat.go
```

Then open `demo.html` in your browser to interact with the WebSocket endpoints.

## WebSocket Support in hyperserve

hyperserve supports WebSocket connections through the `http.Hijacker` interface using a standard library implementation. The `loggingResponseWriter` middleware properly implements hijacking to enable WebSocket upgrades while maintaining compatibility with the existing middleware stack.

### Key Features

- **Zero Dependencies**: WebSocket support using only Go standard library
- **Middleware Compatibility**: WebSocket upgrades work through the complete middleware stack
- **Real-time Communication**: Perfect for progress updates, chat applications, and live data streaming
- **Production Ready**: Maintains all existing hyperserve performance characteristics

### Usage

```go
upgrader := hyperserve.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true // Configure based on your needs
    },
}

srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }
    defer conn.Close()
    
    // Handle WebSocket messages
    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            break
        }
        // Echo message back
        conn.WriteMessage(messageType, p)
    }
})
```