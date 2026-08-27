# MCP Example

This example exposes HyperServe's Model Context Protocol support over HTTP so
an MCP client can call application tools and read application resources.

## Features

- JSON-RPC 2.0 protocol endpoint
- Built-in tools (calculator and sandboxed file operations)
- Built-in resources (config, metrics, system info, logs)
- Custom tool example (timestamp generator)
- Custom resource example (server status)
- Sandboxed file access using Go 1.24's os.Root
- Rate-limited MCP endpoint
- Template-based dashboard

## Usage

The curl commands below use HyperServe's explicit 2025-11-25 initialize-era
request/response fallback. New integrations should use the 2026-07-28 request
metadata documented in the [MCP guide](../../docs/MCP_GUIDE.md), including
`subscriptions/listen` for live updates.

```bash
# Run the server (from the repo root)
go run ./examples/mcp-basic

# Visit the dashboard
open http://localhost:8080

# Test MCP endpoint - list all tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Test custom timestamp tool
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"timestamp","arguments":{"format":"unix"}},"id":2}'

# Test custom server status resource
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"resources/read","params":{"uri":"custom://server/status"},"id":3}'
```

## Configuration

This example calls `WithEnvironment()` before its application-owned MCP
capabilities. Supported deployment overrides include:

- `HS_MCP_ENDPOINT` - Change MCP endpoint (default: /mcp)
- `HS_PORT` - Server port (default: 8080)

Rate limiting is explicit application policy, not server configuration.
Middleware is a request wrapper: create one gate, then place it in front of the
MCP path.

```go
mcpGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 50,
	Burst:             100,
})
if err != nil {
	log.Fatal(err)
}
app.UsePrefix("/mcp", mcpGate)
```

## Custom Tools and Resources

This example demonstrates how to implement and register custom MCP extensions:

### Custom Tool (timestamp)
- Generates timestamps in various formats (unix, iso8601, rfc3339)
- Shows proper schema definition for tool parameters
- Demonstrates parameter validation and execution

### Custom Resource (server status)
- Provides server status information
- Returns JSON-formatted data
- Shows how to implement the MCPResource interface

Check `main.go` for the complete implementation of `TimestampTool` and `ServerStatusResource`.

## Using with Claude Desktop

For Claude Desktop integration, see the [mcp-stdio example](../mcp-stdio) which provides a standalone stdio server that Claude can connect to directly.
