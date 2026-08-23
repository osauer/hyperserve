# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/osauer/hyperserve)](go.mod)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a small, `net/http`-shaped service kit for Go: HTTP lifecycle,
middleware, and typed inputs; an RFC 6455 WebSocket implementation for servers
and outbound clients; and an optional in-process Model Context Protocol (MCP)
control plane.

The shipped module has one external dependency, `golang.org/x/time`. Developer
tooling lives in the separate [`tools/go.mod`](./tools/go.mod) graph and does
not enter applications that import HyperServe.

## Install

```sh
go get github.com/osauer/hyperserve@latest
```

Public packages:

| Import path | Purpose |
| --- | --- |
| `github.com/osauer/hyperserve/pkg/server` | HTTP server, middleware, lifecycle, MCP wiring |
| `github.com/osauer/hyperserve/pkg/mcp` | MCP handler, transports, discovery, namespaces |
| `github.com/osauer/hyperserve/pkg/mcp/builtin` | Opt-in demonstration tools and resources |
| `github.com/osauer/hyperserve/pkg/websocket` | WebSocket server upgrader and outbound client |
| `github.com/osauer/hyperserve/pkg/jsonrpc` | Standalone JSON-RPC 2.0 engine |

## Why HyperServe?

[Go 1.27](https://go.dev/doc/go1.27) makes an already strong standard library
better: `net/http` is a capable router and server, and `encoding/json` now runs
on the v2 implementation, with notably faster unmarshaling. If routes plus JSON
are all you need, use `net/http` directly. HyperServe earns its place when the
same service plumbing starts accumulating around every handler:

- **Keep the standard HTTP model.** Routes use `http.ServeMux` patterns,
  handlers remain `http.Handler` or `http.HandlerFunc`, and middleware follows
  the familiar handler-wrapper model.
- **Own less boundary and lifecycle glue.** Typed request binding, validation,
  safe error responses, default timeouts, panic recovery, logging, metrics,
  graceful shutdown, readiness, health endpoints, and rooted static-file
  serving live in one tested server shell.
- **Use one WebSocket package at both edges.** Server upgrades, outbound dialing,
  origin and message limits, handshake validation, and context-aware client I/O
  share one API.

The value is this integration, not a replacement programming model. MCP remains
optional to the HTTP and WebSocket layer.

## HTTP quick start

This example is intentionally ordinary Go. `GET` registers the standard
`"GET /"` `ServeMux` pattern, while the callback remains an
`http.HandlerFunc`. `Run` supplies the part usually repeated around the mux:
configured timeouts, default logging/metrics/recovery middleware, signal
handling, and graceful shutdown.

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/osauer/hyperserve/pkg/server"
)

func main() {
    // Adds the default logging, metrics, recovery, timeouts, and health server.
    srv, err := server.NewServer(server.WithHealthServer())
    if err != nil {
        log.Fatal(err)
    }

    // Routes and handlers remain ordinary net/http.
    srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })

    // Blocks until failure or SIGINT/SIGTERM, then drains active requests.
    if err := srv.Run(); err != nil {
        log.Fatal(err)
    }
}
```

Add options only for concerns the service has. See
[`deferred-init`](./examples/deferred-init/) for readiness-gated startup and
shutdown hooks.

## Binding and validation

Go 1.27 improves JSON parsing, but endpoint code still has to decode input,
validate its contract, map errors safely, and encode a response. `JSONHandler`
collapses that boundary code without adding a dependency:

```go
type CreateUser struct {
    Email string `json:"email" validate:"required,email"` // request contract
}

srv.POST("/users", server.JSONHandler(
    func(ctx context.Context, in CreateUser) (User, error) {
        // Binding and validation passed; this is only business logic.
        return createUser(ctx, in)
    },
))
```

Malformed or invalid input becomes a structured `400`; unexpected errors become
a generic `500` without leaking details. Use `BindJSON`, `BindQuery`, `BindForm`,
and `Validate` directly for custom envelopes, streaming, or multi-step responses.
See [`examples/binding`](./examples/binding/) for both levels.

## WebSockets

`websocket.Dial` supports `ws` and `wss`, context cancellation throughout the
opening handshake, TLS verification, bounded redirects, custom headers,
subprotocol negotiation, and a 1 MiB default read limit. Client frames are
masked on the wire as required by RFC 6455.

```go
func exchange(ctx context.Context, relayURL, token string, client *http.Client) ([]byte, error) {
    // This deadline bounds the handshake, not the upgraded connection.
    dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    conn, resp, err := websocket.Dial(dialCtx, relayURL, &websocket.DialOptions{
        // Preserve the caller's transport, proxy, cookies, and redirects.
        HTTPClient:   client,
        HTTPHeader:   http.Header{"Authorization": {"Bearer " + token}},
        Subprotocols: []string{"relay.v1"},
    })
    if err != nil {
        if resp != nil {
            return nil, fmt.Errorf("relay handshake: %s: %w", resp.Status, err)
        }
        return nil, err
    }
    defer conn.Close()

    if err := conn.Write(ctx, websocket.TextMessage, []byte("online")); err != nil {
        return nil, err
    }
    _, reply, err := conn.Read(ctx)
    return reply, err
}
```

`Read` and `Write` accept contexts and support one concurrent reader plus one
concurrent writer. Reconnect policy remains with the application. The same
package handles the server side:

```go
upgrader := websocket.Upgrader{
    // Browser upgrades are same-origin by default; name cross-origin peers.
    AllowedOrigins: []string{"https://app.example.com"},
    // Bound the aggregate size of fragmented messages.
    MaxMessageSize: 512 << 10,
}

srv.GET("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    for {
        messageType, payload, err := conn.Read(r.Context())
        if err != nil {
            return
        }
        if err := conn.Write(r.Context(), messageType, payload); err != nil {
            return
        }
    }
})
```

See the [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) for close semantics,
transport ownership, proxy behavior, deadlines, and unsupported extensions.

## MCP

Enable MCP programmatically:

```go
srv, err := server.NewServer(
    server.WithMCPSupport("payments", "1.0.0"),
    server.WithMCPBuiltinTools(true),
    server.WithMCPBuiltinResources(true),
)
if err != nil {
    log.Fatal(err)
}
```

The MCP handler supports current stateless Streamable HTTP, initialize-era
HTTP/stdio compatibility, discovery, namespaces, and resource templates. A
legacy HyperServe-specific routed-SSE mode remains isolated and documented as
non-standard. Built-in tools and resources are off by default. See the
[MCP guide](./docs/MCP_GUIDE.md) for the implemented protocol surface.

## Scaffold a service

The initializer generates a module, server entry point, and tests. It is a
starting boundary, not an application architecture you must keep.

```sh
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

Use `--with-mcp=false` to omit MCP and `--local-replace` when developing
against a local HyperServe checkout.

## Development

```sh
make check
make test-race
make fuzz-smoke
```

`make check` runs formatting, vet, Staticcheck, govulncheck, Go 1.27
modernization, standalone example checks, and canonical example builds. See
[CONTRIBUTING.md](./CONTRIBUTING.md) for the complete workflow.

## Documentation

- [Architecture](./ARCHITECTURE.md)
- [API stability](./docs/API_STABILITY.md)
- [Production guide](./docs/PRODUCTION.md)
- [WebSocket guide](./docs/WEBSOCKET_GUIDE.md)
- [MCP guide](./docs/MCP_GUIDE.md)
- [Examples](./examples/)
- [Security policy](./SECURITY.md)

MIT — see [LICENSE](./LICENSE).
