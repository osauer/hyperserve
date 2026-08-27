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

## Notes

This example uses raw `text/event-stream` for direct HTMX consumption. MCP's
separate request-scoped SSE behavior is covered under
[resource subscriptions](../../docs/MCP_GUIDE.md#resource-subscriptions).
