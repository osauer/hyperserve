# MCP Integration Guide

This guide covers how to use HyperServe's Model Context Protocol (MCP) support for AI-assisted development and production monitoring.

## Overview

HyperServe provides native MCP support through three main configurations:

1. **Development** (`MCPDev()`) - Tools for local development with Claude Code
2. **Observability** (`MCPObservability()`) - Safe monitoring for production
3. **Custom Extensions** - Your own tools and resources

HyperServe serves two explicitly separated protocol eras on `/mcp`:

- **MCP 2026-07-28 Streamable HTTP** — stateless POST requests with
  per-request metadata. Ordinary requests currently return JSON.
- **MCP 2025-11-25 request/response compatibility** — initialize-era HTTP
  requests with either no protocol header or the configured legacy version.
  Protocol sessions, resumability, and the revision's standalone SSE behavior
  are not implemented.

The older `X-SSE-*` routed stream is a proprietary HyperServe compatibility
transport. It is not MCP Streamable HTTP and new clients must not use it.

## Streamable HTTP (2026-07-28)

Every request is a new POST with both supported response media types and
mirrored protocol metadata:

```bash
curl -X POST http://localhost:8080/mcp \
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

`tools/call`, `resources/read`, and `prompts/get` also require `Mcp-Name`.
Tool parameters marked `x-mcp-header` require the matching `Mcp-Param-*`
header. HyperServe rejects missing, duplicate, malformed, or mismatched
metadata and bounds request bodies to 4 MiB. `server/discover` reports the
current transport version and capabilities.

Browser Origins default to same scheme, host, and port as the request. For an
authenticated trusted cross-origin client, install an explicit policy:

```go
server.WithMCPOriginValidator(func(r *http.Request) bool {
    return r.Header.Get("Origin") == "https://trusted.example"
})
```

This callback is an Origin policy, not authentication.

### Conformance status

| Surface | Status |
|---|---|
| 2026-07-28 JSON request/response, `server/discover`, tools and resources | Supported |
| Required standard and annotated tool-parameter headers | Validated |
| Accepted notifications | `202 Accepted`, empty body |
| Request-scoped SSE responses | Not yet implemented; issue #74 |
| `subscriptions/listen` | Not yet implemented; issue #74 |
| 2025-11-25 request/response fallback | Supported without sessions/resumability |
| Proprietary `X-SSE-*` routed stream | Legacy compatibility only |

The current path is exercised against the official
[`github.com/modelcontextprotocol/go-sdk` v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0)
for discovery, `tools/list`, and `tools/call`, in addition to HyperServe's
protocol and adversarial tests.

## Legacy HyperServe Routed SSE

This section documents the existing proprietary compatibility mode so it
cannot be confused with standards-compliant Streamable HTTP.

### How the compatibility stream works

1. **Connect with SSE**: send a GET with `Accept: text/event-stream`.
2. **Capture the connection event**: the server returns a `clientId` *and* a
   `bindingToken`. The token is the per-stream capability — knowing the
   `clientId` alone is not enough.
3. **Send routed requests**: POST to the same endpoint with **both**
   `X-SSE-Client-ID` and `X-SSE-Binding` set. The server constant-time
   compares the binding token before queuing the request; a missing or
   wrong token returns 403 indistinguishably from "no such client".
4. **Receive responses**: replies are delivered as events on the open
   SSE stream, not in the POST's HTTP response body.

### Example Usage

```bash
# 1. Connect to SSE (keep this connection open)
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp

# Response:
# event: connection
# data: {"clientId":"sse-…","bindingToken":"…32-byte hex…"}

# 2. Send routed requests with BOTH headers. Missing X-SSE-Binding → 403.
curl -X POST http://localhost:8080/mcp \
  -H "X-SSE-Client-ID: sse-..." \
  -H "X-SSE-Binding: <bindingToken from the connection event>" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# 3. Response arrives on the SSE connection from step 1.
