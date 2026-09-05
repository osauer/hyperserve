# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve/v2.svg)](https://pkg.go.dev/github.com/osauer/hyperserve/v2)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a Go server library built on `net/http`. It brings middleware,
typed input, readiness, graceful shutdown, and optional streaming protocols
under one server while keeping standard handlers and `ServeMux` patterns.

Use it when those concerns should share one HTTP boundary and lifecycle. If a
service only needs routes and JSON, plain `net/http` is usually the better
choice. HyperServe does not provide an ORM, browser sessions, identity-provider
setup, or application authorization.

## Quick start

HyperServe requires Go 1.27.

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/osauer/hyperserve/v2@v2.1.1
```

Save this as `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"

    "github.com/osauer/hyperserve/v2"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    app, err := hyperserve.New()
    if err != nil {
        log.Fatal(err)
    }

    app.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello, %s!\n", r.PathValue("name"))
    })

    if err := app.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

Run `go run .`, then request
`http://localhost:8080/hello/Ada`. The application turns Ctrl+C or SIGTERM into
cancellation; HyperServe follows that context and drains the resources it
started. Handlers continue to use `r.Context()` for request lifetime.

For the next step, use the [examples](./examples/), the
[production guide](./docs/PRODUCTION.md), or the
[Go reference](https://pkg.go.dev/github.com/osauer/hyperserve/v2).

## The application model

| Phase | API | Purpose |
|---|---|---|
| Configure | `hyperserve.New(hyperserve.With...())` | Choose addresses, timeouts, and optional capabilities. |
| Attach middleware | `app.Use(...)`, `app.UsePrefix(...)` | Wrap every request or one path tree in an explicit order. |
| Register routes | `app.Handle(...)`, `app.HandleFunc(...)` | Add ordinary `net/http` handlers and method-aware `ServeMux` patterns. |
| Run | `app.Run(ctx)` | Serve until the application context is cancelled or serving exits. |

Constructor options are applied from left to right. Register middleware before
`Run` or before the first request through `Handler`.

### Middleware

HyperServe middleware has the standard
`func(http.Handler) http.Handler` shape. `New` installs request metrics,
structured request logging, and panic recovery. Add application policy with
`Use` or `UsePrefix`; the first registered wrapper is the outermost one.
Middleware from another package works without an adapter.

### Rate limiting

Create a limiter, then attach it to the path that shares the quota:

```go
import "github.com/osauer/hyperserve/v2/ratelimit"

apiLimit, err := ratelimit.New(ratelimit.Config{
    RequestsPerSecond: 20,
    Burst:             40,
})
if err != nil {
    log.Fatal(err)
}

app.UsePrefix("/api", apiLimit)
```

One returned middleware value is one quota namespace. Reuse it to share quotas;
call `ratelimit.New` again to isolate them. The default client key is the
normalized transport peer from `Request.RemoteAddr`; forwarding headers are
not trusted. Deployments behind known proxies can opt in with
`ratelimit.TrustedProxyClientKey`. See
[production rate limiting](./docs/PRODUCTION.md#rate-limit-identity-is-a-separate-trust-decision)
for the trust and capacity rules.

## HTTP capabilities

Method-aware patterns and `r.PathValue` come from `net/http.ServeMux`.
Existing handlers remain ordinary `http.Handler` values:

```go
app.Handle("/admin/", existingHandler)
handler := app.Handler()
```

`JSONHandler` is the short path for typed JSON endpoints. `BindJSON`,
`BindQuery`, `BindForm`, and `Validate` are available when a handler owns
its response shape. Start with the [binding example](./examples/binding/) and
the larger [JSON API](./examples/json-api/).

Disk-backed templates and static files are disabled until the application
selects a root with `WithTemplateDir` or `WithStaticDir`. Static serving is
confined with `os.Root`; registration fails if that boundary cannot be
opened. See the [static-files example](./examples/static-files/).

Browser security headers are also explicit:

```go
app.Use(hyperserve.SecureWeb(app.Options()))
```

`Options()` returns an independent snapshot. Changing it does not reconfigure
the running application.

## Lifecycle and configuration

The application owns process signals and the root context.
`Shutdown(ctx)` is available when another component
coordinates the deadline. MCP over standard input/output uses `RunStdio()`.

Configuration files and process environment are opt-in:

```go
app, err := hyperserve.New(
    hyperserve.WithConfigFile(configPath),
    hyperserve.WithEnvironment(),
    hyperserve.WithAddr("127.0.0.1:8080"),
)
```

Later options win, so the final address above is an application invariant. A
bare `New()` reads neither source. See the
[migration guide](./docs/MIGRATING_V2_1.md) for retired configuration keys.

## Streaming and optional protocols

| Need | Starting point |
|---|---|
| Push progress or dashboard updates from one HTTP request | [Server-Sent Events example](./examples/htmx-stream/) |
| Let client and server send independently | [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) |
| Expose tools or resources to MCP clients | [MCP guide](./docs/MCP_GUIDE.md) |

Long-lived handlers must stop when `r.Context()` is cancelled. WebSocket
reconnection, SSE resume policy, MCP authentication, and application
authorization remain caller-owned.

## Public packages

| Import path | Purpose |
|---|---|
| `github.com/osauer/hyperserve/v2` | HTTP server, middleware, lifecycle, typed input, pages, and MCP wiring |
| `github.com/osauer/hyperserve/v2/auth` | Provider-neutral request authentication and stable principals |
| `github.com/osauer/hyperserve/v2/jsonrpc` | Standalone JSON-RPC 2.0 engine |
| `github.com/osauer/hyperserve/v2/mcp` | MCP handler, transports, discovery, tools, and resources |
| `github.com/osauer/hyperserve/v2/mcp/builtin` | Opt-in built-in MCP tools and resources |
| `github.com/osauer/hyperserve/v2/ratelimit` | Bounded rate-limit middleware and trusted-proxy client keys |
| `github.com/osauer/hyperserve/v2/websocket` | WebSocket upgrader, connection, and outbound dialer |

The runtime module has one external dependency, `golang.org/x/time`, used by
the standalone rate-limit gate. WebSocket, JSON-RPC, and MCP are maintained in
this repository. That keeps the shipped graph small while making HyperServe
responsible for more protocol and security code.

Only the latest stable release receives bug fixes and security updates.
Older tags remain available for reproducible builds; there are no parallel
maintenance branches.

Examples, generated layouts, commands, and demonstrations are tested but are
not stable import surfaces. Repository benchmarks compare revisions on the
same machine; HyperServe publishes no universal throughput claim. See
[API stability](./docs/API_STABILITY.md) and
[performance methodology](./docs/PERFORMANCE.md).

## Scaffold a service

```sh
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@v2.1.1
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

The generated application owns its configuration, lifecycle, and limiter
policy. MCP remains off by default because the generator cannot choose the
application's authorization policy. See [scaffolding](./docs/SCAFFOLDING.md).

## Documentation

- [Examples](./examples/) — focused runnable programs
- [Migrating from v1](./docs/MIGRATING_V2.md) — v1 to the current v2 API
- [Migrating from v2.0.x](./docs/MIGRATING_V2_1.md) — earlier package and configuration changes
- [Production guide](./docs/PRODUCTION.md) — deployment, TLS, proxies, and security boundaries
- [Architecture](./ARCHITECTURE.md) — package, configuration, and lifecycle design
- [Project structure](./PROJECT_STRUCTURE.md) — code map and package graph
- [Contributing](./CONTRIBUTING.md) — local workflow and pull requests
- [Security policy](./SECURITY.md) — supported releases and private reporting

MIT — see [LICENSE](./LICENSE). Report bugs in
[GitHub Issues](https://github.com/osauer/hyperserve/issues); ask usage questions in
[Discussions](https://github.com/osauer/hyperserve/discussions).
