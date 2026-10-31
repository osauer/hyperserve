# Upgrading to v2.2.2

Keep `github.com/osauer/hyperserve/v2` imports and update the dependency to
`v2.2.2`. No public Go API is removed or renamed. The unpublished unsuffixed
and v3 proposals are superseded by [ADR-0019](0019-v2-maintenance.md).

## Compatibility details

`BindForm` retains v2 parsing behavior: it calls `Request.ParseForm`. For
multipart fields, set an application body limit and call `ParseMultipartForm`
first. Unparsed multipart bodies remain unread; already parsed fields and
files keep the caller's budget and standard-library precedence. When using
requests outside net/http serving, call `MultipartForm.RemoveAll` for cleanup.

MCP and discovery retain their methodless route registrations and protocol
method rejection, even with a methodless application catch-all. Explicit
application method routes retain ServeMux precedence. For a homepage alongside
MCP, use `app.GET("/{$}", handler)`, not the overlapping `GET /` subtree. Register
assets explicitly or use a methodless file-server fallback. HyperServe follows
net/http's pattern specificity and conflict rules.

## Corrections and example changes

- Static mounts serve `index.html` for the mount root.
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
