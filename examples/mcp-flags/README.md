# MCP Configuration Examples

This example demonstrates different ways to configure MCP support in HyperServe without hardcoding development settings in your source code.

## Running the Examples

### Basic Usage (No MCP)
```bash
go run main.go
```

### Enable MCP with Developer Tools (Claude Code)
```bash
# Using flags
go run main.go --mcp --mcp-dev

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_DEV=true go run main.go

# Or build and run
go build -o myapp
./myapp --mcp --mcp-dev
```

### Enable MCP with Observability (Production Monitoring)
```bash
./myapp --mcp --mcp-observability
```

### Claude Desktop Integration (STDIO)
```bash
# Build the binary first
go build -o myapp

# Test STDIO mode
./myapp --mcp --mcp-dev --mcp-transport=stdio
```

Then configure Claude Desktop:
```json
{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/myapp",
      "args": ["--mcp", "--mcp-dev", "--mcp-transport=stdio"]
    }
  }
}
```

### Claude Code Integration (HTTP)

1. Start your server with MCP dev tools:
```bash
./myapp --mcp --mcp-dev
```

2. Configure Claude Code to connect to your server:
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

## Configuration Methods

### 1. Command-Line Flags
```bash
./myapp \
  --mcp \
  --mcp-name="MyApp" \
  --mcp-version="2.0.0" \
  --mcp-dev \
  --mcp-transport=stdio
```

### 2. Environment Variables
```bash
export HS_MCP_ENABLED=true
export HS_MCP_SERVER_NAME="MyApp"
export HS_MCP_SERVER_VERSION="2.0.0"
export HS_MCP_DEV=true
export HS_MCP_TRANSPORT=stdio
./myapp
```

### 3. Configuration File (options.json)
```json
{
  "mcp_enabled": true,
  "mcp_server_name": "MyApp",
  "mcp_server_version": "2.0.0",
  "mcp_dev": true,
  "mcp_transport": "http"
}
```

### 4. In Code (Use Sparingly)
```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport("MyApp", "1.0.0",
        hyperserve.MCPDev(),  // Only in dev builds!
    ),
)
```

## Best Practices

1. **Never hardcode MCPDev() in production code** - Use flags or environment variables
2. **Use build tags for different environments** if you must configure in code
3. **Default to HTTP transport** - It's more flexible for remote access
4. **Use STDIO only for Claude Desktop** - It's designed for that use case
5. **Enable observability for production** - Safe, read-only monitoring

## Security Considerations

- `MCPDev()` enables dangerous operations (restart, reload, debug)
- Only enable in development environments
- Use `MCPObservability()` for production monitoring
- Consider network restrictions for MCP endpoints