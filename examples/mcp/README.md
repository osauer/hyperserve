# MCP (Model Context Protocol) Example

This example demonstrates how to use Hyperserve with MCP (Model Context Protocol) support to create a server that AI assistants can connect to and interact with.

## What is MCP?

The Model Context Protocol (MCP) is an open standard that allows AI assistants to securely connect to data sources and tools. It provides:

- **Standardized access** to external data sources
- **Tool execution** capabilities
- **Real-time data streaming** to AI models
- **Security-focused** design with capability negotiation

## Features Demonstrated

### 🛠️ Built-in Tools
- **Calculator** - Basic mathematical operations (add, subtract, multiply, divide)
- **File Reader** - Secure file access within sandbox directory
- **Directory Lister** - List directory contents
- **HTTP Client** - Make HTTP requests to external services

### 📊 Built-in Resources
- **Server Configuration** - Access to server settings (sanitized)
- **Performance Metrics** - Real-time server statistics
- **System Information** - Runtime and system details
- **Recent Logs** - Access to recent log entries

### 🔒 Security Features
- **Sandboxed File Access** - File operations restricted to `./sandbox/` directory
- **os.Root Integration** - Uses Go 1.24's secure file access
- **JSON-RPC 2.0** - Standard protocol implementation
- **Capability Negotiation** - Clients can only access enabled features

## Running the Example

1. **Start the server:**
   ```bash
   cd examples/mcp
   go run main.go
   ```

2. **Access the web interface:**
   Open your browser to `http://localhost:8080` to see the example documentation and request samples.

3. **Health checks:**
   - General health: `http://localhost:9080/healthz/`
   - Readiness: `http://localhost:9080/readyz/`
   - Liveness: `http://localhost:9080/livez/`

## MCP Protocol Usage

### 1. Initialize Connection

First, establish a connection with the MCP server:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "example-client",
        "version": "1.0.0"
      }
    },
    "id": 1
  }'
```

### 2. List Available Tools

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 2
  }'
```

### 3. Use the Calculator Tool

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "calculator",
      "arguments": {
        "operation": "multiply",
        "a": 15,
        "b": 4
      }
    },
    "id": 3
  }'
```

### 4. Read a File from Sandbox

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "read_file",
      "arguments": {
        "path": "welcome.txt"
      }
    },
    "id": 4
  }'
```

### 5. List Directory Contents

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "list_directory",
      "arguments": {
        "path": "."
      }
    },
    "id": 5
  }'
```

### 6. Access Server Resources

List available resources:
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/list",
    "id": 6
  }'
```

Read system information:
```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {
      "uri": "system://runtime/info"
    },
    "id": 7
  }'
```

## Sandbox Directory

The example creates a `./sandbox/` directory with sample files:

- `welcome.txt` - Welcome message and documentation
- `data.json` - Sample JSON data
- `numbers.txt` - Simple list of numbers
- `config.yaml` - Sample configuration file
- `subdir/nested.txt` - File in subdirectory

All file operations are restricted to this directory for security.

## Configuration Options

The example demonstrates various MCP configuration options:

```go
srv, err := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithMCPSupport(),                    // Enable MCP
    hyperserve.WithMCPEndpoint("/mcp"),             // Custom endpoint
    hyperserve.WithMCPServerInfo("name", "1.0.0"), // Server identification
    hyperserve.WithMCPFileToolRoot(sandboxDir),    // Restrict file access
    // hyperserve.WithMCPToolsDisabled(),           // Disable tools
    // hyperserve.WithMCPResourcesDisabled(),       // Disable resources
)
```

## Error Handling

The MCP implementation includes comprehensive error handling:

- **Parse Errors** - Invalid JSON requests
- **Method Not Found** - Unknown MCP methods
- **Invalid Parameters** - Missing or incorrect parameters
- **Tool Errors** - Tool execution failures
- **Resource Errors** - Resource access failures

All errors are returned in standard JSON-RPC 2.0 error format.

## Integration with AI Assistants

This server can be used with any MCP-compatible AI assistant. The assistant can:

1. Connect to the server using the MCP protocol
2. Discover available tools and resources
3. Execute tools to perform operations
4. Read resources to access server information
5. Use the data to provide intelligent responses

## Next Steps

- **Add Custom Tools** - Implement application-specific tools
- **Add Custom Resources** - Expose application data as resources
- **Authentication** - Add token-based authentication if needed
- **TLS** - Enable HTTPS for production deployments
- **Rate Limiting** - Configure appropriate rate limits
- **Monitoring** - Use the metrics endpoint for monitoring

## Learn More

- [MCP Specification](https://modelcontextprotocol.io/)
- [Hyperserve Documentation](../../README.md)
- [Go 1.24 os.Root Documentation](https://pkg.go.dev/os#Root)