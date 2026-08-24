# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve/v2.svg)](https://pkg.go.dev/github.com/osauer/hyperserve/v2)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a Go server library built on `net/http`. It leaves routing and
handlers alone, then fills in the work that tends to collect around them:
request binding and validation, middleware, health and readiness, shutdown,
templates and static files, Server-Sent Events, WebSockets, and optional Model
Context Protocol (MCP).

Most Go services start comfortably with a mux and a few handlers. Later they
need probes, signal handling, request limits, validation, streaming, or another
listener. You can wire those pieces separately. HyperServe is for applications
that would rather keep them in one server with one configuration and shutdown
path, without taking on a framework-specific router or request context.

For a small service with a handful of routes, plain `net/http` is usually the
better choice. HyperServe also does not provide an ORM, a frontend framework,
browser sessions, or application authorization. Its `auth` package establishes
an identity; the application still decides what that identity may do.

## Quick start

HyperServe requires Go 1.27.

```sh
go get github.com/osauer/hyperserve/v2@latest
```

Public package APIs on the v2 module line follow
[semantic versioning](./docs/API_STABILITY.md).
Applications upgrading from v1 should start with the
[v2 migration guide](./docs/MIGRATING_V2.md).
See the [examples](./examples/) for runnable variants and the
[production guide](./docs/PRODUCTION.md) before deployment.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"

    "github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    srv, err := server.NewServer()
    if err != nil {
        log.Fatal(err)
    }

    // Routes use Go's method-aware ServeMux patterns, and handlers remain
    // ordinary net/http functions—there is no framework-specific context.
    srv.GET("/hello/{name}", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello, %s!\n", r.PathValue("name"))
    })

    // The application owns cancellation; HyperServe owns orderly cleanup of
    // the listeners and workers it starts.
    if err := srv.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

Run the server:

```sh
go run .
```

Then, from another terminal:

```sh
curl -sS http://localhost:8080/hello/Ada
```

Expected response:

```text
Hello, Ada!
```

That is the basic shape: keep standard `net/http` handlers while HyperServe
applies request logging, request metrics, and panic recovery. Cancelling the
context stops the listeners, workers, filesystem roots, and shutdown hooks
owned by that server.

For typed request bodies, validation, and JSON responses, continue with the
[binding example](./examples/binding/). Those helpers are optional; they do not
replace Go's handler or request types.

## Why use it?

HyperServe does not introduce an application model. Routes use Go's
method-aware `ServeMux` patterns, handlers are `http.Handler` values, and
request cancellation travels through `context.Context`. Existing handlers and
`httptest` continue to work.

It also keeps the pieces it starts on the same lifecycle. The HTTP server,
health listener, shutdown hooks, internal workers, filesystem roots, and MCP
streams are closed through the same shutdown path. Startup failures run that
cleanup too.

Configuration is explicit. A bare `NewServer()` does not read a configuration
file, environment variables, or specially named asset directories. Applications
opt into those sources and decide their precedence.

There is still an ownership line. HyperServe handles transport and server
mechanics, and can establish a request identity. The application chooses the
identity provider and credential policy, then owns authorization, data access,
browser sessions, WebSocket reconnection, and deployment topology.

## Routes, pages, and assets

Existing handlers can be registered directly. The assembled server is also
available as an `http.Handler`:

```go
// No adapter is needed for an existing handler.
srv.Handle("/admin/", existingHandler)

// This includes the mux and HyperServe middleware. It can be wrapped, mounted
// in another server, or passed directly to httptest.
handler := srv.Handler()
```

`GET`, `POST`, `PUT`, `PATCH`, and the other method helpers use standard
`ServeMux` patterns, including path values. `Handle` and `HandleFunc` remain
available when one handler covers several methods.

`JSONHandler` is the short path for typed JSON endpoints. `BindJSON`,
`BindQuery`, `BindForm`, and `Validate` are available when a handler needs
custom headers, streaming, or its own response shape. See the
[binding example](./examples/binding/).

For HTML applications, HyperServe can render `html/template` files and serve
static assets. Disk roots are off until the application selects them with
`WithTemplateDir` or `WithStaticDir`. Static files are confined with
`os.Root`; `HandleStatic` returns an error and leaves the route closed
if the root cannot be opened. Embedded assets can be served through an ordinary
handler.

## SSE or WebSockets?

Use HTTP for normal request/response work. For a long-lived connection, the
direction of communication usually decides:

| Need | Use |
| --- | --- |
| The server pushes progress, notifications, logs, or dashboard updates | Server-Sent Events (SSE) |
| The client and server can both send at any time | WebSocket |

`SSEMessage` formats event names and string, byte, or JSON data:

```go
// HyperServe formats the event. The application still chooses authorization,
// event cadence, buffering, and resume behavior.
msg := server.NewSSEMessage(map[string]any{
    "progress": 75,
    "status":   "indexing",
})
msg.Event = "progress"

fmt.Fprint(w, msg)
flusher.Flush() // Send this event without closing the response.
```

An SSE handler must stop when the request context is cancelled and use server
and proxy timeouts that permit a long response. The
[HTMX + SSE example](./examples/htmx-stream/) includes the full loop.

For WebSockets, the server-owned upgrader applies the default same-origin check
and records the upgrade with the server's request metrics:

```go
// Prefer the server-owned upgrader to a standalone websocket.Upgrader: it
// keeps the same-origin default and records upgrades with server metrics.
upgrader := srv.WebSocketUpgrader()
```

