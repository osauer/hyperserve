# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/osauer/hyperserve)](go.mod)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

A Go HTTP framework with built-in MCP (Model Context Protocol) support. The runtime
has one transitive dependency: `golang.org/x/time`. (The `go.mod` `tool` directive
pulls in `golang.org/x/tools` for the modernize check gate; those are build-time
only and don't ship in your binary.)

The point: a small `net/http`-shaped server that ships an MCP control plane in the
same binary, so AI assistants can introspect and operate the server without an
out-of-process bridge.

## Quick Start

```go
import (
    "fmt"
    "net/http"

    server "github.com/osauer/hyperserve/pkg/server"
)

func main() {
    srv, _ := server.NewServer()

    srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })

    srv.Run()
}
```

## Install

```bash
go get github.com/osauer/hyperserve/pkg/server
```

## What's in the box

- HTTP server built on `net/http`, with grouping, middleware chain, and graceful shutdown.
- Method-aware route registration (`srv.GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`)
  on top of stdlib 1.22+ pattern syntax — wrong-method requests get an automatic 405.
- MCP server (HTTP, SSE, stdio transports) with discovery endpoints and namespace support.
- WebSocket implementation (RFC 6455).
- JSON-RPC 2.0 engine reused by MCP.
- Middleware: recovery, request logging, metrics, CORS, security headers, rate limiting, auth.
- Static file serving sandboxed via `os.Root`.
- Deferred-init lifecycle: serve `/healthz` immediately while bootstrap work runs in the background.
- **Request binding + validation** with struct-tag rules
  (`required,min,max,len,email,url,oneof`) and structured `*ValidationError`
  for per-field 400 responses. Use `server.JSONHandler[In, Out]` for typed
  bind + validate + respond in one line, or drop to `server.Bind` /
  `BindJSON` / `BindQuery` / `BindForm` for finer control. No external
  dependencies. See [examples/binding](./examples/binding/).

## Scaffold a new service

```bash
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

Flags: `--name` (display name), `--out` (output directory), `--with-mcp=false` to opt
out of MCP, `--local-replace` to develop against a local checkout.

## MCP

```bash
HS_MCP_ENABLED=true
HS_MCP_SERVER_NAME=MyServer
HS_MCP_SERVER_VERSION=1.0.0
```

Or programmatically:

```go
srv, _ := server.NewServer(
    server.WithMCPSupport("MyServer", "1.0.0"),
    server.WithMCPBuiltinTools(true),
    server.WithMCPBuiltinResources(true),
)
```

Built-in MCP tools and resources are off by default; you opt in per server.

## Request binding & validation

Parse a JSON body, query string, or form into a typed struct, then validate
against struct-tag rules. Zero external dependencies; the rules cover the
checks that show up in 90% of input-handling code.

The fastest path is `server.JSONHandler`, which wraps bind + validate +
respond around a typed business function. The handler shrinks to the one
line of logic that actually changes per endpoint:

```go
type CreateUser struct {
    Name  string `json:"name"  validate:"required,min=2,max=64"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age"   validate:"required,min=13,max=120"`
    Role  string `json:"role"  validate:"required,oneof=admin user guest"`
}

srv.POST("/users", server.JSONHandler(
    func(ctx context.Context, in CreateUser) (User, error) {
        return createUser(ctx, in)
    },
))
```

`JSONHandler` decodes the body, runs `validate:"..."` rules, calls your
function with `r.Context()`, then JSON-encodes the result with 200. A
`*server.ValidationError` becomes a per-field 400 envelope; an error
implementing `HTTPStatus() int` (e.g. `server.NewStatusError(404, "…")`)
sets its own status; everything else is a 500. Returning `struct{}` or a
nil pointer writes 204.

When you need more control — custom headers, streaming, a non-JSON body —
drop to the lower-level binders:

```go
srv.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
    var u CreateUser
    if err := server.BindJSON(r, &u); err != nil {
        var verr *server.ValidationError
        if errors.As(err, &verr) {
            // verr.Fields is []*FieldError — render however you want.
        }
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // u is valid; carry on.
})
```

Available rules: `required, min, max, len, email, url, oneof`.

Entry points:

- `server.JSONHandler[In, Out](fn)` — typed wrapper: bind + validate + respond.
- `server.JSONEcho[T]()` — shorthand for validate-and-pass-through (webhook acks, dev stubs).
- `server.Bind(r, dst)` — picks JSON / form / query by `Content-Type`.
- `server.BindJSON(r, dst)` — JSON with `DisallowUnknownFields` and a 1 MiB body cap.
- `server.BindQuery(r, dst)` — URL query parameters (slices via repeated keys).
- `server.BindForm(r, dst)` — `application/x-www-form-urlencoded` or `multipart/form-data`.
- `server.Validate(dst)` — run rules without binding.

See [examples/binding](./examples/binding/) for both shapes side-by-side.

## Middleware

`NewServer` wires recovery, request logging, and metrics. Apply security stacks per route:

```go
srv, _ := server.NewServer()
srv.AddMiddleware("/api", server.RateLimitMiddleware(srv))
srv.AddMiddlewareStack("/web", server.SecureWeb(srv.Options))
```

## Deferred initialization

Serve `/healthz` immediately, return 503 for application routes, flip to ready once
bootstrap (and any `WithOnReady` hooks) succeed:

```go
srv, _ := server.NewServer(
    server.WithDeferredInit(func(ctx context.Context, app *server.Server) error {
        return warmCaches(ctx)
    }),
    server.WithOnReady(func(ctx context.Context, app *server.Server) error {
        app.HandleFunc("/api/users", usersHandler)
        return nil
    }),
)
```

Use `WithDeferredInitStopOnFailure(false)` to keep the listener up after a bootstrap
failure, then call `CompleteDeferredInit(ctx, nil)` once the issue is resolved.

See [examples/deferred-init](./examples/deferred-init/).

## Examples

[examples/](./examples) covers HTTP, WebSocket, MCP (HTTP/SSE/stdio/discovery/extensions),
auth + RBAC, htmx, and static file serving. Each example is a self-contained
`go run .` target.

## Documentation

- [Roadmap](./docs/ROADMAP.md) — what's planned and why.
- [Architecture](./ARCHITECTURE.md)
- [API specification](./spec/api.md)
- [MCP guide](./docs/MCP_GUIDE.md)
- [WebSocket guide](./docs/WEBSOCKET_GUIDE.md)
- [Scaffolding guide](./docs/SCAFFOLDING.md)
- [Contributing](./CONTRIBUTING.md) — local CI gate + code map.
- [Security policy](./SECURITY.md)

## License

MIT — see [LICENSE](./LICENSE).
