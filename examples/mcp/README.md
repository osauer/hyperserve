# MCP (Model Context Protocol) Example

This example demonstrates HyperServe's native support for the Model Context Protocol (MCP), enabling AI assistants to connect and interact with the server through standardized tools and resources.

## What is MCP?

The Model Context Protocol is an open standard that allows AI assistants like Claude to interact with servers through:
- **Tools**: Execute operations (calculations, file reading, HTTP requests)
- **Resources**: Access server information (config, metrics, logs)
- **JSON-RPC 2.0**: Standardized communication protocol
- **Security**: Sandboxed file operations using Go 1.24's `os.Root`

## Running the Example

```bash
go run main.go
```

The server will start on:
- Main server: http://localhost:8080
- Health checks: http://localhost:9080
- MCP endpoint: http://localhost:8080/mcp

## Features Demonstrated

### 🛠️ Built-in Tools
1. **calculator** - Basic math operations (add, subtract, multiply, divide)
2. **read_file** - Read files from the sandbox directory
3. **list_directory** - List directory contents
4. **http_request** - Make HTTP requests to external services

### 📊 Built-in Resources
1. **config://server/options** - Server configuration (sanitized)
2. **metrics://server/stats** - Performance metrics
3. **system://runtime/info** - System and Go runtime information
4. **logs://server/recent** - Recent log entries

### 🔒 Security Features
- File operations restricted to `./sandbox/` directory
- Uses Go 1.24's `os.Root` for secure file access
- Sanitized configuration output (no sensitive data)

## Testing the MCP Protocol

### 1. Initialize Connection
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "test-client", "version": "1.0"}
    },
    "id": 1
  }'
```

### 2. List Available Tools
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "tools/list", "id": 2}'
```

### 3. Use Calculator
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "calculator",
      "arguments": {"operation": "multiply", "a": 15, "b": 4}
    },
    "id": 3
  }'
```

### 4. Read a File
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "read_file",
      "arguments": {"path": "welcome.txt"}
    },
    "id": 4
  }'
```

### 5. List Directory
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "list_directory",
      "arguments": {"path": "."}
    },
    "id": 5
  }'
```

### 6. Read System Information
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {"uri": "system://runtime/info"},
    "id": 6
  }'
```

## Sample Files

The example creates several sample files in the `./sandbox/` directory:
- `welcome.txt` - Introduction to MCP features
- `data.json` - Sample JSON data
- `numbers.txt` - Simple number list
- `config.yaml` - Sample configuration
- `subdir/nested.txt` - File in subdirectory

## Using with AI Assistants

This server is designed to work with AI assistants that support MCP. The assistant can:
1. Connect to the MCP endpoint
2. Discover available tools and resources
3. Execute tools to perform tasks
4. Read resources to understand server state

## Customization

To add custom tools or resources:

```go
// Custom tool
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "My custom tool" }
func (t *MyTool) Schema() map[string]interface{} { 
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "input": map[string]interface{}{
                "type": "string",
                "description": "Input parameter",
            },
        },
        "required": []string{"input"},
    }
}
func (t *MyTool) Execute(params map[string]interface{}) (interface{}, error) {
    // Implementation
    return map[string]interface{}{"result": "success"}, nil
}

// Register in main.go after server creation
if srv.mcpHandler != nil {
    srv.mcpHandler.RegisterTool(&MyTool{})
}
```

## Architecture

The MCP implementation consists of:
- `mcp.go` - Core MCP handler and protocol implementation
- `mcp_tools.go` - Built-in tool implementations
- `mcp_resources.go` - Built-in resource implementations
- `jsonrpc.go` - JSON-RPC 2.0 protocol handling

## Performance

- Zero overhead when MCP is disabled
- Lazy initialization of components
- Direct handler registration (not middleware-based)
- Efficient JSON-RPC routing

## Security Considerations

1. **File Access**: Restricted to sandbox directory
2. **HTTP Requests**: Consider adding URL filtering for production
3. **Rate Limiting**: Apply standard HyperServe rate limiting
4. **Authentication**: Can be combined with auth middleware
5. **Resource Access**: Sensitive data is filtered from config resource