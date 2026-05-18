# MCP Discovery Policy Examples

This example demonstrates different discovery policies for controlling how MCP tools and resources are exposed through discovery endpoints.

## Discovery Policies

1. **DiscoveryCount** - Only shows counts, no tool names
2. **DiscoveryAuthenticated** - Shows full list only with Authorization header
3. **DiscoveryPublic** - Shows all discoverable tools (default)
4. **DiscoveryNone** - Hides all tool information
5. **Custom Filter** - Context-aware filtering based on request

## Running the Example

```bash
go run main.go
```

This starts three servers demonstrating different policies:
- Port 8081: Count-only policy
- Port 8082: Authenticated policy  
- Port 8083: Custom IP-based filter

## Testing

### Server 1: Count-only (port 8081)
```bash
curl http://localhost:8081/.well-known/mcp.json | jq '.capabilities.tools'
# Output: {"supported": true, "count": 2}
```

### Server 2: Authenticated (port 8082)
```bash
# Without auth - only counts
curl http://localhost:8082/.well-known/mcp.json | jq '.capabilities.tools'
# Output: {"supported": true, "count": 2}

# With auth - full list
curl -H "Authorization: Bearer token" http://localhost:8082/.well-known/mcp.json | jq '.capabilities.tools'
# Output: {"supported": true, "count": 2, "available": ["public_info"]}
# Note: secret_operation is hidden because IsDiscoverable() returns false
```

### Server 3: Custom filter (port 8083)
```bash
# From localhost - see non-secret tools
curl http://localhost:8083/.well-known/mcp.json | jq '.capabilities.tools'
# Output: {"supported": true, "count": 3, "available": ["public_info", "admin_tool"]}
```

## Key Concepts

1. **IsDiscoverable()** - Tools can opt out of discovery by implementing this method
2. **Discovery Filter** - Custom logic for context-aware filtering
3. **RBAC Compatible** - Filters can decode JWT tokens from Authorization headers
4. **Default Behavior** - Tools without IsDiscoverable() default to being discoverable

## Security Notes

- Dev tools like `server_control` are automatically hidden in production
- Tools prefixed with `internal_` or `_` are always hidden
- Custom filters can implement complex RBAC logic
- Discovery policies work independently of actual tool access control