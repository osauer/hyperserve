# MCP Extensions Example

This example demonstrates how to build applications on top of hyperserve that expose their functionality through MCP tools and resources.

## Overview

The example creates a simple blog application that exposes one tool per
operation. Most tools use `mcp.NewTypedTool`; `search_posts` intentionally
uses the lower-level builder so both shapes are visible.

### MCP Tools
- **create_post** - Create a blog post
- **get_post** - Fetch one blog post by ID
- **list_posts** - List blog posts
- **delete_post** - Delete a blog post
- **search_posts** - Search posts by keyword or tag

## Key Concepts

### 1. Extension Builder Pattern

```go
extension := mcp.NewExtension("blog").
    WithDescription("Blog management tools").
    WithTool(myTool).
    Build()
```

### 2. Typed Tool Pattern

```go
type CreatePostArgs struct {
    Title  string `json:"title" validate:"required,max=200"`
    Author string `json:"author" validate:"required"`
}

tool := mcp.NewTypedTool("create_post", "Create a new blog post.", store.Create)
```

### 3. Builder Tool Pattern

```go
tool := mcp.NewTool("search_posts").
    WithDescription("Search posts").
    WithParameter("query", "string", "Substring matched against title and content", false).
    WithExecute(func(params map[string]any) (any, error) {
        return search(params), nil
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

func (t *StatefulTool) Execute(params map[string]any) (any, error) {
    // Access t.db, t.cache, etc.
}
```

### Context-Aware Resources

```go
type UserResource struct {
    getCurrentUser func() *User
}

func (r *UserResource) Read() (any, error) {
    user := r.getCurrentUser()
    // Return user-specific data
}
```

### Async Operations

```go
func (t *JobTool) Execute(params map[string]any) (any, error) {
    jobID := startBackgroundJob(params)
    return map[string]any{
        "job_id": jobID,
        "status": "started",
        "check_status_with": "job_status tool",
    }, nil
}
```
