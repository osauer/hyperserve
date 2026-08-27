# MCP application extensions

This example registers an in-memory blog domain as MCP tools and a resource
template. Read [mcp-basic](../mcp-basic/) first if you only need the transport
shape.

The example contrasts two tool APIs:

- `NewTypedTool` derives input and output schemas from Go types and validates
  tagged arguments before calling the handler;
- `NewTool` gives the application direct control over a schema and a
  `map[string]any` handler.

Prefer typed tools for normal domain operations. Use the builder when a schema
cannot be expressed by the typed generator.

## Register the domain

Each tool is one operation on the store. The extension keeps related tools and
resources together:

```go
ext := mcp.NewExtension("blog").
	WithTool(mcp.NewTypedTool("create_post", "Create a post.", store.Create)).
	WithTool(mcp.NewTypedTool("get_post", "Fetch a post.", store.Get)).
	WithTool(store.searchTool()).
	WithResourceTemplate(postResourceTemplate{store: store}).
	Build()

if err := srv.RegisterMCPExtension(ext); err != nil {
	log.Fatal(err)
}
```

The typed handler uses ordinary Go input and output types:

```go
type CreatePostArgs struct {
	Title  string `json:"title" validate:"required,max=200"`
	Author string `json:"author" validate:"required"`
}

func (b *blog) Create(ctx context.Context, args CreatePostArgs) (Post, error)
```

`blog://posts/{id}` is a resource template rather than a tool because reading a
post has no side effect. Its `Subscribe` method emits invalidations; the client
then reads the resource again for the current value.

## Run it

From the repository root:

```bash
go run ./examples/mcp-extensions
```

From another terminal, list the registered tools using the current Streamable
HTTP request metadata:

```bash
curl -sS -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: tools/list' \
  -d '{
    "jsonrpc":"2.0",
    "method":"tools/list",
    "params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},
    "id":1
  }'
```

For `tools/call`, send the same protocol metadata, set `Mcp-Method` to
`tools/call`, and set `Mcp-Name` to `create_post`. See the
[MCP guide](../../docs/MCP_GUIDE.md) for complete tool calls and resource
subscriptions.

This program has no authentication and stores data only in memory. Keep it on a
development listener; a real service must authenticate `/mcp`, authorize each
operation, and use durable storage where required.
