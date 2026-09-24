# HTMX + Server-Sent Events streaming

An HTMX page receiving a random number every 100 ms. The server uses
`sse.NewWriter(w, 5*time.Second)` to encode JSON events, bound each write,
and flush through `http.ResponseController`.

## Run

```sh
cd examples/htmx-stream
go run .
```

Open <http://localhost:8080>. Closing the tab cancels the request and stops the
handler. A write or flush failure also ends the stream.

## Writing frames

```go
stream, err := sse.NewWriter(w, 5*time.Second)
if err != nil {
    return
}
if err := stream.Send("status", "42", []byte("ready")); err != nil {
    return
}
```

`Send` accepts raw UTF-8 data; `SendJSON` marshals a Go value and returns encoding
errors before writing. `Comment("heartbeat")` sends a flushed comment without
dispatching an event. The application chooses when to send heartbeats.

The writer requires a positive timeout, deadline support, and flushing.
Middleware can expose the underlying response with `Unwrap() http.ResponseWriter`.
Deadlines are cleared after each frame so an idle stream stays connected.
Set custom headers, such as `Cache-Control: no-store`, before the first frame.

An empty ID omits the field and preserves the browser's last event ID. Invalid
event names or IDs (CR, LF, NUL, or invalid UTF-8) return an error. Existing
`hyperserve.SSEMessage` formatting remains available with its original behavior.
ID assignment, persistence, and replay from `Last-Event-ID` belong to the
application. This example does not retain events for replay. See the
[SSE specification](https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-header).

MCP's separate request-scoped SSE behavior is covered under
[resource subscriptions](../../docs/MCP_GUIDE.md#resource-subscriptions).
