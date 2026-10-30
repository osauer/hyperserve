# MCP Integration Guide

Expose application tools and resources through the same HTTP boundary and
lifecycle as ordinary handlers. Begin with the [MCP example](../examples/mcp-basic/)
or the [stdio example](../examples/mcp-stdio/). Enable only the capabilities
your application needs, and attach authentication to the resolved endpoint:

```go
app.UsePrefix(app.Options().MCPEndpoint, auth.Require(auth.Bearer(verifier)))
```

`verifier` is the application's token verifier. See [authentication](../examples/auth/)
for a provider integration; HyperServe does not configure an identity provider.

## Overview

HyperServe provides native MCP support through three main configurations:

1. **Development** (`MCPDev()`) - Tools for local development with Claude Code
2. **Observability** (`MCPObservability()`) - Read-only operational resources; applications must authenticate access
3. **Custom Extensions** - Your own tools and resources

Canonical construction uses the root package plus `mcp`. Built-in presets and
resources additionally require the explicit `mcp/builtin` import:

```go
import (
    "github.com/osauer/hyperserve/v3"
    "github.com/osauer/hyperserve/v3/mcp"
    _ "github.com/osauer/hyperserve/v3/mcp/builtin"
)

app, err := hyperserve.New(
    hyperserve.WithMCPSupport("service", "1.0.0"),
    hyperserve.WithMCPBuiltinResources(true),
)
```

The root package never imports builtins automatically. `MCPDev` and
`MCPObservability` select modes, but their built-in tools and resources are
registered only when `mcp/builtin` is imported. Neither mode creates an
implicit authorization policy.

HyperServe serves two explicitly separated protocol eras on `/mcp`:

- **MCP 2026-07-28 Streamable HTTP** — stateless POST requests with
  per-request metadata. Finite requests return JSON;
  `subscriptions/listen` returns request-scoped SSE.
- **MCP 2025-11-25 request/response compatibility** — initialize-era HTTP
  requests with either no protocol header or the configured legacy version.
  Protocol sessions, resumability, and the revision's standalone SSE behavior
  are not implemented.

The older `X-SSE-*` routed stream is a deprecated, proprietary HyperServe
compatibility transport. It is disabled by default and new clients must not
use it.

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

`tools/call` and `resources/read` also require `Mcp-Name`.
Tool parameters marked `x-mcp-header` require the matching `Mcp-Param-*`
header. HyperServe rejects missing, duplicate, malformed, or mismatched
metadata. Every MCP POST path has a 4 MiB request-body limit.
`server/discover` reports the
current transport version and capabilities.

### Resource subscriptions

`subscriptions/listen` takes a required JSON-RPC request ID and a required
`notifications` object. Resource subscriptions use the same concrete URIs
implemented by `mcp.SubscribableResourceTemplate`:

```bash
curl -N -X POST http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -H 'Mcp-Method: subscriptions/listen' \
  -d '{
    "jsonrpc":"2.0",
    "method":"subscriptions/listen",
    "params":{
      "notifications":{"resourceSubscriptions":["quotes://AAPL"]},
      "_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}
    },
    "id":"quotes-aapl"
  }'
```

The first event is
`notifications/subscriptions/acknowledged` and names only the matched resource
URIs. Later `notifications/resources/updated` events carry
`io.modelcontextprotocol/subscriptionId` equal to the listen request ID.
Duplicate URIs are coalesced; a request may contain at most 128 URIs.
Unsupported tool, prompt, and resource-list filters are omitted from the
acknowledgement.

Each stream has one writer, a 32-event queue that blocks producers instead of
dropping notifications, 30-second write deadlines, and 30-second SSE-comment
keepalives. It does not emit event IDs, protocol pings, or resumability state.
Closing the response cancels its producers. Natural producer completion and
`(*mcp.Handler).Shutdown` send a final `resultType: complete` response; a
client disconnect does not.

Browser Origins default to same scheme, host, and port as the request. For an
authenticated trusted cross-origin client, install an explicit policy:

