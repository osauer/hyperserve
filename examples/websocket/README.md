# WebSocket Examples

This directory contains examples demonstrating WebSocket functionality with hyperserve.

## Examples

### 1. Echo Server (`echo.go`)
A simple WebSocket echo server that reflects messages back to the client.

### 2. DAW Progress Updates (`daw.go`)
Demonstrates real-time progress updates for Digital Audio Workstation rendering, the original use case for WebSocket support.

### 3. Chat Demo (`chat.go`)
A multi-user chat application showcasing real-time messaging capabilities.

### 4. Demo HTML (`demo.html`)
An HTML page that demonstrates WebSocket connectivity and functionality.

## Running the Examples

```bash
# Run the echo server
go run echo.go

# Run the DAW progress server
go run daw.go

# Run the chat server
go run chat.go
```

Then open `demo.html` in your browser to interact with the WebSocket endpoints.

## WebSocket Support in hyperserve

hyperserve now supports WebSocket connections through the `http.Hijacker` interface. The `loggingResponseWriter` middleware properly implements hijacking to enable WebSocket upgrades while maintaining compatibility with the existing middleware stack.

### Key Features

- **Middleware Compatibility**: WebSocket upgrades work through the complete middleware stack
- **Gorilla WebSocket Support**: Full compatibility with the popular gorilla/websocket library
- **Real-time Communication**: Perfect for progress updates, chat applications, and live data streaming
- **Production Ready**: Maintains all existing hyperserve performance characteristics