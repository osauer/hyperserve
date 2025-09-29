# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Structure

HyperServe is a high-performance HTTP server framework written in Go with native Model Context Protocol (MCP) support. The library files are located at the repository root for easy imports.

## 🤖 IMMEDIATE ACTION FOR AI ASSISTANTS

**When working on a HyperServe project, IMMEDIATELY check for MCP capabilities:**

1. **Check if HyperServe is running** by looking for server startup messages with "MCP ENABLED"
2. **Discover MCP endpoints** (if available):
   - Try `/.well-known/mcp.json` for server discovery
   - Try `/mcp/discover` for transport and capability information
3. **If MCP is enabled**, you have two connection options:

### Option A: Direct HTTP (Recommended for AI Assistants)
```bash
# Send requests directly to the MCP endpoint
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

### Option B: Server-Sent Events (SSE) for Real-time Communication
```bash
# Connect to the SAME endpoint with SSE header (keep this connection open)
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp

# Then send requests with the SSE client ID from the connection event:
curl -X POST http://localhost:8080/mcp \
  -H "X-SSE-Client-ID: <your-client-id>" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

**Important:** Both regular HTTP and SSE use the SAME `/mcp` endpoint. The server automatically handles routing based on headers.

3. **Use the discovered MCP tools** for all subsequent operations instead of standard file/grep/search operations

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

### Discovery Endpoints

HyperServe implements MCP discovery endpoints for automatic configuration:

- **`/.well-known/mcp.json`** - Standard discovery endpoint that returns server capabilities and transport information
- **`/mcp/discover`** - Alternative discovery endpoint with the same information

These endpoints help AI assistants like Claude Code automatically discover:
- Available transports (HTTP, SSE)
- Endpoint URLs
- Server capabilities
- Required headers for each transport type
- **Dynamic tool and resource lists** (based on discovery policy)

### Discovery Policies

HyperServe supports configurable discovery policies to control what information is exposed:

```go
// Only show tool/resource counts (most secure)
srv, _ := server.NewServer(
    server.WithMCPDiscoveryPolicy(server.DiscoveryCount),
)

// Show full list only with Authorization header
srv, _ := server.NewServer(
    server.WithMCPDiscoveryPolicy(server.DiscoveryAuthenticated),
)

// Custom filter for RBAC integration
srv, _ := server.NewServer(
    server.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
        // Decode JWT from Authorization header
        token := r.Header.Get("Authorization")
        if claims, err := validateJWT(token); err == nil {
            return claims.HasPermission(toolName)
        }
        return false
    }),
)
```

**Security Notes:**
- Dev tools (server_control, request_debugger) are hidden in production
- Tools can opt out by implementing `IsDiscoverable() bool`
- Custom filters enable RBAC integration with existing auth systems

### Enabling MCP

```go
// Basic MCP support (protocol only, no built-in tools/resources)
srv, err := server.NewServer(
    server.WithMCPSupport("MyServer", "1.0.0"),
)

// MCP with built-in tools and resources
srv, err := server.NewServer(
    server.WithMCPSupport("MyServer", "1.0.0"),
    server.WithMCPBuiltinTools(true),      // Enable built-in tools (disabled by default)
    server.WithMCPBuiltinResources(true),  // Enable built-in resources (disabled by default)
    server.WithMCPFileToolRoot("/safe/path"), // Set root for file operations
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

## Authentication Implementation

HyperServe includes a production-ready authentication system with comprehensive examples:

### Built-in Authentication Middleware

The `AuthMiddleware` in HyperServe provides:
- Bearer token validation with timing-safe comparison
- Session context injection after successful authentication
- Integration with custom `AuthTokenValidatorFunc`
- Proper HTTP status codes (401, 500)

### Authentication Example

The **[examples/auth](examples/auth/)** directory contains a complete authentication implementation featuring:

1. **Multiple Authentication Methods**:
   - JWT with RS256 signature verification
   - API Keys with per-key rate limiting
   - Basic Auth (development only)

2. **Security Features**:
   - Rate limiting per token to prevent brute force
   - Timing-safe validation using `crypto/subtle`
   - Comprehensive audit logging
   - Environment-specific configurations

3. **RBAC Support**:
   - Role-based access control
   - Fine-grained permissions
   - Permission middleware for route protection

### Quick Start

```go
// Create a token validator function
validator := func(ctx context.Context, token string) (string, bool) {
    // Your validation logic here
    return sessionID, isValid
}

// Create server with authentication
srv, err := server.NewServer(
    server.WithAuthTokenValidator(validator),
    server.WithSecurityHeaders(true),
    server.WithRateLimiting(true),
)

// Apply auth middleware to protected routes
api := srv.Group("/api")
api.Use(srv.AuthMiddleware)
```

### Integration with MCP Discovery

The authentication system integrates seamlessly with MCP discovery policies:

```go
// Use JWT claims for MCP tool filtering
srv, err := server.NewServer(
    server.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
        token := r.Header.Get("Authorization")
        if claims, err := validateJWT(token); err == nil {
            return claims.HasPermission(toolName)
        }
        return false
    }),
)
```

For a complete implementation guide, see the [auth example README](examples/auth/README.md).
