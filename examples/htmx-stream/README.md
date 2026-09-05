# HTMX + Server-Sent Events streaming

An HTMX page that subscribes to an SSE endpoint and updates a DOM element
in real time as the server streams events. Demonstrates:

- Setting the SSE response headers correctly
  (`text/event-stream`, `no-cache`, `keep-alive`).
- Using `http.Flusher` to push individual events.
- Cleanly stopping on `r.Context().Done()` when the client disconnects.
- Pairing this with the `htmx-sse` extension on the browser side.

## Run

```sh
cd examples/htmx-stream
go run .
```

Open <http://localhost:8080>. A random integer streams to the page every
100 ms. Close the tab and the
goroutine exits — context cancellation is the only stop signal.

## Event IDs

An application can attach an ID to an event:

```go
msg := hyperserve.NewSSEMessage("ready")
msg.Event = "status"
msg.ID = "42"
fmt.Fprint(w, msg)
```

This adds `id: 42` before the data lines. An empty `ID` omits the field and
leaves the client's last event ID unchanged. IDs containing CR, LF, NUL, or
invalid UTF-8 are omitted entirely; they are never rewritten into another ID.
To deliberately reset the client's last event ID, write `id:\n\n` directly to
the stream and flush it.

ID assignment, persistence, and replay from `Last-Event-ID` belong to the
application. This random-number example does not retain events for replay.
See the [SSE specification](https://html.spec.whatwg.org/multipage/server-sent-events.html#the-last-event-id-header)
for the browser's reconnection behavior.

## Notes

This example uses raw `text/event-stream` for direct HTMX consumption. MCP's
separate request-scoped SSE behavior is covered under
[resource subscriptions](../../docs/MCP_GUIDE.md#resource-subscriptions).
