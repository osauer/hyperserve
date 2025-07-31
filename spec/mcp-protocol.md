# Model Context Protocol (MCP) Specification

This document defines the MCP implementation requirements for HyperServe.

## Protocol Version

Both implementations MUST support MCP version `2024-11-05`.

## Transport Support

### HTTP Transport (Required)
- Endpoint: `/mcp` (configurable via `HS_MCP_ENDPOINT`)
- Method: `POST`
- Content-Type: `application/json`
- Response: JSON-RPC 2.0

### SSE Transport (Required)
- Same endpoint with `Accept: text/event-stream`
- Unified endpoint design (not `/mcp/sse`)
- Header-based routing with `X-SSE-Client-ID`

## Required Methods

### Core Methods
1. `initialize` - Initialize MCP session
2. `initialized` - Client confirmation
3. `ping` - Connectivity test
4. `tools/list` - List available tools
5. `tools/call` - Execute a tool
6. `resources/list` - List available resources
7. `resources/read` - Read a resource

### Response Formats

#### Initialize
```json
{
  "jsonrpc": "2.0",
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {"listChanged": false},
      "resources": {"subscribe": false, "listChanged": false},
      "sse": {"enabled": true, "endpoint": "same", "headerRouting": true}
    },
    "serverInfo": {
      "name": "hyperserve",
      "version": "1.0.0"
    }
  },
  "id": 1
}
```

#### Tools List
```json
{
  "jsonrpc": "2.0",
  "result": {
    "tools": [
      {
        "name": "tool_name",
        "description": "Tool description",
        "inputSchema": {
          "type": "object",
          "properties": {},
          "required": []
        }
      }
    ]
  },
  "id": 2
}
```

## Built-in Tools (When Enabled)

When `HS_MCP_TOOLS_ENABLED=true`:

1. **read_file** - Read file contents
2. **list_directory** - List directory contents
3. **http_request** - Make HTTP requests
4. **calculator** - Basic arithmetic

## Built-in Resources (When Enabled)

When `HS_MCP_RESOURCES_ENABLED=true`:

1. **config** - Server configuration
2. **metrics** - Server metrics
3. **system** - System information
4. **logs** - Recent log entries

## Namespace Support

Tools and resources can be namespaced:
- Format: `mcp__namespace__name`
- Example: `mcp__hyperserve__read_file`

## Discovery Endpoints

Both implementations MUST provide:
- `GET /.well-known/mcp.json` - Standard discovery
- `GET /mcp/discover` - Alternative discovery

Discovery response:
```json
{
  "type": "mcp-server",
  "version": "2024-11-05",
  "serverInfo": {
    "name": "hyperserve",
    "version": "1.0.0"
  },
  "transports": [
    {
      "type": "http",
      "endpoint": "/mcp"
    },
    {
      "type": "sse",
      "endpoint": "/mcp"
    }
  ]
}
```

## Error Handling

Use standard JSON-RPC 2.0 error codes:
- `-32700` - Parse error
- `-32600` - Invalid request
- `-32601` - Method not found
- `-32602` - Invalid params
- `-32603` - Internal error

## Security

1. Tools/resources disabled by default
2. File operations sandboxed to configured root
3. No shell execution in built-in tools
4. Rate limiting applies to MCP endpoints