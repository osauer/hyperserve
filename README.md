# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/osauer/hyperserve)](go.mod)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve/pkg/server)
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
- MCP server (HTTP, SSE, stdio transports) with discovery endpoints and namespace support.
- WebSocket implementation (RFC 6455).
- JSON-RPC 2.0 engine reused by MCP.
- Middleware: recovery, request logging, metrics, CORS, security headers, rate limiting, auth.
- Static file serving sandboxed via `os.Root`.
- Deferred-init lifecycle: serve `/healthz` immediately while bootstrap work runs in the background.
- **Request binding + validation** (`server.Bind`, `server.BindJSON`, `server.BindQuery`, `server.BindForm`)
  with struct-tag rules (`required,min,max,len,email,url,oneof`) and structured
  `*ValidationError` for per-field 400 responses. No external dependencies. See
  [examples/binding](./examples/binding/).

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

```go
type createUser struct {
    Name  string `json:"name"  validate:"required,min=2,max=64"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age"   validate:"required,min=13,max=120"`
    Role  string `json:"role"  validate:"oneof=admin user guest"`
}

srv.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
    var u createUser
    if err := server.BindJSON(r, &u); err != nil {
        var verr *server.ValidationError
        if errors.As(err, &verr) {
            // verr.Fields is []*FieldError — one entry per failing rule,
            // ready to render as a structured 400 response.
        }
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // u is valid; carry on.
})
```

Available rules: `required, min, max, len, email, url, oneof`.

Entry points:

- `server.Bind(r, dst)` — picks JSON / form / query by `Content-Type`.
- `server.BindJSON(r, dst)` — JSON with `DisallowUnknownFields` and a 1 MiB body cap.
- `server.BindQuery(r, dst)` — URL query parameters (slices via repeated keys).
- `server.BindForm(r, dst)` — `application/x-www-form-urlencoded` or `multipart/form-data`.
- `server.Validate(dst)` — run rules without binding.

See [examples/binding](./examples/binding/) for a working endpoint with
structured 400 responses.

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

- [Architecture](./ARCHITECTURE.md)
- [API specification](./spec/api.md)
- [MCP guide](./docs/MCP_GUIDE.md)
- [WebSocket guide](./docs/WEBSOCKET_GUIDE.md)
- [Scaffolding guide](./docs/SCAFFOLDING.md)

## License

MIT — see [LICENSE](./LICENSE).
