# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🤖 IMMEDIATE ACTION FOR AI ASSISTANTS

**When working on a HyperServe project, IMMEDIATELY check for MCP capabilities:**

1. **Check if HyperServe is running** by looking for server startup messages with "MCP ENABLED"
2. **If MCP is enabled**, connect via SSE first:
   ```bash
   # Connect to SSE endpoint (keep this connection open)
   curl -N http://localhost:8080/mcp/sse
   ```
3. **Send MCP requests** with the SSE client ID from the connection event:
   ```bash
   curl -X POST http://localhost:8080/mcp \
     -H "X-SSE-Client-ID: <your-client-id>" \
     -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
   ```
4. **Use the discovered MCP tools** for all subsequent operations instead of standard file/grep/search operations

## Development Guidelines

- **Testing and Documentation**
  * Always test your changes thoroughly
  * When building new features or updating existing ones, update:
    - Examples
    - Documentation
    - Metadata files
  * Ensure comprehensive test coverage for new functionality
  * **Honor good library design practices**

## Model Context Protocol (MCP) Support

HyperServe provides native support for the Model Context Protocol (MCP), enabling AI assistants to connect and interact with the server through standardized tools and resources.

### Enabling MCP

```go
// Basic MCP support (protocol only, no built-in tools/resources)
srv, err := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
)

// MCP with built-in tools and resources
srv, err := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPBuiltinTools(true),      // Enable built-in tools (disabled by default)
    hyperserve.WithMCPBuiltinResources(true),  // Enable built-in resources (disabled by default)
    hyperserve.WithMCPFileToolRoot("/safe/path"), // Set root for file operations
)
```

### Important Notes

- **Built-in tools and resources are disabled by default** for security reasons
- Users must explicitly enable them using `WithMCPBuiltinTools(true)` and `WithMCPBuiltinResources(true)`
- File operations are sandboxed using Go 1.24's `os.Root` when a file tool root is configured

### Custom Tools and Resources

```go
// Register custom tools after server creation
srv.RegisterMCPTool(&MyCustomTool{})
srv.RegisterMCPResource(&MyCustomResource{})
```

### Complete MCP Documentation

For comprehensive MCP information including multiple namespace support, custom tool development, and advanced configuration, see:
- **[MCP Integration Guide](docs/MCP_GUIDE.md)** - Complete guide with examples, namespaces, and best practices