```

### Compatibility behavior

- **Real-time Updates**: Receive notifications and async responses instantly
- **Bidirectional**: Send requests while maintaining an open connection
- **Automatic Keepalive**: Built-in ping/pong every 30 seconds
- **Single Endpoint**: No need to configure separate SSE paths

- **Use Streamable HTTP** for MCP clients and ordinary request/response work.
- **Use the legacy stream only** for an existing HyperServe-specific consumer
  while migrating it. It uses non-standard events and headers.

## Development with Claude Code

### Quick Start (Recommended)

HyperServe does not parse command-line flags. If your application owns flags
(as [`examples/mcp-cli`](../examples/mcp-cli/) does), translate them into
`WithMCPSupport` options. Environment variables are loaded directly:

```bash
# Using flags
./myapp --mcp-dev

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_DEV=true ./myapp
```

### Claude Code Configuration (HTTP)

```json
{
  "mcpServers": {
    "myapp-local": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

### Claude Desktop Configuration (STDIO)

1. Build your application:
```bash
go build -o myapp
```

2. Configure Claude Desktop:
```json
{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/myapp",
      "args": ["--mcp-dev", "--mcp-transport=stdio"],
      "cwd": "/path/to/project"
    }
  }
}
```

3. Start developing with Claude:
- "Set log level to DEBUG"
- "Show me all routes"

### Available Tools

**mcp__hyperserve__server_control**
- `set_log_level` - Change log level (DEBUG, INFO, WARN, ERROR)
- `get_status` - Get server status

**mcp__hyperserve__route_inspector**
- List all registered routes
- View middleware chains
- Filter routes by pattern

**mcp__hyperserve__dev_guide**
- Reference card for the developer toolkit (tools, resources, workflows).

### Security Warning

⚠️ **Never use MCPDev() in production.** It exposes server-introspection
endpoints (log-level switching, route enumeration) that should not be on
an internet-facing interface.

## Production Observability

### Setup

```bash
# Using flags
./myapp --mcp-observability

# Using environment variables
HS_MCP_ENABLED=true HS_MCP_OBSERVABILITY=true ./myapp
```

### Available Resources

**config://server/current**
- Server version and build info
- Network configuration
- Feature flags
- No secrets or sensitive paths

**health://server/status**
- Liveness and readiness
- Uptime
- Request metrics
- Average response time

**logs://server/recent**
- Recent log entries (default: last 100)
- Structured log format
- Circular buffer for memory efficiency

### Remote Access

For production monitoring via Claude:

```json
{
  "mcpServers": {
    "prod-monitor": {
      "command": "ssh",
      "args": [
        "-o", "StrictHostKeyChecking=yes",
        "prod-server",
        "curl", "-s", "http://localhost:8080/mcp"
      ]
    }
  }
}
```

## Custom Extensions

### Typed Tool (recommended)

`mcp.NewTypedTool` wraps a typed Go function as an MCP tool. The framework
derives the JSON Schema from the args struct via reflection — field names
from `json:"…"`, types from Go types, `required`/`oneof`/`min`/`max`/`len`
from `validate:"…"`, descriptions from `mcp:"desc=…"`. Each call decodes
the incoming arguments into the struct, runs the same validator used by
the HTTP binding helpers, then invokes the handler. The return type
drives `outputSchema` on `tools/list` so MCP clients can introspect the
response shape too.

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

srv.RegisterMCPTool(mcp.NewTypedTool(
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

Validation failures surface through the JSON-RPC tool-call error with the
same per-field message format produced by `server.BindJSON`
(`"validation failed: field: rule message; …"`). That format is part of
the wire surface — see [pkg/mcp/typed_tool_test.go](../pkg/mcp/typed_tool_test.go)
`TestNewTypedTool_ValidationErrorMessageFormat` for the pinning test.
Nested structs are inlined; pointer fields are optional (presence in
`required` is controlled by the tag, not the pointer).

Out of scope for v1: cross-field rules, custom validators, JSON Schema
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

srv.RegisterMCPTool(tool)
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

srv.RegisterMCPResource(userStatsResource{})
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

srv.RegisterMCPResourceTemplate(quoteResource{})
```

Templates that also implement `mcp.SubscribableResourceTemplate` enable the
legacy `resources/subscribe` and `resources/unsubscribe` methods over the
proprietary routed stream or stdio. Current Streamable HTTP will expose these
updates through `subscriptions/listen`; that remaining work is tracked in
issue #74.

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

srv.RegisterMCPExtension(ext)
```

## Namespace Support

HyperServe supports organizing MCP tools and resources into namespaces for better organization and to avoid naming conflicts.

### Registering Tools in Namespaces

```go
err := srv.RegisterMCPNamespace("daw",
    mcp.WithNamespaceTools(playTool, stopTool),
)
// playTool is accessible as "mcp__daw__play".

err = srv.RegisterMCPNamespace("analytics",
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
err := srv.RegisterMCPNamespace("daw",
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

## Best Practices

### 1. Security First
- Use `MCPObservability()` for production
- Never expose `MCPDev()` to networks
- Sanitize all data in resources
- Validate tool parameters

### 2. Clear Naming
```go
// Good
tool := NewTool("create_user")
resource := NewResource("users://active/list")

// Bad
tool := NewTool("do_thing")
resource := NewResource("data://stuff")
```

### 3. Error Handling
```go
WithExecute(func(params map[string]any) (any, error) {
    name, ok := params["name"].(string)
    if !ok {
        return nil, fmt.Errorf("name parameter required")
    }
    
    if err := validateName(name); err != nil {
        return nil, fmt.Errorf("invalid name: %w", err)
    }
    
    // ... rest of logic
})
```

Return `mcp.ToolError("message")` for domain failures that should be
successful MCP `tools/call` responses with `isError: true`. Keep returning
ordinary errors for protocol, validation, decode, or unexpected failures:

```go
WithExecute(func(params map[string]any) (any, error) {
    if gatewayDown() {
        return nil, mcp.ToolError("gateway unavailable")
    }
    return callGateway(params)
})
```

### 4. Resource Caching
Resources are live by default. Expensive static resources can opt into bounded
caching by implementing `mcp.CacheableResource`; dynamic observability and
template reads should normally stay uncached.

### 5. Documentation
Always provide clear descriptions:
```go
NewTool("backup_database").
    WithDescription("Create a database backup with optional encryption").
    WithParameter("encrypt", "boolean", "Enable encryption (default: true)", false).
    WithParameter("location", "string", "Backup location (s3|local)", false)
```

## Testing MCP Endpoints

### Using curl

```bash
# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/list" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "params": {"_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}},
    "id": 1
  }'

# Execute a tool
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/call" \
  -H "Mcp-Name: mcp__hyperserve__server_control" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "mcp__hyperserve__server_control",
      "arguments": {
        "action": "get_status"
      },
      "_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}
    },
    "id": 2
  }'

# Read a resource
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: resources/read" \
  -H "Mcp-Name: health://server/status" \
  -d '{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {
      "uri": "health://server/status",
      "_meta": {"io.modelcontextprotocol/protocolVersion": "2026-07-28"}
    },
    "id": 3
  }'

