# MCP Discovery Policy Examples

This example demonstrates different discovery policies for controlling how MCP tools and resources are exposed through discovery endpoints.

## Discovery Policies

1. **DiscoveryCount** - Only shows counts, no tool names
2. **DiscoveryAuthenticated** - Shows full list only with Authorization header
3. **DiscoveryPublic** - Shows all discoverable tools (default)
4. **DiscoveryNone** - Hides all tool information
5. **Custom Filter** - Context-aware filtering based on request

## Running the Example

```sh
go run ./examples/mcp-discovery
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
# From localhost - the custom filter deliberately exposes all three tools
curl http://localhost:8083/.well-known/mcp.json | jq '.capabilities.tools'
# Output: {"supported": true, "count": 3, "available": ["admin_tool", "public_info", "secret_operation"]}

# From a non-loopback peer - the custom filter exposes only names containing "public"
# Output: {"supported": true, "count": 3, "available": ["public_info"]}
```

## Key Concepts

1. **IsDiscoverable()** - Tools can opt out of discovery by implementing this method
2. **Discovery Filter** - Custom logic that overrides the default name and `IsDiscoverable` rules
3. **RBAC Compatible** - Filters can decode JWT tokens from Authorization headers
4. **Default Behavior** - Tools without IsDiscoverable() default to being discoverable

## Security Notes

- Default discovery hides underscore-prefixed tools and honors `IsDiscoverable`
- A custom filter replaces those defaults and must reproduce any rules it needs
- Custom filters can implement complex RBAC logic
- Discovery policies change metadata presentation only; they do not authorize tool calls
