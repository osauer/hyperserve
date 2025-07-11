# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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