# List resource templates
curl -X POST http://localhost:8080/mcp \
  -H "Accept: application/json, text/event-stream" \
  -H "Content-Type: application/json" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: resources/templates/list" \
  -d '{"jsonrpc":"2.0","method":"resources/templates/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"id":4}'
```

## Legacy HyperServe Routed SSE Reference

This proprietary compatibility stream enables:
- Real-time server-to-client notifications
- Lower latency for interactive tools
- Better support for streaming responses

It is not MCP 2026-07-28 Streamable HTTP: responses are delivered on a
separate long-lived GET stream, and it does not implement request-scoped SSE
or `subscriptions/listen`.

### Compatibility endpoints

When MCP is enabled, HyperServe automatically provides:
- `/mcp` - Standard HTTP endpoint for JSON-RPC POST requests
- `/mcp` - legacy stream only for a GET sending `Accept: text/event-stream`

### Using SSE from JavaScript

```javascript
// Connect to the unified MCP endpoint as an SSE stream.
const eventSource = new EventSource('/mcp');
let clientId = null;
let bindingToken = null;

// Capture the per-stream capability emitted in the connection event.
// Both fields are required for any subsequent routed POST.
eventSource.addEventListener('connection', (e) => {
    const data = JSON.parse(e.data);
    clientId = data.clientId;
    bindingToken = data.bindingToken;
    console.log('Connected:', clientId);
});

// Handle responses
eventSource.addEventListener('message', (e) => {
    const response = JSON.parse(e.data);
    console.log('Response:', response);
});

// Send routed requests. Both headers MUST be set — the binding token is
// the capability, not the client ID. A wrong/missing token returns 403.
async function callMethod(method, params) {
    const response = await fetch('/mcp', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-SSE-Client-ID': clientId,
            'X-SSE-Binding': bindingToken
        },
        body: JSON.stringify({
            jsonrpc: '2.0',
            method: method,
            params: params,
            id: Date.now()
        })
    });
    // Response comes via SSE, not HTTP body
}
```

### SSE Features

- Automatic reconnection on disconnect
- Keepalive pings every 30 seconds
- Buffered message delivery
- Thread-safe connection management
- Proper MCP lifecycle support
- Resource subscription notifications for `SubscribableResourceTemplate`
  values registered on the handler

## Troubleshooting

### MCP Not Working
1. Check logs for "MCP handler initialized"
2. Verify endpoint (default: `/mcp`)
3. Ensure tools/resources are registered before `Run()`

### Claude Desktop Connection Issues
1. Check `claude_desktop_config.json` syntax
2. Verify command path is absolute
3. Check server logs for connection attempts
4. Try HTTP transport first, then STDIO

### Performance Issues
1. Resources are live unless they implement `mcp.CacheableResource`
2. Use pagination for large datasets
3. Tools run with 30s timeout
4. Monitor MCP metrics via observability resources

### Subscription Backpressure
SSE subscription updates share the per-client bounded event queue. If the
client is gone or the queue is full, `ResourceEmitter.Update` returns an
error; subscription implementations should coalesce, drop, or stop on that
error. Stdio notifications are serialized with responses and may block on the
writer.
