# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve/v2.svg)](https://pkg.go.dev/github.com/osauer/hyperserve/v2)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a Go server library built on `net/http`. It keeps standard
handlers and `ServeMux` patterns, then coordinates the work around them:
middleware, typed input, health and readiness, shutdown, static files,
templates, Server-Sent Events, WebSockets, and optional MCP.

Use it when those pieces should share one configuration and lifecycle. If a
small service only needs routes and JSON, plain `net/http` is usually the better
choice. HyperServe does not provide an ORM, browser sessions, or application
authorization.

HyperServe requires Go 1.27. Public package APIs on the v2 module line follow
[semantic versioning](./docs/API_STABILITY.md).

## Quick start

```sh
go get github.com/osauer/hyperserve/v2@latest
```

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

    srv.GET("/hello/{name}", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello, %s!\n", r.PathValue("name"))
    })

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

The response is `Hello, Ada!`. The executable turns Ctrl+C into cancellation;
HyperServe follows that context and drains the resources it started. Handlers
continue to use `r.Context()` for request cancellation and request-scoped data.

The [hello-world example](./examples/hello-world/) contains the same shape as a
runnable repository program.

## The server model

HyperServe separates four decisions:

| Phase | API | Purpose |
|---|---|---|
| Configure | `server.NewServer(server.With...())` | Choose addresses, timeouts, policy values, and optional capabilities. |
| Attach middleware | `srv.Use(...)`, `srv.UsePrefix(...)` | Build the ordered request pipeline globally or for one path tree. |
| Register routes | `srv.GET(...)`, `srv.Handle(...)` | Add standard `net/http` handlers and method-aware `ServeMux` patterns. |
| Run | `srv.Run(ctx)` | Serve until the application context is cancelled or the server exits. |

`server` is the imported package in these examples; `srv` is one configured
`*server.Server`. Constructor options are applied from left to right. Middleware
must be registered before `Run` or before the first request through `Handler`.

`With...` options choose server configuration during construction. Middleware
is executable request behavior whose order and path scope matter, so it stays
visible in `Use` or `UsePrefix`. Putting it behind a constructor option would
hide its position in the request pipeline and make route scope less explicit.

## Middleware patterns

`NewServer` installs request metrics, structured request logging, and panic
recovery. Add application policy with ordinary middleware of type
`func(http.Handler) http.Handler`.

### Add global middleware

This middleware adds one response header to every route:

```go
func responseHeader(name, value string) server.Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set(name, value)
            next.ServeHTTP(w, r)
        })
    }
}

srv.Use(responseHeader("X-Service-Version", "2026-08"))
```

Middleware runs in registration order; the first item is the outermost wrapper.
Middleware from another package works without an adapter when it has the same
standard handler shape.

### Apply policy to a path tree

Configuration and scope are separate. Here the constructor configures a
limiter, while `UsePrefix` applies it only to `/api` and its descendants:

```go
srv, err := server.NewServer(server.WithRateLimit(20, 40))
if err != nil {
    log.Fatal(err)
}

srv.UsePrefix("/api", server.RateLimitMiddleware(srv))
```

The prefix is segment-aware: it matches `/api/users`, not `/apiv2`. Several
middleware values form one ordered stack:

```go
srv.UsePrefix("/api", requireIdentity, server.RateLimitMiddleware(srv))
srv.UsePrefix("/api/admin", requireAdmin)
```

The [middleware example](./examples/middleware-basics/) provides a runnable
custom wrapper, global middleware, prefix middleware, and recovery path. The
[authentication example](./examples/auth/) shows how `requireIdentity` is built.

### Derive browser headers from server configuration

Browser headers depend on the server's TLS, CSP, CORS, and optional `Server`
header settings. Build the middleware from the finalized configuration, then
attach it to the request pipeline:

```go
headers := server.HeadersMiddleware(srv.Options())
srv.Use(headers)
```

`Options()` returns an independent snapshot; changing it does not reconfigure
the server. Keeping the snapshot explicit prevents middleware from reading
mutable server state. TLS, browser sessions, and authorization remain separate
application decisions.

## Routes, input, and files

Method helpers use Go's method-aware `ServeMux` patterns and path values.
Existing handlers can be registered directly, and the assembled server remains
an ordinary `http.Handler`:

