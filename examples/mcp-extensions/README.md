# MCP Extensions Example

This example demonstrates how to build applications on top of hyperserve that expose their functionality through MCP tools and resources.

## Overview

The example creates a simple blog application that exposes:

### MCP Tools
- **manage_posts** - Create, update, delete, and list blog posts
- **search_posts** - Search posts by keyword or tag

### MCP Resources
- **blog://posts/recent** - Latest blog posts
- **blog://stats/overview** - Blog statistics and analytics

## Key Concepts

### 1. Extension Builder Pattern

```go
extension := hyperserve.NewMCPExtension("blog").
    WithDescription("Blog management tools").
    WithTool(myTool).
    WithResource(myResource).
    Build()
```

### 2. Tool Builder Pattern

```go
tool := hyperserve.NewTool("manage_posts").
    WithDescription("Manage blog posts").
    WithParameter("action", "string", "Action to perform", true).
    WithParameter("title", "string", "Post title", false).
    WithExecute(func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
        // Implementation
    }).
    Build()
```

### 3. Resource Builder Pattern

```go
resource := hyperserve.NewResource("blog://posts/recent").
    WithName("Recent Posts").
    WithDescription("Latest blog posts").
    WithRead(func() (interface{}, error) {
        // Return data
    }).
    Build()
```

## Running the Example

```bash
go run main.go
```

## Using with Claude

After configuring Claude Desktop with your server, you can:

1. **Content Management**
   - "Create a blog post about Go generics"
   - "List all blog posts"
   - "Show me posts by Alice"

2. **Search and Discovery**
   - "Find posts tagged with 'golang'"
   - "Search for posts about 'concurrency'"

3. **Analytics**
   - "Show me blog statistics"
   - "How many posts does each author have?"

## Testing with curl

```bash
# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 1
  }'

# Create a blog post
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "manage_posts",
      "arguments": {
        "action": "create",
        "title": "My New Post",
        "content": "This is the content...",
        "author": "Claude",
        "tags": ["ai", "mcp"]
      }
    },
    "id": 2
  }'

# Read blog statistics
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {
      "uri": "blog://stats/overview"
    },
    "id": 3
  }'
```

## Building Your Own Extensions

### Step 1: Define Your Domain

Identify the tools and resources that make sense for your application:
- **Tools**: Actions users can perform
- **Resources**: Data users can access

### Step 2: Create Tools

Tools should:
- Have clear, action-oriented names
- Include comprehensive parameter schemas
- Return structured, predictable responses
- Handle errors gracefully

### Step 3: Create Resources

Resources should:
- Use descriptive URIs (e.g., `app://type/name`)
- Return consistent data structures
- Be read-only (resources don't modify state)
- Cache when appropriate

### Step 4: Package as Extension

Group related tools and resources into extensions:
- Logical grouping (e.g., "blog", "auth", "analytics")
- Shared configuration
- Clear documentation

## Best Practices

1. **Clear Naming** - Use descriptive names for tools and resources
2. **Rich Schemas** - Provide detailed parameter descriptions
3. **Error Handling** - Return helpful error messages
4. **Idempotency** - Make tools idempotent when possible
5. **Security** - Validate all inputs, sanitize outputs
6. **Documentation** - Include examples in descriptions

## Advanced Patterns

### Stateful Tools

```go
type StatefulTool struct {
    db Database
    cache Cache
}

func (t *StatefulTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    // Access t.db, t.cache, etc.
}
```

### Context-Aware Resources

```go
type UserResource struct {
    getCurrentUser func() *User
}

func (r *UserResource) Read() (interface{}, error) {
    user := r.getCurrentUser()
    // Return user-specific data
}
```

### Async Operations

```go
func (t *JobTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    jobID := startBackgroundJob(params)
    return map[string]interface{}{
        "job_id": jobID,
        "status": "started",
        "check_status_with": "job_status tool",
    }, nil
}
```