# HyperServe

[![CI](https://github.com/osauer/hyperserve/actions/workflows/ci.yml/badge.svg)](https://github.com/osauer/hyperserve/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/osauer/hyperserve?label=release&sort=semver)](https://github.com/osauer/hyperserve/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/osauer/hyperserve)](go.mod)
[![Go reference](https://pkg.go.dev/badge/github.com/osauer/hyperserve.svg)](https://pkg.go.dev/github.com/osauer/hyperserve)
[![License: MIT](https://img.shields.io/github/license/osauer/hyperserve)](LICENSE)

HyperServe is a small, `net/http`-shaped Go server with an in-process Model
Context Protocol (MCP) control plane and an RFC 6455 WebSocket implementation
for servers and outbound clients.

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

## HTTP quick start

```go
package main

import (
    "fmt"
    "net/http"

    "github.com/osauer/hyperserve/pkg/server"
)

func main() {
    srv, _ := server.NewServer()
    srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
        fmt.Fprintln(w, "Hello, World!")
    })
    srv.Run()
}
```

HyperServe provides method-aware routes, middleware, graceful shutdown,
sandboxed static files, request binding and validation, and deferred startup.
It stays close to standard-library handler shapes.

## Outbound WebSocket client

`websocket.Dial` supports `ws` and `wss`, context cancellation throughout the
opening handshake, TLS verification, bounded redirects, custom headers,
subprotocol negotiation, and a 1 MiB default read limit. Client frames are
masked on the wire as required by RFC 6455.

```go
package relay

import (
    "context"
    "net/http"
    "time"

    "github.com/osauer/hyperserve/pkg/websocket"
)

func exchange(ctx context.Context, relayURL, token string, httpClient *http.Client) error {
    dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    conn, resp, err := websocket.Dial(dialCtx, relayURL, &websocket.DialOptions{
        HTTPClient:   httpClient,
        HTTPHeader:   http.Header{"Authorization": {"Bearer " + token}},
        Subprotocols: []string{"relay.v1"},
    })
    if err != nil {
        _ = resp // non-nil when the peer returned an HTTP response
        return err
    }
    defer conn.Close()

    if err := conn.Write(ctx, websocket.TextMessage, []byte("online")); err != nil {
        return err
    }
    messageType, payload, err := conn.Read(ctx)
    _ = messageType
    _ = payload
    return err
}
```

`Read` and `Write` accept contexts and support one concurrent reader plus one
concurrent writer. Canceling either operation closes a potentially partial
connection. `ReadMessage`, `WriteMessage`, and deadline setters remain
available for lower-level use. `CloseWithStatus` sends an explicit close code
and reason; `Close` sends normal closure. Compression and other WebSocket
extensions are not negotiated.

Pass a configured `HTTPClient` to preserve its transport, proxy, cookie jar,
redirect callback, and handshake timeout. `http.DefaultClient` uses
`http.ProxyFromEnvironment` through the standard transport. HyperServe copies
the client and applies `HTTPClient.Timeout` only to the handshake, so it does
not expire the upgraded connection. Custom transports must return an
`io.ReadWriteCloser` body for a successful 101 response; `http.Transport`
does. `HTTPClient` is mutually exclusive with `NetDialer` and `TLSConfig`.

## WebSocket server

```go
upgrader := websocket.Upgrader{
    AllowedOrigins: []string{"https://app.example.com"},
    MaxMessageSize: 512 << 10,
}

srv.GET("/ws", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    messageType, payload, err := conn.ReadMessage()
    if err == nil {
        err = conn.WriteMessage(messageType, payload)
    }
    _ = err
})
```

The upgrader defaults to same-origin browser requests. Configure
`AllowedOrigins` or `CheckOrigin` deliberately for cross-origin clients. See
the [WebSocket guide](./docs/WEBSOCKET_GUIDE.md) for handshake, limits, and
deployment details.

## MCP

Enable MCP programmatically:

```go
srv, _ := server.NewServer(
    server.WithMCPSupport("payments", "1.0.0"),
    server.WithMCPBuiltinTools(true),
    server.WithMCPBuiltinResources(true),
)
```

The unified MCP handler supports HTTP, SSE, and stdio transports, discovery,
namespaces, resource templates, and live resource subscriptions. Built-in
tools and resources are off by default. See the [MCP guide](./docs/MCP_GUIDE.md).

## Binding and validation

`server.JSONHandler`, `BindJSON`, `BindQuery`, `BindForm`, and `Validate`
cover typed request input without another dependency. Supported validation
rules are `required`, `min`, `max`, `len`, `email`, `url`, and `oneof`.

```go
type CreateUser struct {
    Email string `json:"email" validate:"required,email"`
}

srv.POST("/users", server.JSONHandler(
    func(ctx context.Context, in CreateUser) (User, error) {
        return createUser(ctx, in)
    },
))
```

See [`examples/binding`](./examples/binding/) for typed and lower-level forms.

## Scaffold a service

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
