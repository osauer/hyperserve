# MCP Server-Sent Events (SSE) Support

HyperServe now includes built-in support for Server-Sent Events (SSE) as a transport mechanism for the Model Context Protocol (MCP). This enables real-time, bidirectional communication between MCP clients and servers.

## Overview

The SSE implementation provides:
- Real-time server-to-client communication
- Automatic reconnection handling
- Message queuing and delivery
- Proper MCP lifecycle management (initialize → initialized → ready)
- Thread-safe connection management

## Architecture

### Endpoints

- **HTTP Endpoint**: `/mcp` - Standard JSON-RPC 2.0 over HTTP
- **SSE Endpoint**: `/mcp/sse` - Server-Sent Events for real-time updates

### Connection Flow

1. Client connects to `/mcp/sse` via GET request
2. Server sends connection event with unique client ID
3. Client sends requests to `/mcp` with `X-SSE-Client-ID` header
4. Server processes requests and sends responses via SSE connection

## Usage

### Server Setup

```go
srv, err := hyperserve.NewServer(
    hyperserve.WithMCPSupport("MyApp", "1.0.0"),
    hyperserve.WithMCPBuiltinTools(true),
)
```

### Client Connection

```javascript
// Connect to SSE endpoint
const eventSource = new EventSource('/mcp/sse');
let clientId = null;

// Handle connection event
eventSource.addEventListener('connection', (e) => {
    const data = JSON.parse(e.data);
    clientId = data.clientId;
    console.log('Connected with ID:', clientId);
});

// Handle messages
eventSource.addEventListener('message', (e) => {
    const response = JSON.parse(e.data);
    console.log('Response:', response);
});

// Send requests with SSE client ID
async function sendRequest(method, params) {
    const response = await fetch('/mcp', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-SSE-Client-ID': clientId
        },
        body: JSON.stringify({
            jsonrpc: '2.0',
            method: method,
            params: params,
            id: Date.now()
        })
    });
    
    // Response comes via SSE, not HTTP response body
}
```

## Protocol Details

### SSE Event Types

- **connection**: Initial connection event with client ID
- **message**: JSON-RPC responses
- **notification**: Server-initiated notifications
- **ping**: Keepalive messages (every 30 seconds)

### Request Routing

When a client sends a request with the `X-SSE-Client-ID` header:
1. Server validates the client ID
2. Queues the request for processing
3. Returns HTTP 202 Accepted
4. Processes the request asynchronously
5. Sends response via SSE connection

### MCP Lifecycle

The SSE transport properly implements the MCP lifecycle:

1. **Initialize**: Client sends initialize request
2. **Server Response**: Server responds with capabilities
3. **Initialized**: Client sends initialized notification
4. **Ready**: Server sends ready notification
5. **Active**: Normal request/response flow

## Implementation Details

### Connection Management

- Each SSE client gets a unique ID
- Connections are tracked in a thread-safe map
- Automatic cleanup on disconnect
- Buffered channels for message queuing

### Error Handling

- Invalid client IDs return HTTP 400
- Full request queues return HTTP 503
- Connection errors are logged
- Graceful shutdown on server stop

### Performance Considerations

- Default message buffer: 100 messages per client
- Request queue buffer: 10 requests per client
- Keepalive interval: 30 seconds
- Automatic connection cleanup

## Testing

Run the example:

```bash
cd examples/mcp-sse
go run main.go
```

Then visit http://localhost:8080 for an interactive demo.

## Security Considerations

- Client IDs are randomly generated
- No authentication built-in (add middleware as needed)
- Rate limiting recommended for production
- CORS headers may be needed for browser clients