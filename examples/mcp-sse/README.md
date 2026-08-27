# Legacy HyperServe Routed SSE Example

Demonstrates HyperServe's proprietary routed-SSE compatibility mode. This is
not MCP 2026-07-28 Streamable HTTP: it uses a standalone GET stream,
`X-SSE-*` headers, and non-standard connection/ping events. New MCP clients
should use the standards-compliant POST transport documented in
[MCP_GUIDE.md](../../docs/MCP_GUIDE.md).

The server in this example opts in with
`hyperserve.WithMCPLegacyRoutedSSE(true)`. Normal HyperServe MCP servers do not
expose this transport.

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
- **GET-only routing**: a GET with `Accept: text/event-stream` opens the legacy stream;
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
