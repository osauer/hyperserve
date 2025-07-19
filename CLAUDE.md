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

### MCP Namespaces for AI Assistants

HyperServe supports **multiple MCP namespaces** to organize tools and resources logically. This is crucial for AI assistants to understand tool organization and use the correct tools for specific tasks.

#### Understanding Namespace Prefixes

When MCP namespaces are configured, tools appear with prefixes:
- `mcp__hyperserve__server_control` - Server operations
- `mcp__app__user_create` - Application functionality  
- `mcp__audio__analyze` - Domain-specific tools
- `mcp__system__config_read` - System administration

#### AI Assistant Best Practices

1. **Tool Discovery**: Always list available tools first to understand the namespace structure:
   ```bash
   curl -X POST http://localhost:8080/mcp \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
   ```

2. **Namespace Recognition**: Group tools by namespace when presenting options to users:
   - **Server Operations** (`mcp__hyperserve__*`): Control server, debug requests
   - **Application Features** (`mcp__app__*`): Business logic, user operations
   - **Domain Tools** (`mcp__audio__*`, `mcp__daw__*`): Specialized functionality

3. **Tool Selection Logic**: Choose tools based on the task domain:
   ```bash
   # For server debugging - use hyperserve namespace
   {"method":"tools/call","params":{"name":"mcp__hyperserve__server_control","arguments":{"action":"get_status"}}}
   
   # For audio processing - use audio namespace
   {"method":"tools/call","params":{"name":"mcp__audio__analyze","arguments":{"file_path":"/path/to/audio.wav"}}}
   ```

4. **User Communication**: When suggesting tools, explain the namespace organization:
   ```
   I found tools organized in these categories:
   - Server operations (hyperserve): server_control, route_inspector
   - Audio processing (audio): analyze, decompose  
   - DAW controls (daw): play, stop, set_bpm
   ```

#### Implementing Namespaces for New Projects

When building MCP-enabled applications, organize tools by logical domains:

```go
// Server with multiple organized namespaces
srv, err := hyperserve.NewServer(
    hyperserve.WithMCPSupport("hyperserve", "1.0.0"),
    
    // Application core functionality
    hyperserve.WithMCPNamespace("app",
        hyperserve.WithNamespaceTools(userTool, postTool, dataTool),
    ),
    
    // Domain-specific tools
    hyperserve.WithMCPNamespace("audio",
        hyperserve.WithNamespaceTools(analyzeTool, processTool),
        hyperserve.WithNamespaceResources(trackListResource),
    ),
    
    // System administration
    hyperserve.WithMCPNamespace("system",
        hyperserve.WithNamespaceTools(configTool, logTool),
    ),
)
```

#### Backward Compatibility

- Tools registered with `srv.RegisterMCPTool()` keep original names (no prefix)
- Existing MCP clients continue to work unchanged
- New namespaced tools coexist with legacy tools