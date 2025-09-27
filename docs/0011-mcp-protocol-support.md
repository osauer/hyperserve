# ADR-0011: MCP (Model Context Protocol) Support

## Status

ACCEPTED

## Context

AI assistants are becoming increasingly important in software development and operations. The Model Context Protocol (MCP) provides a standardized way for AI assistants to connect to data sources and tools. Hyperserve needs to support MCP to enable AI assistant integrations while maintaining its core principles of simplicity, security, and performance.

### Requirements

1. **Protocol Compliance**: Full JSON-RPC 2.0 and MCP specification compliance
2. **Security**: Secure tool execution with sandboxed file access
3. **Extensibility**: Easy addition of custom tools and resources
4. **Performance**: Minimal overhead for non-MCP requests
5. **Consistency**: Integration with existing hyperserve patterns
6. **Optional**: MCP support should be optional and configurable

### MCP Protocol Overview

MCP (Model Context Protocol) is an open standard that provides:
- **Tools**: Functions that AI assistants can call (e.g., file operations, calculations)
- **Resources**: Data sources that AI assistants can read (e.g., configuration, metrics)
- **JSON-RPC 2.0**: Standard protocol for communication
- **Capability Negotiation**: Clients discover available capabilities

## Decision

We will implement native MCP support in hyperserve with the following architecture:

### 1. Protocol Implementation

- **JSON-RPC 2.0 Engine** (`jsonrpc.go`): Complete JSON-RPC 2.0 implementation
- **MCP Handler** (`mcp.go`): MCP protocol-specific request handling
- **HTTP Transport**: MCP requests via HTTP POST to configurable endpoint

### 2. Tools and Resources Architecture

- **Tool Interface**: Standardized interface for all MCP tools
- **Resource Interface**: Standardized interface for all MCP resources
- **Built-in Tools**: File operations, HTTP requests, calculations
- **Built-in Resources**: Server config, metrics, system information, logs

### 3. Security Model

- **os.Root Integration**: File operations use Go 1.24's secure `os.Root`
- **Sandboxed Access**: File tools restricted to configured root directory
- **Configuration Sanitization**: Sensitive data excluded from config resource
- **Optional Authentication**: Integration with existing auth middleware

### 4. Configuration Integration

- **Functional Options**: `WithMCPSupport(name, version, ...)`, `WithMCPEndpoint()`, etc.
- **ServerOptions Fields**: MCP configuration in main options struct
- **Default Values**: Sensible defaults following hyperserve patterns
- **Environment Variables**: Support for env-based configuration

### 5. Performance Considerations

- **Lazy Initialization**: MCP components only created when enabled
- **Minimal Overhead**: No performance impact when MCP is disabled
- **Efficient Routing**: Direct handler registration, not middleware-based
- **Memory Management**: Proper cleanup and resource management

## Implementation Details

### Core Components

```
hyperserve/
├── jsonrpc.go              # JSON-RPC 2.0 engine
├── mcp.go                  # MCP protocol handler
├── mcp_builtin.go          # Built-in tools & resources implementation
├── mcp_test.go             # MCP protocol tests
├── mcp_tools_test.go       # Tools tests
├── mcp_resources_test.go   # Resources tests
├── mcp_integration_test.go # Integration tests
├── mcp_transport.go        # SSE/stdio transport helpers
└── examples/mcp-*          # Complete example applications
```

### Configuration Schema

```go
type ServerOptions struct {
    // ... existing fields ...
    MCPEnabled             bool     `json:"mcp_enabled,omitempty"`
    MCPEndpoint            string   `json:"mcp_endpoint,omitempty"`
    MCPServerName          string   `json:"mcp_server_name,omitempty"`
    MCPServerVersion       string   `json:"mcp_server_version,omitempty"`
    MCPToolsEnabled        bool     `json:"mcp_tools_enabled,omitempty"`
    MCPResourcesEnabled    bool     `json:"mcp_resources_enabled,omitempty"`
    MCPFileToolRoot        string   `json:"mcp_file_tool_root,omitempty"`
}
```

### Built-in Capabilities

**Tools:**
- `read_file`: Read file contents (sandboxed)
- `list_directory`: List directory contents (sandboxed)
- `http_request`: Make HTTP requests to external services
- `calculator`: Basic mathematical operations

**Resources:**
- `config://server/options`: Server configuration (sanitized)
- `metrics://server/stats`: Performance metrics
- `system://runtime/info`: System and runtime information
- `logs://server/recent`: Recent log entries

