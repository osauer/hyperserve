# WebSocket echo

This example shows the useful integration point between HyperServe’s HTTP
server and its dependency-free WebSocket package:

- the route remains an ordinary HTTP handler;
- same-origin browser upgrades are allowed by default;
- `srv.WebSocketUpgrader()` records successful upgrades in server telemetry;
- aggregate message memory is bounded, including fragmented messages;
- reads and writes use the request context.

The core setup is intentionally small:

```go
upgrader := srv.WebSocketUpgrader()
upgrader.MaxMessageSize = 512 << 10 // 512 KiB across all fragments

srv.GET("/ws/echo", func(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()

    messageType, payload, err := conn.Read(r.Context())
    if err == nil {
        err = conn.Write(r.Context(), messageType, payload)
    }
})
```

Run from the repository root:

```bash
go run ./examples/websocket-demo
```

Then open <http://localhost:8080>. The embedded page connects to the same
origin, sends text, and displays the echoed response.

For a cross-origin browser client, set `AllowedOrigins` to an explicit
allowlist. Reconnect policy, authentication, and application messages remain
application concerns. See the [WebSocket guide](../../docs/WEBSOCKET_GUIDE.md)
for outbound dialing, proxy behavior, close semantics, and deployment details.
