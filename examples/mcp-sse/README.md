# MCP with Server-Sent Events Example

Demonstrates HyperServe's unified MCP endpoint: the same `/mcp` URL serves
both regular HTTP POST requests and SSE streams, selected by the `Accept`
header.

## Running

The example is a single binary with a `-mode` flag. Open two terminals:

```bash
# Terminal 1 — server
go run ./examples/mcp-sse -mode=server

# Terminal 2 — client (connects to the server above and exercises the API)
go run ./examples/mcp-sse -mode=client
```

## Key Points

- **Single Endpoint**: Both HTTP POSTs and SSE streams use `/mcp`.
- **Header-Based Routing**: `Accept: text/event-stream` opens an SSE stream;
  otherwise the request is treated as a regular JSON-RPC POST.
- **Connection event**: On stream open, the server emits a `connection`
  event carrying a per-client `clientId` and `bindingToken`.
- **Routed POSTs need two headers**: subsequent POSTs that target the SSE
  stream MUST include BOTH `X-SSE-Client-ID` and `X-SSE-Binding` headers.
  The binding token is the capability — the client ID alone is not enough
  to inject requests into another client's stream. Missing/wrong binding
  returns 403.

## Example Flow

1. Client connects with `Accept: text/event-stream` → receives
   `clientId` + `bindingToken` in the `connection` event.
2. Client sends JSON-RPC requests via `POST /mcp` with the two headers.
3. Server processes each request and delivers the response over the SSE
   stream (event type `message`).
4. A periodic `ping` event keeps the stream alive.

See [main.go](./main.go) for the full implementation.