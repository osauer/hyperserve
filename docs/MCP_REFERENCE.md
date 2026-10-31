# MCP extension reference

Start with the [integration guide](MCP_GUIDE.md). These examples describe
application extensions; the application supplies access control and lifecycle.

## Custom Extensions

### Typed Tool (recommended)

`mcp.NewTypedTool` wraps a typed Go function as an MCP tool. The framework
derives the JSON Schema from the args struct via reflection — field names
from `json:"…"`, types from Go types, `required`/`oneof`/`min`/`max`/`len`
from `validate:"…"`, descriptions from `mcp:"desc=…"`. Each call decodes
the incoming arguments into the struct, runs the same validator used by
the HTTP binding helpers, then invokes the handler. The return type
drives `outputSchema` on `tools/list` so MCP clients can introspect the
response shape too. Successful results with an output schema include
`structuredContent` and the same JSON in a text content block. Scalars and
arrays are wrapped as `{"result": ...}` in both the wire schema and result;
object outputs are returned directly. A nil result with an output schema is
a tool error; return an empty slice rather than nil for an empty array result.

Prefer **one tool per verb** — narrow args structs let `required` mean
what it says, and named tools like `create_post` / `delete_post` are
easier for an LLM to select than a `manage_posts` tool with an `action`
enum.

```go
type CreatePostArgs struct {
    Title   string   `json:"title"   validate:"required,max=200"`
    Author  string   `json:"author"  validate:"required"`
    Tags    []string `json:"tags,omitempty" validate:"max=10"`
}
type Post struct {
    ID, Title, Author string
    Tags              []string
    CreatedAt         time.Time
}

func (b *Blog) Create(ctx context.Context, args CreatePostArgs) (Post, error) {
    return b.create(args) // args is already decoded and validated.
}

app.RegisterMCPTool(mcp.NewTypedTool(
    "create_post", "Create a new blog post.", blog.Create))
```

Type inference picks both `In` and `Out` off the method value — callers
don't write the generic parameters explicitly. Use `struct{}` on either
side for tools that take no arguments (`List`) or return no payload
(`Delete`); the empty struct also suppresses `outputSchema`.

Supported `validate` verbs map to JSON Schema as:

| Verb                  | Field type     | Schema constraint           |
|-----------------------|----------------|-----------------------------|
| `required`            | any            | appears in `required` list  |
| `oneof=A B C`         | string         | `enum: ["A","B","C"]`       |
| `oneof=1 2 3`         | integer        | `enum: [1,2,3]`             |
| `min=N` / `max=N`     | integer/number | `minimum` / `maximum`       |
| `min=N` / `max=N`     | string         | `minLength` / `maxLength`   |
| `min=N` / `max=N`     | array/slice    | `minItems` / `maxItems`     |
| `len=N`               | string/array   | min and max set to N        |

Numeric `oneof` values retain their JSON number type, including the full
`uint64` range. Invalid or out-of-range numeric options panic when the tool is
constructed. Unknown `validate` rules return an error during argument binding.

Validation failures surface through the JSON-RPC tool-call error with the
same per-field message format produced by `hyperserve.BindJSON`
(`"validation failed: field: rule message; …"`). That format is part of
the wire surface — see [mcp/typed_tool_test.go](../mcp/typed_tool_test.go)
`TestNewTypedTool_ValidationErrorMessageFormat` for the pinning test.
Nested structs are inlined; pointer fields are optional (presence in
`required` is controlled by the tag, not the pointer).

Current limitations include cross-field rules, custom validators, JSON Schema
`$ref` / `$defs`, OpenAPI generation. Hand-author the schema with the
builder below when those matter.

See `examples/mcp-extensions/` for `create_post` / `get_post` /
`list_posts` / `delete_post` (typed) and `search_posts` (builder) side
by side.

### Simple Tool

```go
tool := mcp.NewTool("deploy").
    WithDescription("Deploy application").
    WithParameter("version", "string", "Version to deploy", true).
    WithParameter("environment", "string", "Target environment", true).
    WithExecute(func(params map[string]any) (any, error) {
        version := params["version"].(string)
        env := params["environment"].(string)

        // Your deployment logic here
        return map[string]any{
            "status": "deployed",
            "version": version,
            "environment": env,
        }, nil
    }).
    Build()

app.RegisterMCPTool(tool)
```

### Simple Resource

```go
type userStatsResource struct{}

func (userStatsResource) URI() string         { return "app://stats/users" }
func (userStatsResource) Name() string        { return "User Statistics" }
func (userStatsResource) Description() string { return "Current user statistics" }
func (userStatsResource) MimeType() string    { return "application/json" }
func (userStatsResource) List() ([]string, error) {
    return []string{"app://stats/users"}, nil
}
func (userStatsResource) Read() (any, error) {
    return map[string]any{
        "total_users": getUserCount(),
        "active_today": getActiveUsers(),
        "new_this_week": getNewUsers(),
    }, nil
}

app.RegisterMCPResource(userStatsResource{})
```

