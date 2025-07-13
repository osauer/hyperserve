# MCP Development Mode Example

This example demonstrates hyperserve's MCP Developer Mode, which provides AI assistants like Claude Code with powerful tools to help build and debug applications.

## ⚠️ Security Warning

**NEVER enable developer mode in production!** 

The developer tools allow:
- Server restart and reload
- Dynamic log level changes
- Request capture and replay
- Route inspection

These capabilities are designed for local development only. When enabled, a prominent warning will be logged.

## Features

### Developer Tools

1. **server_control** - Manage server lifecycle
   - Restart server
   - Reload configuration
   - Change log levels dynamically
   - Get server status

2. **route_inspector** - Inspect registered routes
   - List all routes
   - View middleware chains
   - Filter by pattern

3. **request_debugger** - Debug HTTP requests
   - Capture requests and responses
   - Replay requests with modifications
   - List recent requests

### Developer Resources

1. **logs://server/stream** - Real-time log streaming
2. **routes://server/all** - All registered routes
3. **requests://debug/recent** - Recent captured requests

## Running the Example

```bash
# Run the server
go run main.go
```

You'll see a warning in the logs:
```
⚠️  MCP DEVELOPER MODE ENABLED ⚠️
```

## Using with Claude Desktop

1. Configure Claude Desktop:
```json
{
  "mcpServers": {
    "my-dev-app": {
      "command": "go",
      "args": ["run", "main.go"],
      "cwd": "/path/to/examples/mcp-development"
    }
  }
}
```

2. Ask Claude to help:
- "Show me all registered routes"
- "Set the log level to DEBUG"
- "Show me recent requests to /api/error"
- "Restart the server"
- "Show me the server logs"

## Using with HTTP Transport

```bash
# Query available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 1
  }'

# Change log level
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "server_control",
      "arguments": {
        "action": "set_log_level",
        "log_level": "DEBUG"
      }
    },
    "id": 2
  }'
```

## Best Practices

1. **Development Only** - Never enable in production
2. **Local Development** - Keep dev servers on localhost
3. **Monitor Logs** - Watch for the developer mode warning
4. **Clean Up** - Remove developer mode before deployment
5. **Network Security** - Don't expose dev servers to public networks

## Integration Tips

For AI-assisted development:

1. **Clear Error Messages** - The AI can see your logs
2. **Descriptive Routes** - Help the AI understand your API
3. **Good Logging** - Log important operations
4. **Request Context** - Include relevant info in errors

Example of AI-friendly code:
```go
srv.HandleFunc("/api/users/:id", func(w http.ResponseWriter, r *http.Request) {
    userID := r.PathValue("id")
    logger.Debug("Fetching user", "id", userID, "method", r.Method)
    
    user, err := db.GetUser(userID)
    if err != nil {
        logger.Error("Failed to fetch user", "id", userID, "error", err)
        http.Error(w, fmt.Sprintf("User not found: %s", userID), 404)
        return
    }
    
    // ... rest of handler
})
```