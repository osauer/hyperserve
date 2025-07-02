# MCP Example

Demonstrates hyperserve's Model Context Protocol (MCP) support, enabling AI assistants to interact with your server.

## Features

- JSON-RPC 2.0 protocol endpoint
- Built-in tools (calculator, file operations, HTTP requests)
- Built-in resources (config, metrics, system info, logs)
- Sandboxed file access using Go 1.24's os.Root
- Rate-limited MCP endpoint
- Template-based dashboard

## Usage

```bash
# Run the server
go run main.go

# Visit the dashboard
open http://localhost:8080

# Test MCP endpoint
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

## Configuration

Environment variables:
- `HS_MCP_ENDPOINT` - Change MCP endpoint (default: /mcp)
- `HS_RATE_LIMIT` - Requests per second (default: 1)
- `HS_PORT` - Server port (default: 8080)