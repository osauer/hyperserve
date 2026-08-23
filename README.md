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
on the v2 implementation, with notably faster unmarshaling. HyperServe is
useful when the problem is no longer one handler, but the same service plumbing
accumulating around every handler:

- **Stay in the standard HTTP model.** Routes use `http.ServeMux` patterns,
  handlers remain `http.Handler` or `http.HandlerFunc`, and middleware keeps
  the usual `func(http.Handler) http.Handler` shape. Existing Go code does not
  need a framework-specific context or adapter layer.
- **Own less boundary and lifecycle glue.** Typed request binding, validation,
  safe error responses, default timeouts, panic recovery, logging, metrics,
  graceful shutdown, readiness, health endpoints, and rooted static-file
  serving live in one tested server shell.
- **Use one WebSocket API at both edges.** The server upgrader and outbound
  client put origin checking, message-size limits, handshake validation, and
  context-aware client I/O in one small package instead of requiring a separate
  networking stack.

If routes plus JSON are all you need, use `net/http` directly. Adopt HyperServe
when you would otherwise maintain this same service shell in each binary. Its
value is the integration between these concerns, not replacing Go's standard
library. MCP remains optional; it is not the reason to choose the HTTP and
WebSocket layer.

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

The result is a small production-shaped server without hiding the underlying
HTTP model. Add options only for concerns the service actually has, such as a
separate health listener, rate limiting, hardened headers, rooted static-file
serving, or deferred startup.

## Outbound WebSocket client

`websocket.Dial` supports `ws` and `wss`, context cancellation throughout the
opening handshake, TLS verification, bounded redirects, custom headers,
subprotocol negotiation, and a 1 MiB default read limit. Client frames are
masked on the wire as required by RFC 6455.

The snippet below opens one authenticated, time-bounded handshake, sends one
message, and reads one reply. It shows how a relay, event feed, or worker can
reuse an application's existing HTTP client and cancellation tree. Reconnect
and retry policy deliberately remain with the application.

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

The server counterpart solves the browser upgrade boundary: accept only known
origins, cap message memory, and then work with complete messages. This minimal
echo handler is the protocol skeleton; application code can keep the same
connection API while replacing the echo with its own loop or dispatcher.

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

The MCP handler supports current stateless Streamable HTTP, initialize-era
HTTP/stdio compatibility, discovery, namespaces, and resource templates. A
legacy HyperServe-specific routed-SSE mode remains isolated and documented as
non-standard. Built-in tools and resources are off by default. See the
[MCP guide](./docs/MCP_GUIDE.md).

## Binding and validation

`server.JSONHandler`, `BindJSON`, `BindQuery`, `BindForm`, and `Validate`
cover typed request input without another dependency. Supported validation
rules are `required`, `min`, `max`, `len`, `email`, `url`, and `oneof`.

Go 1.27 improves JSON parsing, but endpoint code still has to decode input,
validate its contract, choose safe status codes, and encode a response. This
snippet collapses that repeated boundary code so the function contains only
the operation being performed:

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

Malformed or invalid input becomes a structured `400` response. Successful
output is encoded as JSON; expected application errors can provide an HTTP
status, while unexpected errors become a generic `500` without leaking their
details. Use the lower-level bind helpers when an endpoint needs custom
envelopes, streaming, or multi-step responses.

See [`examples/binding`](./examples/binding/) for typed and lower-level forms.

## Scaffold a service

The initializer is for the point where copying a working `main.go` starts to
create drift. It generates the module, server entry point, and tests as a
starting boundary—not an application architecture you must keep.

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
