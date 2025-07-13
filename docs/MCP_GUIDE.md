# MCP Integration Guide

This guide covers how to use HyperServe's Model Context Protocol (MCP) support for AI-assisted development and production monitoring.

## Overview

HyperServe provides native MCP support through three main configurations:

1. **Development** (`MCPDev()`) - Tools for local development with Claude Code
2. **Observability** (`MCPObservability()`) - Safe monitoring for production
3. **Custom Extensions** - Your own tools and resources

## Development with Claude Code

### Quick Start (Recommended)

Use flags or environment variables to avoid hardcoding development settings:

```bash
# Using flags
./myapp --mcp --mcp-dev

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_DEV=true ./myapp
```

### Claude Code Configuration (HTTP)

```json
{
  "mcpServers": {
    "myapp-local": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

### Claude Desktop Configuration (STDIO)

1. Build your application:
```bash
go build -o myapp
```

2. Configure Claude Desktop:
```json
{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/myapp",
      "args": ["--mcp", "--mcp-dev", "--mcp-transport=stdio"],
      "cwd": "/path/to/project"
    }
  }
}
```

3. Start developing with Claude:
- "Set log level to DEBUG"
- "Show me all routes"
- "Restart the server"
- "Capture the next POST request"

### Available Tools

**server_control**
- `restart` - Restart the server process
- `reload` - Reload configuration without restart
- `set_log_level` - Change log level (DEBUG, INFO, WARN, ERROR)
- `get_status` - Get server status

**route_inspector**
- List all registered routes
- View middleware chains
- Filter routes by pattern

**request_debugger**
- Capture HTTP requests
- List captured requests
- Replay requests with modifications

### Security Warning

⚠️ **Never use MCPDev() in production!** It enables dangerous operations like server restart.

## Production Observability

### Setup

```bash
# Using flags
./myapp --mcp --mcp-observability

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_OBSERVABILITY=true ./myapp
```

### Available Resources

**config://server/current**
- Server version and build info
- Network configuration
- Feature flags
- No secrets or sensitive paths

**health://server/status**
- Liveness and readiness
- Uptime
- Request metrics
- Average response time

**logs://server/recent**
- Recent log entries (default: last 100)
- Structured log format
- Circular buffer for memory efficiency

### Remote Access

For production monitoring via Claude:

```json
{
  "mcpServers": {
    "prod-monitor": {
      "command": "ssh",
      "args": [
        "-o", "StrictHostKeyChecking=yes",
        "prod-server",
        "curl", "-s", "http://localhost:8080/mcp"
      ]
    }
  }
}
```

## Custom Extensions

### Simple Tool

```go
tool := hyperserve.NewTool("deploy").
    WithDescription("Deploy application").
    WithParameter("version", "string", "Version to deploy", true).
    WithParameter("environment", "string", "Target environment", true).
    WithExecute(func(params map[string]interface{}) (interface{}, error) {
        version := params["version"].(string)
        env := params["environment"].(string)
        
        // Your deployment logic here
        return map[string]interface{}{
            "status": "deployed",
            "version": version,
            "environment": env,
        }, nil
    }).
    Build()

srv.RegisterMCPTool(tool)
```

### Simple Resource

```go
resource := hyperserve.NewResource("app://stats/users").
    WithName("User Statistics").
    WithDescription("Current user statistics").
    WithRead(func() (interface{}, error) {
        return map[string]interface{}{
            "total_users": getUserCount(),
            "active_today": getActiveUsers(),
            "new_this_week": getNewUsers(),
        }, nil
    }).
    Build()

srv.RegisterMCPResource(resource)
```

### Complete Extension

```go
ext := hyperserve.NewMCPExtension("analytics").
    WithDescription("Analytics tools and data").
    WithTool(
        hyperserve.NewTool("query_metrics").
            WithParameter("metric", "string", "Metric name", true).
            WithParameter("timeframe", "string", "Time range", false).
            WithExecute(queryMetrics).
            Build(),
    ).
    WithResource(
        hyperserve.NewResource("analytics://dashboard/summary").
            WithRead(getDashboardData).
            Build(),
    ).
    Build()

srv.RegisterMCPExtension(ext)
```

## Best Practices

### 1. Security First
- Use `MCPObservability()` for production
- Never expose `MCPDev()` to networks
- Sanitize all data in resources
- Validate tool parameters

### 2. Clear Naming
```go
// Good
tool := NewTool("create_user")
resource := NewResource("users://active/list")

// Bad
tool := NewTool("do_thing")
resource := NewResource("data://stuff")
```

### 3. Error Handling
```go
WithExecute(func(params map[string]interface{}) (interface{}, error) {
    name, ok := params["name"].(string)
    if !ok {
        return nil, fmt.Errorf("name parameter required")
    }
    
    if err := validateName(name); err != nil {
        return nil, fmt.Errorf("invalid name: %w", err)
    }
    
    // ... rest of logic
})
```

### 4. Resource Caching
Resources are cached for 5 minutes by default. Design accordingly:
- Expensive queries benefit from caching
- Real-time data might need shorter TTL
- Use tools for operations that modify state

### 5. Documentation
Always provide clear descriptions:
```go
NewTool("backup_database").
    WithDescription("Create a database backup with optional encryption").
    WithParameter("encrypt", "boolean", "Enable encryption (default: true)", false).
    WithParameter("location", "string", "Backup location (s3|local)", false)
```

## Testing MCP Endpoints

### Using curl

```bash
# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 1
  }'

# Execute a tool
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "server_control",
      "arguments": {
        "action": "get_status"
      }
    },
    "id": 2
  }'

# Read a resource
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {
      "uri": "health://server/status"
    },
    "id": 3
  }'
```

## Troubleshooting

### MCP Not Working
1. Check logs for "MCP handler initialized"
2. Verify endpoint (default: `/mcp`)
3. Ensure tools/resources are registered before `Run()`

### Claude Desktop Connection Issues
1. Check `claude_desktop_config.json` syntax
2. Verify command path is absolute
3. Check server logs for connection attempts
4. Try HTTP transport first, then STDIO

### Performance Issues
1. Resources are cached (5 min default)
2. Use pagination for large datasets
3. Tools run with 30s timeout
4. Monitor MCP metrics via observability resources