```go
hyperserve.WithMCPOriginValidator(func(r *http.Request) bool {
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
| Request-scoped SSE | Supported for `subscriptions/listen` |
| Resource subscription acknowledgement, updates, cancellation, graceful completion | Supported |
| 2025-11-25 request/response fallback | Supported without sessions/resumability |
| Proprietary `X-SSE-*` routed stream | Deprecated; disabled by default |

The current path is exercised against the official
[`github.com/modelcontextprotocol/go-sdk` v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0)
for discovery, `tools/list`, `tools/call`, listen acknowledgement, resource
updates, and cancellation, in addition to HyperServe's protocol and
adversarial tests. The SDK dependency exists only in the separate `tools`
module and is not shipped to library users.

Stdio conformance also covers discovery, structured/text tool results, and
domain errors for the default version and the `2025-06-18` compatibility profile.
Current Streamable HTTP is checked with both initialize-era settings.

### Older stdio clients

Clients that accept `2025-06-18` but reject `2025-11-25`, including the Torok
client used in the local consumer checks, need an explicit compatibility version:

```go
app, err := hyperserve.New(
    hyperserve.WithMCPSupport("my-tools", "1", mcp.OverStdio()),
    hyperserve.WithMCPProtocolVersion("2025-06-18"),
)
```

For a standalone handler, use `handler.SetProtocolVersion("2025-06-18")`.
This changes the initialize-era response; it does not change the current
Streamable HTTP version or add legacy sessions/resumability. HyperServe does
not negotiate down automatically. The [stdio example](../examples/mcp-stdio/)
accepts `-protocol-version 2025-06-18`.

## Legacy HyperServe Routed SSE

See [proprietary routed-SSE compatibility](MCP_LEGACY_SSE.md).

## Development with Claude Code

### Quick Start (Recommended)

HyperServe does not parse command-line flags. If your application owns flags
(as [`examples/mcp-cli`](../examples/mcp-cli/) does), translate them into
`WithMCPSupport` options. To accept HyperServe environment variables, bind them
explicitly when constructing the server:

```go
app, err := hyperserve.New(hyperserve.WithEnvironment())
```

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
- "Show me the server status"
- "Show me all routes"

### Available Tools

**mcp__hyperserve__server_control**
- `get_status` - Get server status

**mcp__hyperserve__route_inspector**
- List all registered routes
- View middleware registration scopes and counts
- Filter routes by pattern

With `include_middleware: true` (the default), the response includes
`middleware_registrations`, a list of `prefix` and `count` pairs for the main
server. `*` is global, an empty prefix covers all paths, and other prefixes
apply at slash boundaries. The route
filter leaves this list intact, including scopes without registered routes.
Health routes run on a separate listener. These registration counts do not
describe a per-request execution trace or identify middleware functions.

**mcp__hyperserve__dev_guide**
- Reference card for the developer toolkit (tools, resources, workflows).

### Security Warning

⚠️ **Never use MCPDev() in production.** It exposes server-introspection
endpoints (server status, route enumeration, middleware layout, and development
logs) that should not be on an internet-facing interface.

## Production Observability

### Setup

The environment form below requires `WithEnvironment()` in the application.

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

Create a local-only SSH tunnel to the remote loopback listener and keep it
running while the MCP client is connected:

```bash
ssh -N -o StrictHostKeyChecking=yes -L 127.0.0.1:18080:127.0.0.1:8080 prod-server
```

Then point an HTTP-capable MCP client at the forwarded URL:

```json
{
  "mcpServers": {
    "prod-monitor": {
      "type": "http",
      "url": "http://127.0.0.1:18080/mcp"
    }
  }
}
```

The tunnel protects the network hop but does not add application
authentication or authorization. The remote endpoint must still enforce its
own access policy, and it should expose `MCPObservability()`, not `MCPDev()`.

## Custom Extensions

See the [extension reference](MCP_REFERENCE.md) for typed tools, resources,
subscriptions and namespaces.

## Application boundaries

Authenticate network requests, authorize capabilities in application code, and
bound request bodies and tool work. Keep sensitive resources disabled unless
specifically required. Discovery visibility never authorizes access. Use the
[production guide](PRODUCTION.md) for deployment decisions.

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

See [proprietary routed-SSE compatibility](MCP_LEGACY_SSE.md).

## Troubleshooting

### MCP Not Working
1. Check logs for "MCP handler initialized"
2. Verify endpoint (default: `/mcp`)
3. Ensure tools/resources are registered before `Run(ctx)` or `RunStdio()`

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
