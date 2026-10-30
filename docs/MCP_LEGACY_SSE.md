# Proprietary routed-SSE compatibility

This opt-in transport is retained for existing HyperServe clients. New clients
use [current Streamable HTTP](MCP_GUIDE.md#streamable-http-2026-07-28).

## Protocol and client reference

This proprietary compatibility stream enables:
- Real-time server-to-client notifications
- Lower latency for interactive tools
- Better support for streaming responses

It is not MCP 2026-07-28 Streamable HTTP: responses are delivered on a
separate long-lived GET stream with proprietary routing headers.

### Compatibility endpoints

When `WithMCPLegacyRoutedSSE(true)` is set, HyperServe provides:
- `/mcp` - Standard HTTP endpoint for JSON-RPC POST requests
- `/mcp` - legacy stream only for a GET sending `Accept: text/event-stream`

### Using SSE from JavaScript

```javascript
// Connect to the unified MCP endpoint as an SSE stream.
const eventSource = new EventSource('/mcp');
let clientId = null;
let bindingToken = null;

// Capture the per-stream capability emitted in the connection event.
// Both fields are required for any subsequent routed POST.
eventSource.addEventListener('connection', (e) => {
    const data = JSON.parse(e.data);
    clientId = data.clientId;
    bindingToken = data.bindingToken;
    console.log('Connected:', clientId);
});

// Handle responses
eventSource.addEventListener('message', (e) => {
    const response = JSON.parse(e.data);
    console.log('Response:', response);
});

// Send routed requests. Both headers MUST be set — the binding token is
// the capability, not the client ID. A wrong/missing token returns 403.
async function callMethod(method, params) {
    const response = await fetch('/mcp', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-SSE-Client-ID': clientId,
            'X-SSE-Binding': bindingToken
        },
        body: JSON.stringify({
            jsonrpc: '2.0',
            method: method,
            params: params,
            id: Date.now()
        })
    });
    // Response comes via SSE, not HTTP body
}
```

### SSE Features

- Automatic reconnection on disconnect
- Keepalive pings every 30 seconds
- Buffered message delivery
- Thread-safe connection management
- Proper MCP lifecycle support
- Resource subscription notifications for `SubscribableResourceTemplate`
  values registered on the handler