Server and outbound client connections have a 1 MiB message limit unless the
application changes it. `websocket.Dial` accepts caller-owned HTTP clients and
supports headers, subprotocols, TLS verification, and bounded redirects.
Reconnect behavior is left to the application. See the
[WebSocket guide](./docs/WEBSOCKET_GUIDE.md) and
[browser echo example](./examples/websocket-demo/).

## Startup, shutdown, and configuration

`Run(ctx)` blocks until the context is cancelled or the server exits. The
application owns process signals because a library cannot know whether one
signal should stop one server, several servers, or a larger application.
`RunStdio()` is the explicit entry point for MCP over standard input/output.
`Shutdown(ctx)` is available when another component coordinates the deadline.

`WithHealthServer` puts health, readiness, and liveness on a separate
listener. `WithDeferredInit` keeps readiness false while a database, cache, or
other dependency starts.

Configuration options are applied from left to right:

```go
// Configuration sources are ignored unless the application opts into them.
// Options run in order, so the final address cannot be replaced by the file
// or environment.
srv, err := server.NewServer(
    server.WithConfigFile(configPath),
    server.WithEnvironment(),
    server.WithAddr("127.0.0.1:8080"),
)
```

Use `DefaultOptions` with `WithOptions` when the embedding application
wants to bind one reviewed configuration snapshot. The
[configuration example](./examples/configuration/) covers the precedence rules.

## Security

Browser security headers are opt-in:

```go
// Browser headers apply to this route prefix. TLS, sessions, and authorization
// remain separate application decisions.
srv.UsePrefix("/", server.SecureWeb(srv.Options()))
```

Authentication composes from small, named pieces:

```go
verifier := auth.TokenVerifierFunc(verifyToken)
bearerIdentity := auth.Bearer(verifier)
requireIdentity := auth.Require(bearerIdentity)
srv.UsePrefix("/api", requireIdentity, server.RateLimitMiddleware(srv))
```

`SecureWeb` emits a Content Security Policy and other defensive browser
headers, applies configured CORS policy, and emits HSTS when HyperServe serves
TLS. `auth.Require` validates credentials and stores an issuer/subject
principal on the request. It does not define users, roles, sessions, login
redirects, or resource authorization. The
[federated authentication example](./examples/auth/) connects that seam to an
OpenID Connect provider without adding OIDC dependencies to the runtime module.

The [production guide](./docs/PRODUCTION.md) documents TLS, proxies, health
endpoints, filesystem roots, and the remaining application responsibilities.

## MCP

MCP is optional and does not change the HTTP or WebSocket APIs:

```go
// MCP shares the HTTP server's middleware and shutdown path. Enabling the
// endpoint does not enable demonstration tools or resources.
srv, err := server.NewServer(
    server.WithMCPSupport("payments", "1.0.0"),
)
```

Applications must add authorization middleware in front of `/mcp`. HyperServe
supports Streamable HTTP, request-scoped SSE subscriptions, stdio, typed tools,
resources, namespaces, and discovery. Transport versions, limits, and
authorization boundaries are in the [MCP guide](./docs/MCP_GUIDE.md).

## Packages and trade-offs

| Import path | Purpose |
| --- | --- |
| `github.com/osauer/hyperserve/v2/pkg/server` | HTTP server, middleware, lifecycle, pages, and MCP wiring |
| `github.com/osauer/hyperserve/v2/pkg/auth` | Provider-neutral request authentication and stable principals |
| `github.com/osauer/hyperserve/v2/pkg/websocket` | WebSocket upgrader, connection, and outbound dialer |
| `github.com/osauer/hyperserve/v2/pkg/mcp` | MCP handler, transports, discovery, tools, and resources |
| `github.com/osauer/hyperserve/v2/pkg/mcp/builtin` | Opt-in demonstration tools and resources |
| `github.com/osauer/hyperserve/v2/pkg/jsonrpc` | Standalone JSON-RPC 2.0 engine |

The runtime module has one external dependency, `golang.org/x/time`, for rate
limiting. WebSocket, JSON-RPC, and MCP are maintained in this repository. That
means fewer packages for an application to assemble, but more protocol code for
HyperServe to maintain.

The `server`, `auth`, `websocket`, `mcp`, and `jsonrpc` package APIs follow
semantic versioning on the v2 module line. Examples, generated layouts,
commands, and builtin demonstrations are maintained and tested but are not
stable import surfaces. See [API stability](./docs/API_STABILITY.md).

HyperServe does not publish a general throughput number. Its microbenchmarks and
reproducible loopback load profiles are useful for comparing revisions on the
same machine, not for predicting an application's production performance. See the
[performance guide](./docs/PERFORMANCE.md).

## Scaffold a service

```sh
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

The generator creates a Go module, server entry point, and tests. MCP is off
because the generator cannot choose the application's authorization policy.

## Documentation

- [Examples](./examples/) — runnable programs grouped by task
- [Go reference](https://pkg.go.dev/github.com/osauer/hyperserve/v2) — exported API documentation
- [Migrate from v1](./docs/MIGRATING_V2.md) — breaking changes and direct replacements
- [Production guide](./docs/PRODUCTION.md) — deployment and security boundaries
- [MCP guide](./docs/MCP_GUIDE.md) — tools, resources, transports, and discovery
- [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) — server and client behavior
- [Architecture](./ARCHITECTURE.md) — package, configuration, and lifecycle design
- [Contributing](./CONTRIBUTING.md) — local workflow and pull requests
- [Security policy](./SECURITY.md) — supported releases and private reporting

MIT — see [LICENSE](./LICENSE). Bugs and usage questions belong in
[GitHub Issues](https://github.com/osauer/hyperserve/issues).
