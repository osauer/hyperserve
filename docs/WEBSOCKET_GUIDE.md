# WebSocket guide

HyperServe's `github.com/osauer/hyperserve/v2/websocket` package implements
RFC 6455 without an external WebSocket dependency. It supports HTTP server
upgrades and outbound `ws`/`wss` clients.

```go
import "github.com/osauer/hyperserve/v2/websocket"
```

## Outbound client

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

conn, resp, err := websocket.Dial(ctx, "wss://relay.example.com/ws", &websocket.DialOptions{
    HTTPClient:   relayHTTPClient,
    HTTPHeader:   http.Header{"Authorization": {"Bearer " + token}},
    Subprotocols: []string{"relay.v1"},
})
if err != nil {
    // resp is non-nil when the peer returned an HTTP response.
    return err
}
defer conn.Close()

if err := conn.Write(ctx, websocket.TextMessage, []byte("online")); err != nil {
    return err
}
messageType, payload, err := conn.Read(ctx)
```

`Dial` provides:

- context-aware TCP, TLS, request, response, and redirect handling;
- normal certificate and hostname verification for `wss`;
- the supplied HTTP client's transport, proxy, cookie jar, redirect callback,
  and handshake timeout;
- up to ten redirects, with credentials stripped across origins and secure to
  insecure redirects rejected;
- validation of the upgrade, accept key, subprotocol, and extension response;
- correct masking of every client data and control frame.

With no `HTTPClient`, HyperServe connects directly and `NetDialer` plus
`TLSConfig` configure the socket and TLS handshake. With `HTTPClient`, its
transport owns dialing, TLS, and proxy behavior; the options are mutually
exclusive. The standard transport supports HTTP proxies, `CONNECT` tunnels
for `wss`, and `http.ProxyFromEnvironment`.

HyperServe shallow-copies the supplied client. `HTTPClient.Timeout` bounds the
opening handshake only and is cleared on the copy before `Do`, so net/http
does not wrap or expire the upgraded stream. A custom transport must return a
writable `io.ReadWriteCloser` response body for status 101. If it does not
expose a `net.Conn`, context-aware `Read` and `Write` still cancel by closing
the stream, but explicit deadline setters return `errors.ErrUnsupported` and
`LocalAddr`/`RemoteAddr` return nil. Compression and other extensions are
rejected.

`Read` and `Write` accept contexts. Canceling a context interrupts the active
network operation and closes the connection because a frame may have been
partially transferred. One reader and one writer may run concurrently. Use
`MaxMessageSize` to replace the 1 MiB default inbound limit.

`Close()` sends normal closure. Use `CloseWithStatus(code, reason)` when the
peer needs an application-specific close code and UTF-8 reason.

## Server upgrade

```go
upgrader := websocket.Upgrader{
    AllowedOrigins: []string{"https://app.example.com"},
    Subprotocols:   []string{"chat.v1"},
    MaxMessageSize: 512 << 10,
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    for {
        messageType, payload, err := conn.ReadMessage()
        if err != nil {
            return
        }
        if err := conn.WriteMessage(messageType, payload); err != nil {
            return
        }
    }
}
```

The zero-value upgrader accepts only requests whose `Origin` host matches the
request host. Non-browser clients commonly omit `Origin`; configure
`CheckOrigin` when those clients are expected. `AllowedOrigins` supports exact
origins, `*`, and `*.example.com` host patterns.

Use `BeforeUpgrade` for authentication or admission control. Set
`RequireProtocol` when a supported subprotocol is mandatory. A custom `Error`
callback may render handshake failures.

## Deadlines and control frames

The context-aware methods are the normal outbound-client path. The lower-level
API also exposes `SetReadDeadline`, `SetWriteDeadline`, `ReadMessage`,
`WriteMessage`, and `WriteControl`.

Ping frames receive automatic pong responses while reads are active. Installing
a custom ping handler transfers responsibility for the pong to that handler.
Close frames are echoed before `Read` or `ReadMessage` returns a close error.

## Protocol and security boundaries

- Text messages and close reasons must be valid UTF-8.
- Control frames must be final and no larger than 125 bytes.
- Client frames must be masked; server frames must not be masked.
- Reserved bits, reserved opcodes, non-canonical lengths, invalid close codes,
  unexpected continuations, and oversized fragmented messages are rejected.
- The default complete-message read limit is 1 MiB on clients and servers.
- Use `wss` in production and authenticate before upgrading.
- Rate-limit connection attempts with an application-owned gate and validate
  payloads after the protocol layer accepts them.

Middleware around an upgrade route must preserve `http.Hijacker`. HyperServe's
built-in logging middleware does so.