**Custom Extensions:**
- Register custom tools via `srv.RegisterMCPTool()`
- Register custom resources via `srv.RegisterMCPResource()`
- Check MCP status with `srv.MCPEnabled()`

### Usage Pattern

```go
// Basic usage
srv, err := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithMCPSupport("my-server", "1.0.0"),
    hyperserve.WithMCPEndpoint("/mcp"),
    hyperserve.WithMCPBuiltinTools(true),
    hyperserve.WithMCPBuiltinResources(true),
    hyperserve.WithMCPFileToolRoot("/safe/directory"),
)

// With custom tools
type MyTool struct{}
func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Custom tool" }
func (t *MyTool) Schema() map[string]interface{} { /* ... */ }
func (t *MyTool) Execute(params map[string]interface{}) (interface{}, error) { /* ... */ }

srv.RegisterMCPTool(&MyTool{})
srv.Run()
```

## Rationale

### Why JSON-RPC 2.0?

- **Standard Protocol**: Well-established, widely supported
- **Bi-directional**: Supports both request/response and notifications
- **Lightweight**: Minimal overhead compared to alternatives
- **Tooling**: Extensive tooling and library support

### Why Built-in Tools/Resources?

- **Immediate Value**: Users get useful functionality out of the box
- **Best Practices**: Demonstrates secure implementation patterns
- **Common Use Cases**: Covers most typical AI assistant needs
- **Extensibility**: Easy to add custom tools following examples

### Why os.Root for File Security?

- **Modern Security**: Leverages Go 1.24's secure file access
- **Automatic Sandboxing**: Prevents path traversal attacks
- **Performance**: Native implementation with minimal overhead
- **Future-Proof**: Aligns with Go's security roadmap

### Why Optional by Default?

- **Backward Compatibility**: Doesn't affect existing users
- **Performance**: Zero overhead when disabled
- **Security**: Reduces attack surface when not needed
- **Simplicity**: Users opt-in to additional complexity

## Consequences

### Positive

1. **AI Integration**: Enables seamless AI assistant integration
2. **Standardization**: Uses established protocols and patterns
3. **Security**: Secure-by-default file operations
4. **Extensibility**: Easy to add custom tools and resources
5. **Performance**: Minimal impact on existing functionality
6. **Documentation**: Comprehensive examples and documentation

### Negative

1. **Complexity**: Adds new concepts and APIs to learn
2. **Maintenance**: Additional code to maintain and test
3. **Dependencies**: Minimal, but adds MCP protocol dependency
4. **Attack Surface**: New endpoints, though secured by design

### Neutral

1. **Code Size**: Approximately 1000 lines of new code
2. **Learning Curve**: New concepts for users unfamiliar with MCP
3. **Testing**: Comprehensive test suite ensures reliability

## Alternatives Considered

### 1. External MCP Server

**Option**: Separate MCP server that communicates with hyperserve
**Rejected**: Adds deployment complexity, latency, and maintenance overhead

### 2. Plugin Architecture

**Option**: MCP support via plugins or extensions
**Rejected**: Contradicts ADR-0009 (single package architecture)

### 3. WebSocket Transport

**Option**: Use WebSocket instead of HTTP for MCP communication
**Rejected**: HTTP POST is simpler and sufficient for current needs

### 4. Middleware-based Implementation

**Option**: Implement MCP as middleware
**Rejected**: Protocol endpoints don't fit middleware pattern well

## Compliance

This ADR aligns with existing architectural decisions:

- **ADR-0001**: No external dependencies beyond `golang.org/x/time`
- **ADR-0002**: Uses functional options pattern for configuration
- **ADR-0003**: Integrates with existing middleware system
- **ADR-0004**: Follows configuration precedence hierarchy
- **ADR-0006**: Leverages Go 1.24 features (os.Root)
- **ADR-0009**: Maintains single package architecture

## Implementation Status

- ✅ **Phase 1**: Core MCP implementation (JSON-RPC, handlers, tools, resources)
- ⏳ **Phase 2**: WebSocket transport (future consideration)
- ⏳ **Phase 3**: Advanced tools (database access, etc.)
- ⏳ **Phase 4**: Tool composition and workflows

## References

- [Model Context Protocol Specification](https://modelcontextprotocol.io/)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [Go 1.24 os.Root Documentation](https://pkg.go.dev/os#Root)
- [MCP Example Implementation](../../examples/mcp/)
