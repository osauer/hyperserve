# MCP Integration Guide

This guide covers how to use HyperServe's Model Context Protocol (MCP) support for AI-assisted development and production monitoring.

## Overview

HyperServe provides native MCP support through three main configurations:

1. **Development** (`MCPDev()`) - Tools for local development with Claude Code
2. **Observability** (`MCPObservability()`) - Safe monitoring for production
3. **Custom Extensions** - Your own tools and resources

HyperServe supports two transport mechanisms for MCP:
- **HTTP** - Traditional request/response over POST requests
- **SSE (Server-Sent Events)** - Real-time bidirectional communication

## Server-Sent Events (SSE) Support

HyperServe's MCP implementation uses a **unified endpoint approach** - both regular HTTP and SSE connections use the same endpoint path. The server automatically routes based on request headers.

### How SSE Works

1. **Connect with SSE**: Add `Accept: text/event-stream` header to connect via SSE
2. **Get Client ID**: Server returns a unique client ID in the connection event
3. **Send Requests**: Use the same endpoint with `X-SSE-Client-ID` header
4. **Receive Responses**: Responses are delivered through the SSE stream

### Example Usage

```bash
# 1. Connect to SSE (keep this connection open)
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp

# Response:
# event: connection
# data: {"clientId":"abc123"}

# 2. Send requests with the client ID
curl -X POST http://localhost:8080/mcp \
  -H "X-SSE-Client-ID: abc123" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# 3. Response comes through the SSE connection
```

### Benefits of SSE

- **Real-time Updates**: Receive notifications and async responses instantly
- **Bidirectional**: Send requests while maintaining an open connection
- **Automatic Keepalive**: Built-in ping/pong every 30 seconds
- **Single Endpoint**: No need to configure separate SSE paths

### When to Use SSE vs HTTP

- **Use HTTP** for: Simple request/response, AI assistants like Claude Code
- **Use SSE** for: Live monitoring, debugging sessions, real-time notifications

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

**mcp__hyperserve__server_control**
- `restart` - Restart the server process
- `reload` - Reload configuration without restart
- `set_log_level` - Change log level (DEBUG, INFO, WARN, ERROR)
- `get_status` - Get server status

**mcp__hyperserve__route_inspector**
- List all registered routes
- View middleware chains
- Filter routes by pattern

**mcp__hyperserve__request_debugger**
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

## Namespace Support

HyperServe supports organizing MCP tools and resources into namespaces for better organization and to avoid naming conflicts.

### Registering Tools in Namespaces

```go
// Register a tool in a specific namespace
srv.RegisterMCPToolInNamespace(tool, "daw")
// This tool will be accessible as "mcp__daw__play"

// Register a resource in a namespace
srv.RegisterMCPResourceInNamespace(resource, "analytics")
// This resource will be accessible as "mcp__analytics__dashboard"
```

### Registering Entire Namespaces

```go
// Register a complete namespace with tools and resources
err := srv.RegisterMCPNamespace("daw",
    WithNamespaceTools(
        NewCalculatorTool(),
        NewFileReadTool(),
    ),
    WithNamespaceResources(
        NewStatusResource(),
        NewMetricsResource(),
    ),
)
```

### Calling Namespaced Tools

When calling tools that are in namespaces, use the full prefixed name:

```bash
# Call a namespaced tool
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "mcp__daw__calculator",
      "arguments": {
        "operation": "add",
        "a": 5,
        "b": 3
      }
    },
    "id": 1
  }'
```

### Default Namespace

- Tools/resources registered without a namespace maintain their original names for backward compatibility
- When using namespace methods with an empty namespace, the server name is used as the default namespace

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
      "name": "mcp__hyperserve__server_control",
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

## Server-Sent Events (SSE) Support

HyperServe includes built-in SSE support for real-time MCP communication. This enables:
- Real-time server-to-client notifications
- Lower latency for interactive tools
- Better support for streaming responses

### SSE Endpoints

When MCP is enabled, HyperServe automatically provides:
- `/mcp` - Standard HTTP endpoint
- `/mcp/sse` - SSE endpoint for real-time communication

### Using SSE from JavaScript

```javascript
// Connect to SSE endpoint
const eventSource = new EventSource('/mcp/sse');
let clientId = null;

// Handle connection
eventSource.addEventListener('connection', (e) => {
    const data = JSON.parse(e.data);
    clientId = data.clientId;
    console.log('Connected:', clientId);
});

// Handle responses
eventSource.addEventListener('message', (e) => {
    const response = JSON.parse(e.data);
    console.log('Response:', response);
});

// Send requests with SSE routing
async function callMethod(method, params) {
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
    // Response comes via SSE, not HTTP body
}
```

### SSE Features

- Automatic reconnection on disconnect
- Keepalive pings every 30 seconds
- Buffered message delivery
- Thread-safe connection management
- Proper MCP lifecycle support

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