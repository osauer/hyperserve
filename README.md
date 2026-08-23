# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe keeps Go's standard `net/http` handler model and adds the service
boundary most applications otherwise rebuild: browser security headers, typed
input, graceful shutdown, health checks, live updates, WebSockets, and optional
Model Context Protocol (MCP).

Use it when you want those pieces to work together without adopting a different
router or handler abstraction. If routes and JSON are all you need, use
`net/http` directly.

## Quick start

HyperServe requires Go 1.27.

```sh
go get github.com/osauer/hyperserve@latest
```

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/osauer/hyperserve/pkg/server"
)

func main() {
    srv, err := server.NewServer()
    if err != nil {
        log.Fatal(err)
    }

    // Give every web route a browser-hardening header baseline.
    srv.AddMiddlewareStack("/", server.SecureWeb(srv.Options))

    srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })

    // Run handles process signals and drains active requests before stopping.
    if err := srv.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Run it and open `http://localhost:8080`:

```sh
go run .
```

`NewServer` also installs request logging, metrics, and panic recovery. Routes
remain `http.ServeMux` patterns, and handlers remain `http.Handler` values, so
existing Go middleware and handlers still fit.

## What it removes from your application

- **Browser policy in one line.** `SecureWeb` supplies a Content Security
  Policy and defensive browser headers without scattering them across handlers.
- **Typed HTTP boundaries.** Decode and validate requests before application
  logic runs, with safe client and server error responses.
- **Lifecycle plumbing.** Signals, request draining, readiness, and deferred
  startup use one server lifecycle.
- **The right kind of live connection.** Send one-way updates with Server-Sent
  Events (SSE), or use WebSockets when both sides need to talk.
- **An optional AI boundary.** Add an MCP endpoint without replacing the HTTP
  or WebSocket parts of the service.

## Choose how your app communicates

Start with the simplest connection that matches the job:

| Your application needs | Use | Why |
| --- | --- | --- |
| One request and one response | HTTP | The ordinary choice for pages, forms, and JSON APIs |
| The server keeps sending updates | SSE | A browser can listen over normal HTTP and reconnect automatically |
| Either side can send at any time | WebSocket | One long-lived, two-way connection |
| AI clients discover tools and resources | MCP | A standard interface for exposing application capabilities |

### Harden browser responses with one line

```go
srv.AddMiddlewareStack("/", server.SecureWeb(srv.Options))
```

`SecureWeb` adds a Content Security Policy, framing and content-sniffing
protections, referrer and permissions policies, and cross-origin isolation
headers. It also applies configured CORS policy and emits HSTS when HyperServe
itself is serving TLS.

This is a browser-security baseline, not a complete security model. The
application still owns TLS termination, authentication, authorization, session
policy, and any CSP changes required by its frontend. Use `SecureAPI` when an
API route should combine an application-provided bearer-token validator with
rate limiting. See the [production guide](./docs/PRODUCTION.md) for the full
boundary.

### Turn JSON requests into Go functions

`JSONHandler` moves decoding, validation, and safe error mapping to the HTTP
edge. The callback receives a validated Go value and returns a Go value:

```go
type CreateUser struct {
    Email string `json:"email" validate:"required,email"`
}

type User struct {
    Email string `json:"email"`
}

srv.POST("/users", server.JSONHandler(
    func(_ context.Context, in CreateUser) (User, error) {
        return User{Email: strings.ToLower(in.Email)}, nil
    },
))
```

Malformed or invalid input produces a structured `400`. Unexpected callback
errors produce a generic `500` without exposing internal details. Use
`BindJSON`, `BindQuery`, `BindForm`, and `Validate` directly when a handler needs
custom headers, streaming, or a different response envelope. The
[`binding` example](./examples/binding/) shows both approaches.

### Send live updates with SSE

Server-Sent Events are the simple option when the server needs to keep sending
updates and the browser only needs to listen: progress, dashboard values, logs,
or notifications. SSE stays on HTTP, works with the browser's `EventSource`
API, and does not require a WebSocket protocol or reconnect loop.

HyperServe formats event names and string, byte, or JSON data correctly:

```go
msg := server.NewSSEMessage(map[string]any{
    "progress": 75,
    "status":   "indexing",
})
msg.Event = "progress"

fmt.Fprint(w, msg)
flusher.Flush() // Make this update visible without closing the response.
```

A complete endpoint must set the SSE response headers, stop when the request
context is cancelled, and choose server and proxy timeouts suitable for a
long-lived response. See the runnable [HTMX + SSE example](./examples/htmx-stream/).

### Use WebSockets when communication is two-way

WebSockets fit chat, collaborative interfaces, device control, and other cases
where the client and server must both send whenever they have something new.
HyperServe implements RFC 6455 for server upgrades and outbound Go clients.

```go
upgrader := srv.WebSocketUpgrader()
upgrader.MaxMessageSize = 512 << 10

srv.GET("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    messageType, payload, err := conn.Read(r.Context())
    if err != nil {
        return
    }
    _ = conn.Write(r.Context(), messageType, payload)
})
```

