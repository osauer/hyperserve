# Migrating from /v2

Change the module requirement and every HyperServe import from
`github.com/osauer/hyperserve/v2` to `github.com/osauer/hyperserve`.
The next unsuffixed release is `v1.7.0`; old `/v2` releases remain unchanged.
The package names and ordinary `net/http` handler shapes remain the same.
Do not mix suffixed and unsuffixed server, middleware or protocol values in one
integration. They are different Go package identities. See the
[single-owner compatibility policy](API_STABILITY.md).

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
- Typed MCP schemas describe `time.Time` as a date-time string and fixed byte
  arrays as numeric arrays. `json.Number` remains numeric. Custom JSON
  representations remain unconstrained
  rather than advertising an incompatible object schema; use the explicit tool
  builder when clients need a precise custom shape.

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
Remove the replacement and verify retrieval of the public tag at release
time. A local build is not proof of publication. Never edit a live consumer's
checkout or its dependency pin as part of a library-only check.