### Resource Templates and Subscriptions

Use `mcp.ResourceTemplate` for resource families whose concrete URI is not
known at registration time. `resources/templates/list` exposes the
`uriTemplate`, and `resources/read` resolves a concrete URI through the first
matching template after checking exact static resources.

```go
type quoteResource struct{}

func (quoteResource) URITemplate() string { return "quotes://{symbol}" }
func (quoteResource) Name() string        { return "Quotes" }
func (quoteResource) Description() string { return "Latest quote by symbol" }
func (quoteResource) MimeType() string    { return "application/json" }
func (quoteResource) Match(uri string) (map[string]string, bool) {
    symbol, ok := strings.CutPrefix(uri, "quotes://")
    if !ok || symbol == "" {
        return nil, false
    }
    return map[string]string{"symbol": symbol}, true
}
func (quoteResource) Read(ctx context.Context, uri string, params map[string]string) (any, error) {
    return lookupQuote(ctx, params["symbol"])
}

app.RegisterMCPResourceTemplate(quoteResource{})
```

Templates that also implement `mcp.SubscribableResourceTemplate` enable
current `subscriptions/listen` resource updates. They also support legacy
`resources/subscribe` and `resources/unsubscribe` over stdio or the explicitly
enabled proprietary routed stream.

```go
func (quoteResource) Subscribe(ctx context.Context, uri string, params map[string]string, emit mcp.ResourceEmitter) error {
    stream := quoteBus.Subscribe(params["symbol"])
    defer stream.Close()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-stream.Updates():
            if err := emit.Update(uri); err != nil {
                return err
            }
        }
    }
}
```

`emit.Update(uri)` sends the MCP-standard
`notifications/resources/updated` invalidation signal with only the URI.
Clients call `resources/read` to fetch the latest content. Plain one-shot HTTP
does not support subscriptions because there is no channel for later
notifications.

### Complete Extension

```go
ext := mcp.NewExtension("analytics").
    WithDescription("Analytics tools and data").
    WithTool(
        mcp.NewTool("query_metrics").
            WithParameter("metric", "string", "Metric name", true).
            WithParameter("timeframe", "string", "Time range", false).
            WithExecute(queryMetrics).
            Build(),
    ).
    WithResourceTemplate(metricSeriesTemplate{}).
    WithResource(analyticsSummaryResource{}).
    Build()

app.RegisterMCPExtension(ext)
```

## Namespace Support

HyperServe supports organizing MCP tools and resources into namespaces for better organization and to avoid naming conflicts.

### Registering Tools in Namespaces

```go
err := app.RegisterMCPNamespace("daw",
    mcp.WithNamespaceTools(playTool, stopTool),
)
// playTool is accessible as "mcp__daw__play".

err = app.RegisterMCPNamespace("analytics",
    mcp.WithNamespaceResources(analyticsSummaryResource{}),
    mcp.WithNamespaceResourceTemplates(metricSeriesTemplate{}),
)
// analytics://dashboard/summary is listed and read as
// "mcp__analytics__analytics://dashboard/summary".
// analytics://metrics/{name} is listed and read as
// "mcp__analytics__analytics://metrics/{name}".
```

### Registering Entire Namespaces

```go
// Register a complete namespace with tools and resources
err := app.RegisterMCPNamespace("daw",
    mcp.WithNamespaceTools(
        playTool,
        stopTool,
    ),
    mcp.WithNamespaceResources(
        statusResource{},
        metricsResource{},
    ),
)
```

### Calling Namespaced Tools

When calling tools that are in namespaces, use the full prefixed name:

```bash
# Call a namespaced tool
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/call" \
  -H "Mcp-Name: mcp__daw__calculator" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "mcp__daw__calculator",
      "arguments": {
        "operation": "add",
        "a": 5,
        "b": 3
      },
      "_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}
    },
    "id": 1
  }'
```

### Default Namespace

- Tools/resources registered without a namespace maintain their original names for backward compatibility
- When using namespace methods with an empty namespace, the server name is used as the default namespace

### JSON representation and typed schemas

Typed schemas follow `encoding/json`: `time.Time` is a date-time string,
`json.Number` is a number, `[]byte` is base64 text, and fixed byte arrays are
numeric arrays. A custom JSON
marshaler can emit an arbitrary shape, so its field schema is unconstrained.
Use the explicit tool builder when a custom representation needs a precise
schema. The tools module checks advertised output against actual results with
the official MCP client and a JSON Schema validator; these are development-only
dependencies.
