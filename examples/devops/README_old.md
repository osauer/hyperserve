# DevOps Example

This example demonstrates hyperserve's DevOps features including debug logging control and minimal MCP resources for server introspection.

## Features Demonstrated

1. **Debug Mode Control** via environment variables
2. **MCP DevOps Resources** for monitoring
3. **Structured Logging** at different levels

## Running the Example

### Basic Usage

```bash
# Run with default INFO logging
go run main.go

# Enable debug mode
HS_DEBUG=true go run main.go

# Set specific log level
HS_LOG_LEVEL=WARN go run main.go
```

### With MCP HTTP Transport

```bash
# Run server with MCP enabled
go run main.go

# In another terminal, query MCP resources
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {"uri": "config://server/current"},
    "id": 1
  }'
```

### With Claude Desktop (MCP STDIO)

1. Build the binary:
```bash
go build -o devops-server
```

2. Add to Claude Desktop settings:
```json
{
  "mcpServers": {
    "devops-example": {
      "command": "/path/to/devops-server",
      "args": ["--mcp-stdio"],
      "env": {
        "HS_DEBUG": "true"
      }
    }
  }
}
```

3. In Claude, you can ask:
- "Show me the server configuration"
- "What's the current health status?"
- "Show me recent error logs"

## Available MCP Resources

### 1. Server Configuration
- **URI**: `config://server/current`
- **Content**: Current configuration, version info, enabled features

### 2. Server Health
- **URI**: `health://server/status`
- **Content**: Liveness, readiness, uptime, request metrics

### 3. Server Logs
- **URI**: `logs://server/recent`
- **Content**: Recent log entries (last 100 by default)

## Test Endpoints

- `/` - Home page (DEBUG log)
- `/test` - Test endpoint (INFO log)
- `/error` - Simulated error (ERROR log)

## Environment Variables

- `HS_DEBUG` - Enable debug mode (true/false)
- `HS_LOG_LEVEL` - Set log level (DEBUG, INFO, WARN, ERROR)
- `HS_MCP_LOG_RESOURCE_SIZE` - Number of log entries to keep (default: 100)