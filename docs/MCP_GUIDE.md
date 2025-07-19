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

## Multiple Namespaces

HyperServe supports organizing tools and resources into multiple namespaces within a single server instance. This enables logical separation of concerns and clearer organization for MCP clients.

### Why Use Namespaces?

When building applications with MCP support, you often have different categories of tools:
- **Server operations**: `server_control`, `route_inspector`, `request_debugger`
- **Application functionality**: `user_create`, `post_publish`, `data_export`
- **Domain-specific tools**: `audio_analyze`, `image_process`, `daw_play`

Without namespaces, all tools appear under the default namespace (e.g., `mcp__hyperserve__*`). With namespaces, they can be organized logically:
- `mcp__hyperserve__server_control` (server operations)
- `mcp__app__user_create` (application functionality)
- `mcp__audio__analyze` (domain-specific features)

### Creating Namespaces

#### During Server Creation

```go
srv, err := hyperserve.NewServer(
    hyperserve.WithMCPSupport("hyperserve", "1.0.0"),
    
    // Add DAW control namespace
    hyperserve.WithMCPNamespace("daw",
        hyperserve.WithNamespaceTools(
            NewPlayTool(),
            NewStopTool(),
            NewBPMTool(),
        ),
        hyperserve.WithNamespaceResources(
            NewTrackListResource(),
        ),
    ),
    
    // Add audio processing namespace
    hyperserve.WithMCPNamespace("audio",
        hyperserve.WithNamespaceTools(
            NewAnalyzeTool(),
            NewDecomposeTool(),
        ),
    ),
)
```

#### After Server Creation

```go
// Register namespace with tools and resources
err := srv.RegisterMCPNamespace("analytics",
    hyperserve.WithNamespaceTools(queryTool, reportTool),
    hyperserve.WithNamespaceResources(statsResource),
)

// Or register individual tools/resources in a namespace
err = srv.RegisterMCPToolInNamespace(customTool, "utilities")
err = srv.RegisterMCPResourceInNamespace(configResource, "system")
```

### Tool and Resource Naming

When tools and resources are registered in a namespace, they get prefixed automatically:

**Original Names → Namespaced Names**
- `play` → `mcp__daw__play`
- `analyze` → `mcp__audio__analyze`
- `config://app` → `mcp__system__config://app`

**Backward Compatibility**
Tools registered with the original `RegisterMCPTool()` keep their original names (no prefix).

### Complete Example

```go
package main

import (
    "context"
    "log"
    "github.com/osauer/hyperserve"
)

func main() {
    // Create tools for different domains
    playTool := hyperserve.NewTool("play").
        WithDescription("Play audio track").
        WithParameter("track_id", "string", "Track ID", true).
        WithExecute(func(params map[string]interface{}) (interface{}, error) {
            trackID := params["track_id"].(string)
            return map[string]interface{}{
                "status": "playing",
                "track": trackID,
            }, nil
        }).Build()
    
    analyzeTool := hyperserve.NewTool("analyze").
        WithDescription("Analyze audio file").
        WithParameter("file_path", "string", "Audio file path", true).
        WithExecute(func(params map[string]interface{}) (interface{}, error) {
            filePath := params["file_path"].(string)
            return map[string]interface{}{
                "duration": 240.5,
                "tempo": 120,
                "key": "C major",
            }, nil
        }).Build()
    
    trackResource := hyperserve.NewResource("tracks://active").
        WithName("Active Tracks").
        WithDescription("Currently loaded tracks").
        WithRead(func() (interface{}, error) {
            return []string{"track1.wav", "track2.mp3"}, nil
        }).Build()
    
    // Create server with multiple namespaces
    srv, err := hyperserve.NewServer(
        hyperserve.WithAddr(":8080"),
        hyperserve.WithMCPSupport("hyperserve", "1.0.0"),
        
        // DAW control namespace
        hyperserve.WithMCPNamespace("daw",
            hyperserve.WithNamespaceTools(playTool),
            hyperserve.WithNamespaceResources(trackResource),
        ),
        
        // Audio processing namespace
        hyperserve.WithMCPNamespace("audio",
            hyperserve.WithNamespaceTools(analyzeTool),
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Tools are now available as:
    // - mcp__daw__play
    // - mcp__audio__analyze
    // Resources are available as:
    // - mcp__daw__tracks://active
    
    log.Println("Server starting with namespaced MCP tools...")
    srv.Run(context.Background())
}
```

### MCP Client Usage

When connecting from MCP clients (Claude Desktop, etc.), namespaced tools appear with their full names:

```bash
# List all tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Response includes:
# {
#   "result": {
#     "tools": [
#       {"name": "mcp__daw__play", "description": "Play audio track"},
#       {"name": "mcp__audio__analyze", "description": "Analyze audio file"}
#     ]
#   }
# }

# Call a namespaced tool
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "mcp__daw__play",
      "arguments": {"track_id": "track1.wav"}
    },
    "id": 2
  }'
```

### Namespace Organization Tips

1. **Server Operations** (`hyperserve`): Server control, debugging, monitoring
2. **Application Logic** (`app`): Business logic, user management, data operations
3. **Domain Features** (`audio`, `image`, `daw`, etc.): Specialized functionality
4. **System Tools** (`system`): Configuration, file operations, environment

```go
// Recommended namespace structure
WithMCPNamespace("hyperserve", ...) // Server operations (optional, this is default)
WithMCPNamespace("app", ...)        // Core application features
WithMCPNamespace("audio", ...)      // Audio processing tools
WithMCPNamespace("system", ...)     // System administration
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