```go
srv.Handle("/admin/", existingHandler)
handler := srv.Handler()
```

`JSONHandler` is the short path for typed JSON endpoints. `BindJSON`,
`BindQuery`, `BindForm`, and `Validate` are available when a handler owns its
response shape. Start with the focused [binding example](./examples/binding/),
then see the larger [JSON API](./examples/json-api/).

Disk-backed templates and static files are off until the application selects a
root with `WithTemplateDir` or `WithStaticDir`. Static files are confined with
`os.Root`, and `HandleStatic` fails instead of silently opening a weaker file
server. See [static files and an API route](./examples/static-files/).

## Lifecycle and configuration

The application owns process signals and the root context because a library
cannot decide whether one signal should stop one server, several servers, or a
larger host. `Run(ctx)` blocks until cancellation or server exit.
`Shutdown(ctx)` is available when another component coordinates the deadline;
MCP over standard input/output uses the separate `RunStdio()` entry point.

Configuration sources are opt-in and later options win:

```go
srv, err := server.NewServer(
    server.WithConfigFile(configPath),
    server.WithEnvironment(),
    server.WithAddr("127.0.0.1:8080"),
)
```

The final address is an application invariant; neither the file nor the
environment can replace it. A bare `NewServer()` reads neither source. The
[configuration example](./examples/configuration/) demonstrates conflicts and
the [deferred-init example](./examples/deferred-init/) connects dependency
startup to readiness.

## Streaming and optional protocols

Use the protocol that matches the communication pattern:

| Need | Starting point |
|---|---|
| The server pushes progress, notifications, or dashboard updates | [Server-Sent Events](./examples/htmx-stream/) |
| Client and server can both send at any time | [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) |
| The service exposes tools or resources to MCP clients | [MCP guide](./docs/MCP_GUIDE.md) |

Long-lived handlers must stop when `r.Context()` is cancelled. WebSocket
reconnection, SSE resume policy, and MCP authorization remain application
decisions.

## Packages and trade-offs

| Import path | Purpose |
|---|---|
| `github.com/osauer/hyperserve/v2/pkg/server` | HTTP server, middleware, lifecycle, pages, and MCP wiring |
| `github.com/osauer/hyperserve/v2/pkg/auth` | Provider-neutral request authentication and stable principals |
| `github.com/osauer/hyperserve/v2/pkg/websocket` | WebSocket upgrader, connection, and outbound dialer |
| `github.com/osauer/hyperserve/v2/pkg/mcp` | MCP handler, transports, discovery, tools, and resources |
| `github.com/osauer/hyperserve/v2/pkg/jsonrpc` | Standalone JSON-RPC 2.0 engine |

The runtime module has one external dependency, `golang.org/x/time`, for rate
limiting. WebSocket, JSON-RPC, and MCP are maintained in this repository. That
reduces the number of packages an application assembles, while giving
HyperServe more protocol and security code to maintain.

Examples, generated layouts, commands, and demonstrations are tested but are
not stable import surfaces. HyperServe publishes no general throughput number;
repository benchmarks compare revisions on the same machine. See
[API stability](./docs/API_STABILITY.md) and
[performance methodology](./docs/PERFORMANCE.md).

## Scaffold a service

```sh
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

The generator creates a Go module, server entry point, focused middleware,
health checks, and tests. MCP is off by default because the generator cannot
choose the application's authorization policy. When explicitly enabled, the
generated endpoint starts without built-in tools or resources.

## Documentation

- [Examples](./examples/) — focused programs followed by composition references
- [Go reference](https://pkg.go.dev/github.com/osauer/hyperserve/v2) — exported API documentation
- [Migrate from v1](./docs/MIGRATING_V2.md) — breaking changes and direct replacements
- [Production guide](./docs/PRODUCTION.md) — deployment, TLS, proxies, and security boundaries
- [Architecture](./ARCHITECTURE.md) — package, configuration, and lifecycle design
- [Contributing](./CONTRIBUTING.md) — local workflow and pull requests
- [Security policy](./SECURITY.md) — supported releases and private reporting

MIT — see [LICENSE](./LICENSE). Bugs and usage questions belong in
[GitHub Issues](https://github.com/osauer/hyperserve/issues).
