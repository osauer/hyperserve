# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a Go library for the code that accumulates around `net/http`:
server lifecycle, middleware, typed request binding, rooted static files,
WebSockets, and an optional Model Context Protocol (MCP) endpoint. Routes still
use `http.ServeMux` patterns, and handlers remain `http.Handler` values.

If routes plus JSON are all your service needs, use `net/http` directly.
HyperServe is useful when the same timeout, recovery, health, shutdown, input,
and WebSocket plumbing would otherwise be rebuilt around each service.

## Install

HyperServe requires Go 1.27.

```sh
go get github.com/osauer/hyperserve@latest
```

The runtime module has one external dependency, `golang.org/x/time`, for rate
limiting. Build and conformance tools live in a separate
[`tools/go.mod`](./tools/go.mod) module.

## Start a server

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

    srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })

    // Run owns process signals and drains active requests during shutdown.
    if err := srv.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Save the example as `main.go` in your module, then start it:

```sh
go run .
```

From another terminal:

```sh
curl http://localhost:8080/
```

`NewServer` listens on `:8080` and installs request logging, metrics, and panic
recovery. `Run` handles SIGINT, SIGTERM, and SIGQUIT. If the application already
owns its lifecycle, use `RunContext(appCtx)` instead; context cancellation is a
normal request to drain and stop.

Add `WithHealthServer()` for separate liveness and readiness endpoints, or
`WithDeferredInit(...)` when readiness must wait for startup work. The
[`deferred-init` example](./examples/deferred-init/) shows both.

## Bind and validate input

`JSONHandler` keeps decoding, validation, and safe error responses at the HTTP
boundary while the callback works with Go values:

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

Malformed or invalid input produces a structured `400`; unexpected callback
errors produce a generic `500` without exposing error details. Use `BindJSON`,
`BindQuery`, `BindForm`, and `Validate` directly when the response needs custom
headers, streaming, or a different envelope. See the
[`binding` example](./examples/binding/) for both levels.

## Use WebSockets on either side

The `websocket` package implements RFC 6455 for servers and outbound clients.
Server upgrades default to same-origin browser requests and both sides enforce a
1 MiB message limit unless configured otherwise.

Use `srv.WebSocketUpgrader()` when the upgrade should appear in server metrics:

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
    if err := conn.Write(r.Context(), messageType, payload); err != nil {
        return
    }
})
```

For outbound connections, `websocket.Dial` accepts a context plus either a
caller-owned `http.Client` or explicit dial/TLS settings. It supports headers,
subprotocols, TLS verification, and bounded redirects; reconnect policy remains
with the application. See the [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) and
the [browser echo example](./examples/websocket-demo/).

## Add MCP when the service needs it

MCP is opt-in and does not change the HTTP or WebSocket APIs:

```go
srv, err := server.NewServer(
    server.WithMCPSupport("payments", "1.0.0"),
)
```

The standard endpoint implements MCP 2026-07-28 Streamable HTTP: finite
requests return JSON, while `subscriptions/listen` uses request-scoped SSE.
Initialize-era 2025-11-25 request/response remains available for older clients.
HyperServe's proprietary routed-SSE transport is deprecated and disabled by
default.

Applications must put their own authorization middleware in front of `/mcp`.
Built-in tools and resources are also disabled by default. The
[MCP guide](./docs/MCP_GUIDE.md) documents transport headers, subscriptions,
limits, discovery, built-ins, and the legacy migration path.

## Packages

| Import path | Purpose |
| --- | --- |
| `github.com/osauer/hyperserve/pkg/server` | HTTP server, middleware, lifecycle, and MCP wiring |
| `github.com/osauer/hyperserve/pkg/websocket` | WebSocket upgrader, connection, and outbound dialer |
| `github.com/osauer/hyperserve/pkg/mcp` | MCP handler, transports, discovery, and namespaces |
| `github.com/osauer/hyperserve/pkg/mcp/builtin` | Opt-in demonstration tools and resources |
| `github.com/osauer/hyperserve/pkg/jsonrpc` | Standalone JSON-RPC 2.0 engine |

The four top-level package APIs follow semantic versioning on the v1 module
line. Generated layouts, examples, and commands are maintained and tested but
are not stable import surfaces. See [API stability](./docs/API_STABILITY.md) for
the compatibility and deprecation policy.

## Generate a service

`hyperserve-init` creates a Go module, a server entry point, and tests:

```sh
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

The generated project omits MCP because the initializer cannot choose an
application's authorization policy. Pass `--with-mcp` only when the service will
protect `/mcp` itself. `--local-replace` points a generated project at a local
HyperServe checkout.

## Documentation and development

- [Examples](./examples/) — runnable programs grouped by task
- [Production guide](./docs/PRODUCTION.md) — proxies, TLS, health, shutdown, and security boundaries
- [Architecture](./ARCHITECTURE.md) — package and lifecycle design
- [Go reference](https://pkg.go.dev/github.com/osauer/hyperserve) — exported API documentation
- [Contributing](./CONTRIBUTING.md) — local workflow and pull requests
- [Security policy](./SECURITY.md) — supported releases and private reporting

Repository checks:

```sh
make check
make test-race
make fuzz-smoke
```

`make check` runs vet, Staticcheck, vulnerability scans, Go modernization,
example builds, and MCP conformance checks. Bugs and usage questions belong in
[GitHub Issues](https://github.com/osauer/hyperserve/issues).

MIT — see [LICENSE](./LICENSE).