Server upgrades reject cross-origin browser requests by default. Server and
client connections also enforce a 1 MiB message limit unless configured
otherwise. The outbound `websocket.Dial` API supports caller-owned HTTP clients,
headers, subprotocols, TLS verification, and bounded redirects; reconnect policy
remains with the application. See the [WebSocket guide](./docs/WEBSOCKET_GUIDE.md)
and [browser echo example](./examples/websocket-demo/).

### Add an AI-facing interface only when you need one

MCP lets an AI client discover and call tools or read resources exposed by an
application. HyperServe can mount that interface beside ordinary HTTP routes:

```go
srv, err := server.NewServer(
    server.WithMCPSupport("payments", "1.0.0"),
)
```

MCP is opt-in. Demonstration tools and resources are also off by default, and
the application must put its own authorization middleware in front of `/mcp`.
The endpoint uses current MCP Streamable HTTP; request/response compatibility
for older clients remains available. Transport versions, subscriptions,
discovery, limits, and the deprecated routed-SSE migration path belong in the
[MCP guide](./docs/MCP_GUIDE.md).

## Operate the service deliberately

`Run` handles SIGINT, SIGTERM, and SIGQUIT. If the application already owns its
lifecycle, `RunContext(appCtx)` treats context cancellation as a normal request
to drain and stop.

Add `WithHealthServer()` for separate liveness and readiness endpoints. Use
`WithDeferredInit(...)` when the process should be live while dependencies are
still starting, but not ready to receive application traffic. The
[`deferred-init` example](./examples/deferred-init/) shows the complete flow.

Configuration is explicit. `NewServer()` uses deterministic defaults and does
not read a file or the process environment. Opt into only the authorities the
application intends to accept; later options win:

```go
srv, err := server.NewServer(
    server.WithConfigFile(configPath), // Application-chosen JSON file.
    server.WithEnvironment(),          // Enables supported HS_ variables.
    server.WithAddr("127.0.0.1:8080"),  // Keep this invariant after deployment input.
)
```

Use `DefaultServerOptions`, modify the returned value, and pass it through
`WithOptions` when an embedding application wants one reviewed configuration
snapshot. See the [configuration example](./examples/configuration/) for the
precedence rules.

## Packages and stability

The name was inspired by hyperHTML; the design follows the same preference for
a small, understandable core.

The runtime module has one external dependency, `golang.org/x/time`, for rate
limiting. HyperServe's WebSocket, JSON-RPC, and MCP implementations live in this
repository. That reduces dependency assembly for applications, while making
HyperServe responsible for maintaining that protocol code.

| Import path | Purpose |
| --- | --- |
| `github.com/osauer/hyperserve/pkg/server` | HTTP server, middleware, lifecycle, and MCP wiring |
| `github.com/osauer/hyperserve/pkg/websocket` | WebSocket upgrader, connection, and outbound dialer |
| `github.com/osauer/hyperserve/pkg/mcp` | MCP handler, transports, discovery, and namespaces |
| `github.com/osauer/hyperserve/pkg/mcp/builtin` | Opt-in demonstration tools and resources |
| `github.com/osauer/hyperserve/pkg/jsonrpc` | Standalone JSON-RPC 2.0 engine |

The `server`, `websocket`, `mcp`, and `jsonrpc` package APIs follow semantic
versioning on the v1 module line. Generated layouts, examples, commands, and
the opt-in builtin demonstrations are maintained and tested but are not stable
import surfaces. See [API stability](./docs/API_STABILITY.md) for the full
compatibility and deprecation policy.

## Generate a service

`hyperserve-init` creates a Go module, server entry point, and tests:

```sh
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

The generated project omits MCP because the initializer cannot choose the
application's authorization policy. Pass `--with-mcp` only when the service
will protect `/mcp` itself. `--local-replace` points a generated project at a
local HyperServe checkout.

## Go deeper

- [Examples](./examples/) — runnable programs grouped by application task
- [Production guide](./docs/PRODUCTION.md) — proxies, TLS, health, shutdown, and security boundaries
- [MCP guide](./docs/MCP_GUIDE.md) — tools, resources, transports, discovery, and authorization boundaries
- [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) — server and client connection behavior
- [Architecture](./ARCHITECTURE.md) — package and lifecycle design
- [Go reference](https://pkg.go.dev/github.com/osauer/hyperserve) — exported API documentation
- [Project website](https://osauer.dev/hyperserve/) — concise overview and installation
- [Contributing](./CONTRIBUTING.md) — local workflow and pull requests
- [Security policy](./SECURITY.md) — supported releases and private reporting

Repository checks:

```sh
make check
make test-race
make fuzz-smoke
```

MIT — see [LICENSE](./LICENSE). Bugs and usage questions belong in
[GitHub Issues](https://github.com/osauer/hyperserve/issues).
