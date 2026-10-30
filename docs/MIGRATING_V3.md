# Migrating to v3

Change the module requirement and every HyperServe import from
`github.com/osauer/hyperserve/v2` to `github.com/osauer/hyperserve/v3`.
The package names and ordinary `net/http` handler shapes remain the same.
Do not mix v2 and v3 server, middleware or protocol values in one integration.

## Behavior changes

- MCP and discovery use method-qualified registrations so HTTP `GET /` routes
  can coexist with them. MCP handles GET and POST at its configured endpoint;
  the selected transport still rejects methods it does not support. Discovery
  accepts GET; its handler rejects HEAD. Other methods follow ServeMux routing.
- Static mounts serve `index.html` for the mount root.
- `BindForm` parses multipart fields as documented. File parts remain available
  in `Request.MultipartForm`. The unparsed multipart body is capped at the same 10 MiB ceiling as
  standard form parsing; applications may impose smaller limits. net/http
  removes temporary files when the request finishes. Call `RemoveAll` yourself
  when using a request outside a real HTTP server.
- The stdio example creates missing sample files exclusively within its root.
  Existing files and symlinks are preserved.
- Example authentication and limits follow the resolved MCP endpoint.
- Generated containers include `configs/default.json`.

## Application migration check

Test the application handler through HyperServe, including its own access
middleware, a read-only MCP resource, authenticated streaming, cancellation,
and shutdown ordering. Keep builtins disabled unless deliberately required.
The server does not own your application goroutines or storage.

The proprietary routed-SSE transport remains opt-in and deprecated. Ordinary
HTTP SSE and current MCP Streamable HTTP are separate supported paths.

## Before publication

For local consumer validation, use a disposable module with a replacement to
this checkout. Generate it with `--local-replace /absolute/path/to/hyperserve`.
Remove the replacement and verify retrieval of the public v3 tag at release
time. A local build is not proof of publication. Never edit a live consumer's
checkout or its dependency pin as part of a library-only check.
