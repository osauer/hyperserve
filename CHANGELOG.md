# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-05-24 07:21 CEST

Release-line repair and production hardening. HyperServe is v1; this release
moves current `main` back onto the v1 train after the confusing historical
state where older `v1.0.x` tags existed while newer docs continued on
`v0.34.x`.

### Fixed

- **MCP resources are live by default.** `resources/read` no longer caches all
  resources for five minutes. Resources opt in by implementing
  `mcp.CacheableResource`; observability resources now return fresh health,
  metrics, logs, and route data.
- **Namespaced resource reads now echo the requested prefixed URI** in
  `ResourceContent.URI`, matching `resources/list`.
- **Route introspection now reads registered routes**, not middleware-only
  path bindings. `Server.RegisteredRoutes()` exposes a sorted snapshot for
  built-in observability.
- **JSON-RPC notifications no longer receive responses**, including over HTTP,
  SSE, and stdio transports.
- **WebSocket required-subprotocol checks happen before `101 Switching
  Protocols`**, and `Upgrade` now applies the supplied response headers.
- **Config files can override defaults to `false`, `0`, or `null`** because
  merging is now based on JSON field presence rather than Go zero values.
- **Typed MCP tools with pointer input types execute correctly** after decode
  and validation.
- **The auth example's development login now emits RS256 JWTs** that its own
  validator accepts.

### Changed

- Removed the leftover repo-owned `cmd/server` package. HyperServe is
  library-first; `cmd/hyperserve-init` remains the supported command.
- Removed checked-in Mach-O binaries from examples and benchmarks.
- `make build` / `make install` now target `cmd/hyperserve-init`.
- CI builds `cmd/hyperserve-init` and pins `staticcheck` / `govulncheck`
  install versions.
- The release-gated canonical examples are now `examples/devops`,
  `examples/mcp-sse`, and `examples/json-api`.

### Documentation

- Rewrote API stability, roadmap, and security docs around the v1 line.
- Updated MCP docs away from removed builder APIs and the old `/mcp/sse`
  endpoint shape.
- Modernized the JSON API example around method-aware routes and typed binding.
- Fixed scaffold templates so generated projects require HyperServe, blank
  import built-in MCP presets when needed, avoid process-wide env mutation, and
  use Go 1.26 in Dockerfiles.

## [0.34.2] - 2026-05-19

Docs-only patch release. Two cosmetic LOWs flagged in the v0.34 review
pass.

### Documentation

- **`PROJECT_STRUCTURE.md` lists `internal/validate/`** alongside
  `internal/scaffold/`. The validation core moved out of `pkg/server`
  in v0.31 and the structure doc never caught up. `internal/validate`
  is 318 LOC and backs `pkg/server.Validate` / `ValidationError` /
  `FieldError` via type aliases — worth surfacing.

- **`.github/workflows/claude*.yml` placeholder commands** updated
  from npm/Node defaults (left over from the upstream template) to
  Go-appropriate examples (`go test ./...`, `go vet ./...`,
  `make check`). The `custom_instructions` placeholder now points
  at CLAUDE.md and `make check` as the floor; the `claude_env`
  placeholder uses `GOFLAGS` instead of `NODE_ENV`. These are all
  commented-out placeholder examples — no workflow behaviour
  changes.

No code changes, no test changes. `make check` clean.

## [0.34.1] - 2026-05-19

Patch release. One correctness fix (middleware path-boundary) plus a
pure cohesion refactor (server.go split). Zero API changes, zero
behaviour changes outside the bug being fixed.

### Fixed

- **Middleware path-segment boundary** (`pkg/server/middleware.go`).
  `applyToMux` used `strings.HasPrefix(path, key)` to decide whether
  to wrap a request's handler with a route-specific middleware. That
  accepted `/api` as a prefix of `/api2/foo` — middleware registered
  for `/api` would fire on completely unrelated routes that happened
  to share the textual prefix (`/api2/foo`, `/apifoo`,
  `/apiserver`). New `pathPrefixMatches(path, key)` enforces a
  `/`-boundary check after `HasPrefix`, with explicit short-circuits
  for `key == ""` (legacy "apply to all" idiom, used by some tests)
  and trailing-slash keys (which already include the boundary).
  Three new regression tests guard the contract:
  `TestMiddlewarePathPrefixBoundary`,
  `TestMiddlewareRootPrefixMatches`,
  `TestMiddlewareEmptyKeyMatchesAll`. The first one fails on
  pre-v0.34.1 code on exactly the buggy paths.

### Refactored

- **`pkg/server/server.go` split** (1633 → 1390 LOC). Pure cut+paste,
  no behaviour change. `templates.go` (173 LOC) receives the
  template-rendering subsystem (`openTemplateRoot`,
  `HandleFuncDynamic`, `HandleTemplate`, `parseTemplates`,
  `listTemplateFiles`, `DataFunc`). `static.go` (85 LOC) receives the
  static-file-serving subsystem (`EnsureTrailingSlash`,
  `HandleStatic`, `rootFileServer`). `server.go` keeps the lifecycle,
  options pre-processing, mux routing helpers, deferred-init,
  shutdown, and the accessors. The split was easier to land before
  v1.0 froze the file layout than after.

`make check` clean; `go test -race ./...` green.

## [0.34.0] - 2026-05-19

**The actually-final breaking sweep before v1.0.** v0.33.0's release note
called itself "the final breaking sweep" — that turned out to be wrong.
A consolidated security + concurrency + taste review surfaced three
MEDIUM security findings, three HIGH concurrency bugs, and a small set
of API breaks worth taking before the surface freezes. v0.34.0 is the
surface that v1.0 freezes.

### Security

- **MCP help page XSS (MEDIUM)** (`pkg/mcp/handler.go`). `Handler.ServeHTTP`
  injected `r.URL.Path` unescaped into the HTML help template. Safe at
  the default exact-match endpoint `/mcp`, but the moment a user mounted
  the handler on a subtree pattern (`"/mcp/"`), the path carried
  attacker content. Now routed through `html.EscapeString` before
  `Fprintf`. The same template now also documents the required
  `X-SSE-Binding` header for routed POSTs (previous version only
  mentioned `X-SSE-Client-ID`, which would 403 on every click) and
  drops a phantom `/sse` subpath reference.

- **CORS footgun closed (MEDIUM)** (`pkg/server/middleware.go`). The
  static `securityHeaders` table unconditionally set
  `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`,
  `Access-Control-Allow-Credentials: true`, and
  `Access-Control-Max-Age` on every response — including responses
  from user handlers that legitimately echo `Origin` into
  `Access-Control-Allow-Origin`, producing the credentialed-wildcard
  combo browsers refuse. Those four headers now live exclusively in
  `applyCORSHeaders`, which honours `WithCORS`. Sensible defaults
  (`GET, POST, OPTIONS` and `Content-Type, Authorization`) are emitted
  there when CORS is configured but the user didn't enumerate methods
  or headers, so preflight responses still work without explicit
  enumeration.

- **WebSocket handshake conformance (LOW–MED)** (`pkg/websocket/handshake.go`).
  `ValidateHandshake` only checked `Sec-WebSocket-Key != ""`. RFC 6455
  §4.1 mandates the key be a base64-encoded 16-byte nonce — anything
  else is either accidental misconfiguration or a deliberate cache /
  proxy confusion attempt. New `ErrMalformedKey`; the existing test
  fixture `dGhlIHNhbXBsZSBub25jZQ==` decodes to 16 bytes and keeps
  passing.

### Fixed

- **WebSocket reader/writer race (HIGH)** (`pkg/websocket/conn.go`).
  The reader goroutine inside `lowConn.ReadMessage` wrote pong and
  close-echo frames straight to the `FrameWriter` the user's writer
  goroutine reaches via `WriteMessage` / `WriteControl`. No mutex →
  interleaved bytes from two frames on the wire, peer reset. New
  `writeMu sync.Mutex` on `lowConn`, held inside `WriteFrame`.

- **SSE write race (HIGH)** (`pkg/mcp/transport_sse.go`). The
  request-processing goroutine reached `client.writeSSEMessage` for
  the "ready" notification while the main loop was writing responses
  and pings — two writers, no serialisation, and `lastMessageID`
  racing too. Per-client `eventChan` (16-buffered) added; the
  goroutine `enqueueEvent`s instead of writing. The main loop's
  `select` is now the only writer.

- **WebSocket handler chain wired (HIGH)** (`pkg/websocket/websocket.go`,
  `pkg/websocket/conn.go`). `Conn.SetPingHandler` / `SetPongHandler` /
  `SetCloseHandler` shipped public but useless: `lowConn.ReadMessage`
  consumed ping/pong/close opcodes inline, so the outer `Conn.ReadMessage`
  switch was unreachable. Handler callbacks moved to `lowConn`,
  invoked from inside the wire reader. Default behaviour (auto-pong,
  auto-echo close) preserved when no user handler is set.
  `Conn.ReadMessage` is now a one-line delegate.

- **MCP error routing via sentinels** (`pkg/mcp/handler.go`,
  `pkg/mcp/transport_http.go`). `Handler.ServeHTTP` was deciding
  between 405, 400, and 500 by `strings.Contains(err.Error(), …)` over
  free-form messages from the same package's transport. A rename in
  `transport_http.go` would silently downgrade real 405/400 returns
  to 500. New exported `ErrMethodNotAllowed` and
  `ErrUnsupportedContentType` sentinels; transport wraps with `%w`;
  `ServeHTTP` switches on `errors.Is`.

### Changed — BREAKING

- **`Server.Handle` signature: `http.HandlerFunc` → `http.Handler`**
  (`pkg/server/server.go`). The docstring example
  (`srv.Handle("/static", http.FileServer(...))`) never compiled
  against the previous signature. With `http.Handler`, that example
  works and `Handle` becomes the natural sibling of `HandleFunc`.
  Migration: call sites passing a bare closure
  `func(w http.ResponseWriter, r *http.Request)` need to either wrap
  with `http.HandlerFunc(...)` or switch to `srv.HandleFunc`. Call
  sites already passing an `http.Handler` need no change.

- **`MiddlewareRegistry` unexported → `middlewareRegistry`**
  (`pkg/server/middleware.go`). Zero external callers (`grep` across
  the repo found none — including examples and tests in other
  packages). `NewMiddlewareRegistry` → `newMiddlewareRegistry`. The
  field on `Server` was already unexported, so this only affects code
  that directly named the type. Compose middleware via
  `Server.AddMiddlewareStack` instead.

- **`Server.RegisterMCPToolInNamespace` and
  `Server.RegisterMCPResourceInNamespace` removed** (`pkg/server/mcp.go`).
  Zero callers anywhere. Use `Server.RegisterMCPNamespace(name, NamespaceConfig)`
  — the documented path that handles a tool, a resource, or any
  combination in one call.

- **`ExtensionBuilder.WithConfiguration` removed** (`pkg/mcp/builders.go`).
  Zero callers. The builder's `configFunc` field and the corresponding
  `builtExtension` field are gone too; `(*builtExtension).Configure`
  is now a hard no-op. Custom configuration hooks were never used and
  the indirection cost more than it bought.

### Refactored

- **CSP literal de-duplication** (`pkg/server/middleware.go`). The two
  near-identical 340-character CSP strings (with and without web-worker
  support) were a drift risk on a security-critical header. One
  directive slice now, conditional `child-src`/`worker-src` appends,
  `strings.Join`.

### Documentation

- `examples/mcp-basic` rewritten to match its README and the index
  description ("smallest MCP server: enable, expose built-in
  tools/resources"). main.go had drifted into an SSE web demo with
  embedded JS. New version: built-in tools + resources, sandboxed
  file-tool root pointing at `examples/mcp-basic/sandbox/`, custom
  `TimestampTool`, custom `ServerStatusResource`, template-rendered
  dashboard via `HandleFuncDynamic`, rate-limited endpoint.
- `examples/mcp-sse/README.md` updated to point at the actual
  `-mode=server|client` shape (was `go run server.go` / `go run client.go`,
  which never existed). Documents both `X-SSE-Client-ID` and
  `X-SSE-Binding` headers as required for routed POSTs.
- `docs/ROADMAP.md`: header bumped past v0.33.1; maintainer-local
  `../regime` path leak removed from the one-click bundles section.

### Migration

| You had | Change to |
|---|---|
| `srv.Handle("/p", func(w, r){…})` | `srv.HandleFunc("/p", func(w, r){…})` or `srv.Handle("/p", http.HandlerFunc(func(w, r){…}))` |
| `srv.RegisterMCPToolInNamespace(tool, "ns")` | `srv.RegisterMCPNamespace("ns", mcp.NamespaceConfig{Tools: []mcp.Tool{tool}})` |
| `srv.RegisterMCPResourceInNamespace(res, "ns")` | `srv.RegisterMCPNamespace("ns", mcp.NamespaceConfig{Resources: []mcp.Resource{res}})` |
| `mcp.NewExtension("x").WithConfiguration(fn).Build()` | drop the `.WithConfiguration(fn)` call; if you need a hook, register tools/resources directly on the handler |
| `server.NewMiddlewareRegistry(stack)` (external) | unreachable; use `srv.AddMiddlewareStack(route, stack)` |

`make check` clean, `go test -race ./...` green.

## [0.33.1] - 2026-05-18

Patch release. Closes one Dependabot HIGH alert and the process gap
that let it through `make check`.

### Security

- **`examples/auth/go.mod`: `github.com/golang-jwt/jwt/v5` bumped
  `v5.2.1 → v5.2.2`** (GHSA-mh63-6h87-95cp — excessive memory
  allocation during JWT header parsing in versions `>= 5.0.0-rc.1,
  < 5.2.2`). Scope is the standalone auth example only; the main
  HyperServe library has no JWT dependency.

### Changed

- **`make check` now recurses into standalone example modules**
  (`Makefile`). New `check-examples` target discovers
  `examples/*/go.mod` via shell glob and runs
  `go vet ./... && go build ./... && govulncheck ./...` inside each,
  wired into `check:`. Closes the gap that let the JWT vuln above
  reach the default branch: examples with their own `go.mod` (via
  `replace`) live outside the main module's `./...`, so the
  pre-existing govulncheck pass never saw them. Discovery is
  glob-based — new standalone example modules are picked up
  automatically without Makefile edits.

## [0.33.0] - 2026-05-18

**Final breaking sweep before v1.0.** Closes the one MEDIUM security
finding from the post-v0.32.0 security review (cache poisoning on the
authenticated discovery policy), deletes the write-only MCP metrics
machinery, unexports the SSE state machine, drops `Get*` accessor
prefixes, removes the stale `spec/` directory, fixes the discovery
substring leak, and raises `pkg/mcp` test coverage from 35.8 % to
52.1 %. After v0.33.0 stabilises in use, v1.0.0 freezes the surface
and `API_STABILITY.md` gains teeth — no more breaking subtractions
in minors.

### Security

- **Discovery cache-poisoning fix** (`pkg/server/mcp.go`). Both
  `/.well-known/mcp.json` and `<MCPEndpoint>/discover` now emit
  `Vary: Authorization` on every response, and switch
  `Cache-Control` from `public, max-age=300` to `private, max-age=60`
  when `MCPDiscoveryPolicy == DiscoveryAuthenticated`. Under the
  prior shape the response body was content-negotiated on the
  `Authorization` header while the handler set `public` cache
  semantics; any CDN/reverse proxy keyed on URL alone would cache
  an authenticated response and replay it to anonymous clients
  within the 300-second TTL, defeating the discovery policy the
  operator opted into. `TestDiscoveryEndpointCacheVary` pins the
  contract across all four policies and both endpoints.

### Removed (breaking)

- **`pkg/mcp/metrics.go` deleted entirely.** 170 LOC of
  `*Metrics` struct, `durationStats`, `executionStats`,
  `recordRequest`, `recordToolExecution`, `recordResourceRead`,
  and `GetMetricsSummary` — recorded per-method/tool/resource
  execution stats on every JSON-RPC dispatch, but the single
  reader `Handler.GetMetrics` had exactly one caller (a unit
  test). Nothing in docs, examples, `MetricsResource`, or
  downstream ever consumed it. The deletion also removes an
  unguarded `float64(totalErrors) / float64(totalRequests)`
  that returned `NaN` before any request was recorded, which
  `json.Marshal` then errored on. If MCP-level observability is
  wanted later, plumb it through `MetricsResource`
  (`metrics://server/stats`), not a write-only mutex side channel.

- **SSE state-machine types unexported** (`pkg/mcp/transport_sse.go`).
  Zero out-of-package callers across this repo, examples, and tests:
  - `SSEClient` → `sseClient`
  - `SSEManager` → `sseManager`
  - `NewSSEManager` → `newSSEManager`
  Methods on the types remain capitalised — Go idiom keeps
  `Close`/`Send`/`IsReady`/`SetX` consistent, and methods on an
  unexported type are not visible in godoc anyway.

- **`Get*` accessor prefix dropped** across the public API
  (Effective Go: "Get prefix is neither idiomatic nor necessary"):
  - `Handler.GetRegisteredTools`     → `Handler.RegisteredTools`
  - `Handler.GetRegisteredResources` → `Handler.RegisteredResources`
  - `Handler.GetToolByName`          → `Handler.Tool(name) (Tool, bool)`
  - `Engine.GetRegisteredMethods`    → `Engine.RegisteredMethods`
  - `server.GetVersionInfo`          → `server.VersionInfo`
  - `sseManager.GetClientCount`      → `sseManager.ClientCount`
  The rest of the framework already followed `ToolCount`/`HasTool`
  style; these were the inconsistent outliers.

- **`spec/` directory deleted.** Three markdown files (`README.md`,
  `api.md`, `mcp-protocol.md`) that specced a Rust+Go dual
  implementation removed in commit `cc135e9` (Sep 2025). `spec/api.md`
  still listed `HEALTH_ADDR :9080` against the current Go default of
  `:8081`. `README.md`'s "API specification" link now points at
  `pkg.go.dev`.

### Changed

- **Discovery substring sniffing removed** (`pkg/mcp/discovery.go`).
  The protocol package no longer pattern-matches tool names against
  `"debug"`, `"admin"`, or `"server_control"` to hide them from the
  discovery payload. That was a leaky guess at user intent — a
  legitimate user tool named `tax_admin_lookup` got silently hidden —
  and a dependency-direction inversion: `pkg/mcp` was reaching down
  into `pkg/mcp/builtin` domain knowledge. Tools opt out of discovery
  by implementing `interface{ IsDiscoverable() bool }`; the
  `internal_` / `_` prefix convention still applies. Built-in
  dev-only tools (`server_control`, `route_inspector`, `dev_guide`)
  are gated at registration time, not at discovery time, so they
  are unaffected when running outside developer mode.

- **`docs/API_STABILITY.md`** — documents v0.33.0 as the final
  breaking sweep and adds the "no breaking subtractions in 1.x
  minors" contract that v1.0.0 introduces.

- **`docs/ROADMAP.md`** — top item is now the v1.0 freeze plan; the
  `hyperserve init` line was struck (already shipped). Timestamps
  on both ROADMAP and API_STABILITY normalised to the
  `YYYY-MM-DD HH:MM TZ` form mandated by the project's contributor
  guide.

### Added

- **`pkg/server/mcp_discovery_test.go`** — pins the cache
  `Vary`/`Cache-Control` contract across all four discovery
  policies and both endpoints (8 cases).

- **`pkg/mcp/discovery_test.go`** — covers `BuildDiscoveryInfo`
  basics, `X-Forwarded-Proto` scheme detection,
  `StdioTransport` capability inclusion,
  `shouldIncludeToolList` for every policy,
  `shouldExposeToolInDiscovery` for the
  `IsDiscoverable`/internal-prefix/custom-Filter branches, and
  the regression that names containing "admin"/"debug" are no
  longer hidden.

- **`pkg/mcp/handler_test.go`** — covers `RegisterTool` (incl.
  namespace prefix + `cmp.Or` fallback), `RegisterResource` (incl.
  empty-namespace rejection), `RegisterNamespace` via
  `WithNamespaceTools/Resources` (incl. empty-name error),
  `Capabilities`, `ServeHTTP` GET-with-JSON-Accept, `ServeHTTP`
  POST `tools/call` dispatch, the JSON-RPC error envelope for
  unknown methods, and `isJSONAccepted` parsing variants.

- `pkg/mcp` package coverage: **35.8 % → 52.1 %**.

## [0.32.0] - 2026-05-18

Stabilisation sweep. Fixes five HIGH bugs surfaced by a senior taste
review, removes dead surface, splits options out of `server.go`, makes
the per-request middleware chain non-allocating in the steady state,
and brings the docs back in sync with the code. Pre-1.0 means breaking
subtractions are allowed in minor releases — call sites for the
removed types are listed below.

### Fixed

- **HSTS now sent only over TLS, with one consistent value**
  (`pkg/server/middleware.go`). The previous shape unconditionally set
  `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload`
  in the static security-headers table, and then *overwrote* it with
  `max-age=63072000; includeSubDomains` (no `preload`) when
  `EnableTLS`. Plaintext responses shipped the preload variant; TLS
  responses shipped the supposedly-stronger variant without it. The
  fix collapses to one TLS-gated write of
  `max-age=63072000; includeSubDomains; preload`, and a new
  `TestHSTSOnlyOverTLS` pins the contract.

- **`logServerMetrics` no longer always reports zero** (`pkg/server/server.go`).
  The prior formula `totalRequests / totalResponseTime_µs` collapsed
  to 0 for any realistic workload because the denominator is 1000–1M×
  the numerator. The log line now reports `avg-µs-per-req` =
  `totalResponseTime / totalRequests`, with the same renaming applied
  to the log key.

- **WebSocket upgrades no longer double-count `totalRequests`**
  (`pkg/server/server.go`). `MetricsMiddleware` already increments
  the counter for every request that reaches the handler; the
  `WebSocketUpgrader.BeforeUpgrade` hook was bumping it again. The hook
  now only updates the WS-specific `totalWebSocketUpgrades` counter,
  and `TestWebSocketTelemetry` asserts exact-by-1.

- **`handleToolsCall` no longer silently drops a `json.Marshal` error**
  (`pkg/mcp/handler.go`). The result-normalisation switch is extracted
  to a `toToolContent` + `marshalAsText` helper pair. The prior shape
  had a sub-branch (`existingContent.([]any)` with a non-map element)
  that used `_ := json.Marshal(v)` and overwrote partial results
  with a single text frame, leaving consumers no signal that something
  went wrong.

- **`initHealthServer` no longer races a 100 ms timer** (`pkg/server/server.go`).
  The bind step is now synchronous via `net.Listen` and the goroutine
  only runs `Serve(ln)`. `EADDRINUSE` (and friends) surface immediately
  as the function's return value, matching the main HTTP server's
  pattern.

### Removed (breaking)

- **`mcp.SimpleResource` + `mcp.ResourceBuilder` + `mcp.NewResource` +
  `(*ResourceBuilder).WithName/WithMimeType/WithRead/Build`** — deleted
  entirely from `pkg/mcp/builders.go`. ~140 LOC with zero callers in
  this repo, in `examples/`, or in any test. The MCP resource path was
  unused public surface; the tool builder (`mcp.NewTool().WithParameter(...)`)
  remains kept-on-purpose alongside `mcp.NewTypedTool`.

- **`mcp.SimpleTool` unexported to `simpleTool`**. The function-field
  Tool implementation was only reachable via `ToolBuilder.Build()` —
  nobody constructed it directly. Direct struct literal users (none
  detected in the wild) need to switch to the builder.

- **`server.TraceMiddleware` + `generateTraceID` + `requestCounter`**
  — deleted. The middleware was never registered in `DefaultMiddleware`,
  `SecureAPI`, or `SecureWeb`, and the `traceID` field it would have
  populated in `RequestLoggerMiddleware` was empty in 100% of real
  deployments. The `trace_id` field is also removed from the request
  log line; bring your own correlation ID middleware if needed.

- **`server.ServerOptions.TLSHealthAddr`** field deleted. Defaulted to
  `:9443` since v0.x but never read anywhere — no `WithTLSHealthAddr`
  option, no env binding, no use in health server startup.

### Changed

- **17 `With*` option closures moved from `pkg/server/server.go` to
  `pkg/server/options.go`**. The closures wrap `ServerOptions` and
  belong next to it, mirroring the convention already enforced for
  MCP options (`options_mcp.go`, `options_mcp_discovery.go`). No
  behaviour change; `server.go` drops from 1858 to 1631 LOC.

- **`MiddlewareRegistry` now precomputes the route ordering at
  `Add()` time** (`pkg/server/middleware.go`). The per-request hot
  path no longer allocates a route-key slice, no longer calls
  `sort.Slice`, and no longer builds an intermediate "applicable
  middleware" slice. The middleware-closure allocations intrinsic to
  the design remain — every other allocation site is gone.

### Documentation

- **`docs/MCP_GUIDE.md`** SSE flow taught a request shape that
  returns 403 today (only `X-SSE-Client-ID`, missing `X-SSE-Binding`).
  Rewritten to cover the binding-token capability, both shell and
  JavaScript samples.

- **`docs/API_STABILITY.md`** rewritten. The prior doc claimed
  "v0.9.x" and locked signatures (`AddMiddleware(...)` variadic,
  `MCPTool`/`MCPResource` interfaces) the code no longer has. The
  new shape: promise is process not signatures until v1.0.0 —
  CHANGELOG accuracy, breaking-change call-outs, `make check` as the
  floor.

- **`docs/ROADMAP.md`** version stamp moved from v0.27.0 to v0.32.0.

- **`docs/EXAMPLES_GUIDE.md`** and **`docs/mcp-sse.md`** deleted.
  Unreferenced from anywhere; the first duplicated `examples/README.md`
  and the second taught a pre-binding-token SSE flow.

- **22 example directories tracked** for the first time. The
  `.gitignore` allow-list (`!examples/**/`, `!examples/**/*.go`, etc.)
  was already wired, but the directories had never been committed.
  CI can now vet what `README.md` links to.

- **5 missing READMEs added** (`binding`, `deferred-init`, `devops`,
  `htmx-dynamic`, `htmx-stream`) so every example matches the cohort.

- **`docs/posts/hyperserve-vs-gin.svg`** moved from repo root into
  `docs/posts/` next to the post it accompanies, and the post now
  references it inline.

- **Go reference badge in `README.md`** now links to the module root
  (`pkg.go.dev/github.com/osauer/hyperserve`) so the sub-package
  navigation is the landing experience.

## [0.31.0] - 2026-05-18

Feature release. Adds `mcp.NewTypedTool[In, Out]` — typed MCP tool
registration with reflection-derived `inputSchema` *and* `outputSchema`,
the same `validate:"..."` rules used by `BindJSON`, and a panic-free
handler body. Backwards-compatible — `mcp.NewTool().WithParameter(...)`
keeps working and is the right tool when you need a hand-tuned schema.

### Added

- **`mcp.NewTypedTool[In, Out](name, description, fn) mcp.Tool`**
  (`pkg/mcp/typed_tool.go`). Wraps a typed handler `func(ctx, In) (Out, error)`
  where `In` is a struct. The framework derives `inputSchema` from `In`
  via reflection — field names from `json:"…"`, types from Go types,
  `required`/`oneof`/`min`/`max`/`len` from `validate:"…"`, descriptions
  from `mcp:"desc=…"`. Each call JSON-decodes arguments into a fresh
  `In`, runs `validate.Struct`, then invokes `fn`. Type inference picks
  both type parameters off the function value, so call sites don't write
  them explicitly:

  ```go
  type CreatePostArgs struct {
      Title  string   `json:"title"  validate:"required,max=200"`
      Author string   `json:"author" validate:"required"`
      Tags   []string `json:"tags,omitempty" validate:"max=10"`
  }
  type Post struct { ID, Title, Author string; Tags []string; CreatedAt time.Time }

  srv.RegisterMCPTool(mcp.NewTypedTool(
      "create_post", "Create a new blog post.", blog.Create))
  // blog.Create: func(ctx context.Context, args CreatePostArgs) (Post, error)
  ```

  Validation failures surface through the JSON-RPC tool-call error with
  the same per-field format produced by `server.BindJSON`
  (`"validation failed: field: rule message; …"`). The format is pinned
  by `TestNewTypedTool_ValidationErrorMessageFormat` so MCP clients can
  rely on it. `struct{}` works on either side for no-args / no-payload
  tools and suppresses `outputSchema`.

- **`outputSchema` on `tools/list`**. New `ToolInfo.OutputSchema`
  (`pkg/mcp/types.go`) carries the field added in the MCP spec revision
  2025-06-18. Typed tools implement the new `ToolWithOutputSchema`
  interface; the handler type-asserts and emits the schema only when
  present, so builder-based tools stay unchanged on the wire.

- **`internal/validate`**. Extracted the tag-driven struct validator out
  of `pkg/server/binding.go` so `pkg/mcp` can reuse it without an import
  cycle. `pkg/server.Validate` / `ValidationError` / `FieldError` are
  preserved as type aliases — no source-level break for existing callers.

- **`examples/mcp-extensions/`**. Rewritten as `create_post` / `get_post`
  / `list_posts` / `delete_post` (typed verbs, each with a tight args
  struct, no `action` enum or every-field-optional shape) plus a
  `search_posts` builder tool kept for contrast. Each typed tool's
  return type drives an `outputSchema` visible in `tools/list`.

### Changed

- **`docs/MCP_GUIDE.md`** now leads with the typed-tool section, with a
  validate-verb → JSON-Schema mapping table, the one-tool-per-verb
  guidance, and pointers to the example and the validation-format pin
  test. The builder section is preserved for hand-tuned schemas.

- **`pkg/server/binding.go`** shrinks to bind helpers + thin re-exports.
  Validation core moved to `internal/validate`; no behavior change.

## [0.30.0] - 2026-05-18

Feature release. Adds `server.JSONEcho[T]()` — the natural sibling to
v0.28.0's `JSONHandler` for the case where the response shape is the same
as the validated input. Backwards-compatible — `JSONHandler` is unchanged.

### Added

- **`server.JSONEcho[T]()`** (`pkg/server/typed_handler.go`). Shorthand
  for the validate-and-pass-through case: bind the body into `T`, run
  validation, echo the validated value back as the 200 response.

  ```go
  srv.POST("/webhook", server.JSONEcho[Event]())
  ```

  Useful for webhook acks, dev stubs, and "did this payload validate?"
  endpoints. Reach for `JSONHandler[In, Out]` when the response is
  genuinely different from the input (assigning a server-side ID,
  lowercasing the email, joining a related record). An identity function
  is the absence of business logic; `JSONEcho` says so directly.

  Implementation is a one-liner over `JSONHandler` — same bind path,
  same per-field 400 validation envelope, same error model.

### Changed

- **`examples/binding/`** now demonstrates three endpoints side-by-side:
  `/users/echo` (`JSONEcho[CreateUser]()`), `/users` (`JSONHandler` with
  a genuine mapping — assigns ID, lowercases email), and `/users-manual`
  (low-level `BindJSON`). The contrast lets readers pick the right tool.

## [0.29.0] - 2026-05-18

Feature release. Adds method-aware route helpers so handlers no longer
have to switch on `r.Method`. Backwards-compatible — `HandleFunc` is
untouched and remains the lower-level escape hatch for one handler
covering all methods.

### Added

- **`srv.GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`**
  (`pkg/server/server.go`). Thin wrappers that prepend the method to the
  pattern and delegate to `HandleFunc`, exposing the stdlib 1.22+
  `"METHOD /path"` syntax under a method-keyed API:

  ```go
  srv.GET("/users/{id}", getUser)
  srv.POST("/users", server.JSONHandler(createUser))
  srv.PUT("/users/{id}", server.JSONHandler(updateUser))
  ```

  Wrong-method requests get an automatic 405 from the mux with a
  populated `Allow` header. Path wildcards work as usual via
  `r.PathValue`. The helpers pick up the same middleware chain as
  `HandleFunc` since they call the same registration path.

### Changed

- **`examples/binding/`** now registers routes via `srv.POST` instead of
  `srv.HandleFunc("POST /users", …)`. Combined with `JSONHandler`, a
  CRUD endpoint collapses to: business function + one route line.

## [0.28.0] - 2026-05-18

Feature release. Adds a typed JSON handler wrapper that absorbs the
bind + validate + respond boilerplate every JSON endpoint otherwise writes
by hand. Backwards-compatible — the low-level `BindJSON` / `Validate` path
is unchanged and remains the escape hatch.

### Added

- **`server.JSONHandler[In, Out]`** (`pkg/server/typed_handler.go`). A
  generic `http.HandlerFunc` wrapper around a typed business function:

  ```go
  srv.HandleFunc("POST /users", server.JSONHandler(
      func(ctx context.Context, in CreateUser) (User, error) {
          return createUser(ctx, in)
      },
  ))
  ```

  Decodes the body with `server.BindJSON`, runs `validate:"..."` rules,
  calls the function with `r.Context()`, then JSON-encodes the result
  with 200. The 25-line per-endpoint shape collapses to one line of
  business logic.

  Error mapping:
  - `*server.ValidationError` → 400 with per-field envelope
    (`{error, fields: [{field, tag, param, message}]}`); `FieldError.Value`
    is intentionally omitted so handlers can't leak the offending field
    (passwords, tokens) back to callers.
  - errors implementing `HTTPStatus() int` → that status with
    `{"error": err.Error()}`.
  - everything else → 500 with `{"error":"internal server error"}`; the
    original error string stays in the server log, not the client.

  204 No Content is written when `Out` is `struct{}` or when the returned
  value is a nil pointer / nil interface.

- **`server.StatusError` + `server.NewStatusError`**. Sentinel error type
  carrying an HTTP status code so handlers can opt into a non-500
  response without inventing a per-call-site error type. Implements
  `HTTPStatus() int`, `Unwrap() error`, and `Error() string` with
  message → wrapped-err → `http.StatusText` fallback. Any user-defined
  error implementing the same interface works too — `StatusError` is
  the convenience, not the contract.

- **`examples/binding/`**. Side-by-side example: `POST /users` uses
  `JSONHandler`, `POST /users-manual` shows the equivalent hand-rolled
  `BindJSON` + `errors.As(&verr)` path with a `writeValidationError`
  renderer that mirrors the framework envelope. Surfaces what
  `JSONHandler` normally hides.

### Fixed

- **`.gitignore` re-includes under `examples/`.** The prior allow-list
  pattern (`examples/**/*` + per-extension negations) silently excluded
  every subdirectory under `examples/`, which in turn prevented any
  `.go` file under those subdirectories from being tracked — git refuses
  to re-include a file whose parent directory is excluded. Added
  `!examples/**/` so the per-extension negations take effect as
  intended. This is the reason `examples/binding/` (and likely other
  examples referenced in `examples/README.md`) was never committable.

### Documentation

- **README "Request binding & validation" section rewritten** to lead
  with `JSONHandler` as the high-level shortcut and keep the
  `BindJSON`-level API as the explicit escape hatch for streaming,
  custom headers, or non-JSON responses. Entry-points list extended.

## [0.27.1] - 2026-05-18

Docs and project-plumbing release. No code changes — bus-factor de-risking
moves the maintainer can do in one sitting, taken before they need to be
done in a hurry.

### Added

- **`SECURITY.md`.** Single-paragraph reporting policy: email
  `oliver.sauer@gmail.com`, 7-day acknowledgment, 90-day disclosure.
  Closes the gap of having no documented channel for the next vulnerability
  class after v0.27.0's seven-fix sweep.
- **`docs/ROADMAP.md`** (renamed from `docs/PRODUCT_VISION.md`, content
  preserved). Surfaced from the README's Documentation section. The
  roadmap was already written; it was just buried.
- **`.github/FUNDING.yml`.** GitHub Sponsors entry pointing at `osauer`.
  Signals intent without expectation; renders no UI until enrollment.
- **GitHub Discussions enabled** on the repo. Non-issue venue for
  "is this the right fit for X?" questions that precede issues and PRs.
  Linked from `README.md` and `CONTRIBUTING.md`.

### Changed

- **`CONTRIBUTING.md` rewritten to match what CI actually enforces.** The
  prior version told contributors to run `go fmt`/`go vet`/`go test`; the
  real gate is `make check` = gofmt + vet + staticcheck + govulncheck +
  modernize. Now documents the gate, the per-tool catch surface, the
  setup commands for `staticcheck`/`govulncheck`, a code map keyed to
  load-bearing files (`pkg/server/`, `pkg/mcp/`, `pkg/mcp/transport_sse.go`,
  …), and pointers to the relevant ADRs.

## [0.27.0] - 2026-05-18

Security + feature release. Closes seven concrete vulnerability classes in
the built-in MCP surface and ships request binding & validation as a
first-class API. Net change: **−1,012 LOC** across 44 files. Breaking — see
Migration.

### Added

- **Request binding & validation** (`pkg/server/binding.go`). New API for
  parsing JSON / form / query bodies into typed structs with struct-tag
  rules — no external dependencies.
  - `server.Bind(r, dst)` — Content-Type-aware dispatch.
  - `server.BindJSON(r, dst)` — `DisallowUnknownFields`, 1 MiB body cap.
  - `server.BindQuery(r, dst)`, `server.BindForm(r, dst)` — same struct,
    different decoder; slices populate from repeated keys.
  - `server.Validate(dst)` — run rules without binding.
  - Tags: `required`, `min=N`, `max=N`, `len=N`, `email`, `url`,
    `oneof=A B C`. Composable (`validate:"required,min=3,max=64"`).
  - `*server.ValidationError` carries one `*FieldError` per failing rule
    (`Field`, `Tag`, `Param`, `Value`, `Message`) so 400 responses can be
    structured per-field. See [examples/binding](examples/binding/).
- **SSE binding tokens.** The unified `/mcp` endpoint's SSE connection
  event now carries a `bindingToken` alongside `clientId`. Routed POSTs
  must echo it back via `X-SSE-Binding` or receive `403 Forbidden` —
  closes the cross-client request-injection class. Token is 32 random
  hex characters from `crypto/rand`; comparison is constant-time.
- **`WithMCPToolCallTimeout(d)`** — functional option for the per-tool
  execution budget enforced by the MCP handler. Default 30s. Replaces
  the previously-hardcoded literal.
- **Fuzz tests** for `pkg/jsonrpc` (request parser), `pkg/websocket`
  (frame reader), `pkg/server` (CORS origin matcher, email validator).
  - New `make fuzz-smoke` target runs each for 15s; CI runs it on
    every push and gates on failure (no `|| true` suppression).
- **`make test-race`** target wires the race detector into CI. The
  rate-limiter cleanup tick had a latent close-on-closed-channel race
  on shutdown convergence; fixed via `sync.Once`.

### Security

- **`http_request` built-in MCP tool removed.** Allowed any MCP caller to
  make outbound HTTP requests from the server process (SSRF / cloud-metadata
  exfiltration). No replacement; ship a domain-allowlisted tool from your
  own code if you need this primitive.
- **`request_debugger` built-in MCP tool removed,** along with
  `RequestCaptureMiddleware`. The tool stored `r.Header` verbatim
  (including `Authorization`, `Cookie`, `X-API-Key`) in a process-wide
  store readable by any MCP caller — a credential-exfiltration path
  enabled by `MCPDev()`. The `dev_guide` topic strings and ADR-0011 docs
  no longer reference either tool.
- **File tools require a sandbox root.** `NewFileReadTool("")` and
  `NewListDirectoryTool("")` now return an error; the unsandboxed
  `os.ReadFile` / `os.ReadDir` fallback was deleted. Builtin registration
  skips both tools with a warn-log when `WithMCPFileToolRoot` is unset.
- **SSE client IDs sourced from `crypto/rand`.** Previously `math/rand`,
  whose state can be recovered from observed outputs. New IDs are 32
  random hex characters; binding tokens are 64.
- **CORS refuses `AllowedOrigins=["*"] + AllowCredentials=true`.**
  `normalizeCORSOptions` downgrades the combination at construction time
  with a warn-log, matching the Fetch spec.
- **`examples/auth`: substring-admin bypass closed.** Permission gating
  in the auth example previously evaluated `!strings.Contains(authHeader, "admin")`
  — any token containing the substring `admin` granted access. The new
  flow uses `requireRole` / `requirePermission` sourced from the validator's
  `SessionInfo`. A `multiAuthMiddleware` wrapper accepts Bearer, APIKey,
  and Basic schemes (previous flow only accepted Bearer at the framework
  middleware layer, contradicting the example's own setup).

### Changed

- **SSE state machine consolidated** onto `SSEManager`. `Handler` no
  longer maintains a parallel `sseRequests` / `sseMutex` pair; the
  per-client request channels live alongside the client map under one
  lock. `Handler.RegisterSSEClient` / `UnregisterSSEClient` /
  `SendSSENotification` removed (the third had zero callers).
- **`handleShutdown` rewritten.** The previous `for { select }`
  could only iterate once and copy-pasted the same five state mutations
  into each case. Now top-down with named helpers `shutdownAfter` and
  `handleServerExit`; each select arm is a single line.
- **`handleResourcesRead` deduped.** The "`arguments` is a tools/call
  parameter, not a resources/read parameter" guard ran twice; now once
  before the unmarshal.
- **`bytesWritten` is now in the request log line.** The accumulator was
  tracked but never logged — the doc on `RequestLoggerMiddleware` claimed
  to log "response size in bytes" but didn't. Now it does.
- **CLAUDE.md rewritten.** The previous file claimed library files lived
  at the repository root — three releases stale.

### Removed

- Exported orphans, all with zero callers across the tree:
  - `mcp.OverHTTP`, `mcp.OverSSE` — HTTP is the default; SSE shares the
    same endpoint via Accept-header routing.
  - `mcp.Capabilities.Experimental` field — never assigned; violated the
    struct's own "advertise only what's wired" comment.
  - `mcp.DefaultLogger`, `mcp.SetDefaultLogger`, `builtin.SetDefaultLogger`
    — `server.DefaultLogger` / `SetDefaultLogger` was the only used pair.
  - `WithLoglevel`, `WithBannerColor`, `WithReadTimeout`,
    `WithWriteTimeout`, `WithIdleTimeout`, `WithReadHeaderTimeout` —
    each had zero non-test callers. Use `WithDebugMode` /
    `WithTimeouts(read, write, idle)` instead.
  - `MiddlewareRegistry.RemoveStack` — no caller.
  - `HealthCheckHandler`, `PanicHandler` — exported test-only helpers
    that didn't belong in the public API. Moved to `handlers_test.go`.
  - `websocket.Upgrader.{EnableCompression, WriteBufferSize, ReadBufferSize}`
    — silent no-ops; never read by `Upgrade`.
- Unreachable code:
  - `DiscoveryNone` and `DiscoveryCount` switch cases in
    `shouldExposeToolInDiscovery` were dead branches (the policies
    are already gated by `shouldIncludeToolList`).
- Speculative / stale artifacts:
  - `docs/LESSONS_LEARNED.md` — one-author retrospective with the AI
    sign-off intact after a prior cleanup commit claimed to remove it.
  - `docs/BUNDLE_EXPLORATION.md` — proposed a `hyperserve bundle`
    command that doesn't exist in code.
  - `configs/` — three files (GitHub OpenAPI spec, HTMX attribute list,
    Qodana config) that nothing in the codebase referenced.
  - `docs/guides/` — empty for 10 months.
  - `.gocache/` — 258-entry stale GOCACHE-style directory in the repo
    root, untouched in 8 months.

### Fixed

- **Rate-limiter cleanup race.** Convergent shutdown paths could
  call `stopCleanup` more than once, double-closing the `cleanupDone`
  channel. `sync.Once` makes it idempotent without writing to the
  fields the unlocked cleanup goroutine reads from.
- **MCP tool-call timeout was hardcoded** at 30s in
  `handleToolsCall`. Now configurable; the wrapper's leaked-goroutine
  caveat is documented honestly rather than hidden.

### Migration

- Rename `mcp.OverHTTP(endpoint)` and `mcp.OverSSE(endpoint)` call sites
  to `server.WithMCPEndpoint(endpoint)`. The default transport is HTTP
  and SSE shares the same endpoint; the `Over*` constructors had no
  callers.
- Replace `server.WithReadTimeout(d)` / `WithWriteTimeout(d)` /
  `WithIdleTimeout(d)` / `WithReadHeaderTimeout(d)` with the single
  `server.WithTimeouts(read, write, idle)`.
- If you have an SSE client that posts to the routed-POST endpoint,
  read `bindingToken` from the initial `connection` event and pass it
  as `X-SSE-Binding` on every POST. Missing or wrong → 403.
- If you previously called `srv.MCPHandler()` to register tools via the
  removed `RequestDebuggerTool` or `HTTPRequestTool` constructors,
  those types are gone. Calculator, file tools (sandboxed), server
  control, route inspector, and dev guide remain.
- If you constructed `FileReadTool` / `ListDirectoryTool` with an empty
  string, you'll now get an error. Configure `WithMCPFileToolRoot` to
  the directory you want the tools to be confined to.

### Verification

- `make check` (gofmt + vet + staticcheck + govulncheck + modernize):
  zero-drift on Go 1.26.
- `go test ./...`: green.
- `go test -race ./...`: green.
- `make fuzz-smoke`: four targets × 15s, all PASS, 6M+ execs total.
- Built `cmd/server` smoke-tested: `/.well-known/mcp.json` (calculator
  only), `tools/call` calculator (5 for 2+3), SSE initial event carries
  `bindingToken`, routed POST without binding → 403.
- `examples/auth` smoke-tested: admin-only routes return 200 with admin
  key, 403 with user key, 401 with any "admin"-substring forged token.
- `examples/binding` smoke-tested: 200 for valid, 400 with structured
  `fields[]` for each failing rule.

## [0.26.1] - 2026-05-18

Small dev-affordance increment on top of v0.26.0. No API or wire-shape changes.

### Added
- `cmd/server -version` (and `--version`) prints the ldflag-stamped version,
  build hash, and build time, then exits — matches the version info already
  exposed via `server.GetVersionInfo()`.
- README badges: CI status, latest release, Go version (from `go.mod`),
  pkg.go.dev reference, MIT license. Sourced from shields.io; pure metadata,
  no extra dependencies.

## [0.26.0] - 2026-05-18

Taste-review sweep. Targets dead surface, half-wired features, and doc drift
that survived the 0.25.x cleanups. Net change: **−2,479 LOC** across 38 files
plus a removed 9.6 MB tracked binary. Breaking — see Migration.

### Added
- `pkg/server/options_mcp.go` — five MCP option constructors (`WithMCPSupport`,
  `WithMCPEndpoint`, `WithMCPFileToolRoot`, `WithMCPBuiltinTools`,
  `WithMCPBuiltinResources`) moved out of `server.go` so the MCP glue lives in
  one place.
- Warn-log when `WithMCPBuiltinTools(true)` / `WithMCPBuiltinResources(true)`
  is set but the `pkg/mcp/builtin` blank-import is missing, with the exact
  import line to fix it. Closes the silent half-wire that previously made
  `cmd/server -mcp` advertise tools and register none.
- `cmd/server/main.go` blank-imports `pkg/mcp/builtin` so the bundled binary
  actually serves the built-in tools/resources it advertises.

### Changed
- Minimum Go version bumped to **1.26** (was 1.25). `go.mod`, CI workflows,
  and `internal/scaffold/templates/go.mod.tmpl` all aligned. See
  [ADR-0006](docs/0006-go-minimum-version.md).
- `pkg/mcp.Handler` now builds `tools/list`, `resources/list`,
  `resources/read`, and `tools/call` responses from the exported `ToolInfo`,
  `ResourceInfo`, `ResourceContent`, `ToolResult` types instead of
  `map[string]any` literals. Wire shape is unchanged; in-process callers
  using `ProcessRequestDirect` see typed structs instead of maps.
- `ToolResult` gained an `IsError bool` field (`omitempty`) so error
  responses round-trip through the type.
- `MiddlewareRegistry.applyToMux` sorts route keys deterministically
  (ascending length, ties alphabetical) before chaining; map-iteration
  randomness no longer leaks into middleware order when multiple prefixes
  match a request.
- `applyEnvVars` is now table-driven (`defaultEnvBindings` + `parseEnvBool`
  helpers); ~70 LOC shorter and a new env var is one entry, not nine
  branches.
- `NewServer` split into five named helpers (`newServerSkeleton`,
  `applyConfiguredLogLevel`, `autoConfigureMCPFromEnv`, `openTemplateRoot`,
  `initializeMCPHandler`); the top-level body is now ~30 LOC of
  orchestration.
- `rateLimiterEntry.lastAccess` is now `lastAccessUnixNano atomic.Int64`;
  hot-path requests bump the timestamp without re-acquiring the
  rate-limiter pool's write lock.
- `Server.websocketConnections` renamed to `totalWebSocketUpgrades`; the
  metrics-log key changed from `websocket-connections` to
  `websocket-upgrades-total` to reflect that it counts lifetime upgrades,
  not concurrent sessions.
- `WithFIPSMode` doc-comment scoped honestly: "FIPS-approved TLS cipher
  suites + curves" (not "FIPS 140-3 compliance"). The runtime log line
  matches.

### Removed (breaking)
- `pkg/server/interceptor.go` and its tests (`InterceptorChain`,
  `Interceptor`, `InterceptableRequest`, `InterceptableResponse`,
  `InterceptorResponse`, `AuthTokenInjector`, `RateLimitInterceptor`,
  `RequestLogger`, `ResponseTransformer`). Parallel pipeline beside
  `MiddlewareRegistry` with zero non-example callers; `pkg/server/middleware.go`
  provides equivalent middleware. `examples/interceptors/` removed.
- `pkg/websocket/websocket_pool.go` and its test (`WebSocketPool`,
  `pooledConn`). `Get(ctx, endpoint, upgrader, w, r)` returned a connection
  bound to a previous request's socket without ever upgrading the current
  request — broken-by-design for server-side WebSocket. `examples/websocket-pool/`
  removed.
- `WithOutStack` server method + `MiddlewareRegistry.exclude` field +
  `filterMiddleware` function. Exported orphan method (zero callers) that
  dragged a reflect-based comparison loop along.
- `WithLogger` server option. Mutated a package-global without a mutex.
  Use `pkg/server.SetDefaultLogger(l)` (or `pkg/mcp.SetDefaultLogger`)
  instead.
- `WithMCPNamespace` server option (zero callers). Use
  `srv.RegisterMCPNamespace(name, configs...)` post-construction.
- `ResponseTimeMiddleware` (existed only as the prohibited pattern in one
  log-test assertion).
- `FileServer` middleware-stack constructor — verbatim copy of `SecureWeb`.
  Name also collided with `http.FileServer`.
- `Options.DeferredInit` field (`json:"-"`) and its NewServer
  reconciliation. `WithDeferredInit` now writes only to
  `srv.deferred.init`.
- `pkg/mcp.LoggingCapability`, `PromptsCapability`, `SamplingCapability` —
  empty structs never instantiated. The MCP `Capabilities` struct keeps
  only `Resources`, `Tools`, `SSE`, `Experimental`.
- Three `t.Skip("requires full connection")` stub tests in
  `pkg/server/websocket_handlers_test.go`.
- `cmd/example-server/` — 75-line near-duplicate of `cmd/server` with no
  caller; the PROJECT_STRUCTURE.md claim that benchmarks used it was
  outdated.
- `spec/conformance/` — orphan `package main` not invoked by any Makefile
  target, CI job, or doc command.
- `docs/MIGRATION_GUIDE.md` (stale 1.24 migration content), 
  `docs/RELEASE_NOTES.md` (duplicate of CHANGELOG with `interface{}` API
  examples), `docs/PUBLISH_CHECKLIST.md` (one-shot 2025-06-27 launch
  checklist).
- 9.6 MB tracked `deferred-init` Mach-O binary at the repo root, untracked
  and added to `.gitignore`.

### Fixed
- `WithTimeouts(...)` no longer panics at `NewServer` time. `setTimeouts`
  previously wrote `srv.httpServer.X` while `httpServer` was still nil
  (it's built lazily in `StartServer`); now only `srv.Options.X` is set,
  which `StartServer` reads when constructing `http.Server`.
- `cmd/server -mcp` previously advertised built-in tools without
  registering any (the `pkg/mcp/builtin` blank-import was missing and the
  hook-nil check was silent). Fixed by adding the blank import and
  surfacing the misconfiguration as a warn-log.
- README headline claim "`go.sum` has two lines" reworded to reflect the
  actual count (`go.sum` is 12 lines after the `tool` directive pulled in
  the modernize-check transitive deps); ARCHITECTURE.md follows suit.
- `examples/README.md` enumerates every actual example directory; the
  stale `examples/mcp/` reference is gone, the AI-tell star-rating ladder
  is gone, and the v0.25.0 additions (`deferred-init`, `devops`) are now
  listed.
- `pkg/mcp/builtin/server_tools.go` `server_control.restart`,
  `server_control.reload`, and `request_debugger.replay` actions removed
  from the JSON schema enums, descriptions, and dev_guide. They returned
  canned success strings without performing the advertised action —
  direct violation of the project's DoD ("no mocks, stubs, or simulated
  behaviour unless explicitly asked").
- PROJECT_STRUCTURE.md, ARCHITECTURE.md, CLAUDE.md, README.md no longer
  carry stale parenthetical `(Go 1.24)` / `(Go 1.24+)` feature
  attributions; the minimum lives in `go.mod`.
- `benchmarks/run_benchmarks.sh` now targets `./cmd/server` (was the
  stale `./cmd/hyperserve` path).

### Migration

1. **`WithLogger(l)` → `SetDefaultLogger(l)`.** The functional option
   mutated a package global; the explicit setter is now the only path.
2. **`WithMCPNamespace(name, configs...)` → `srv.RegisterMCPNamespace(name, configs...)`** after `NewServer`.
3. **`WithOutStack` callers**: pass a smaller stack to `WithMiddleware`
   instead of subtracting; there were no known callers.
4. **`pkg/server.FileServer(opts)` callers**: replace with
   `pkg/server.SecureWeb(opts)` (the bodies were already identical).
5. **`ResponseTimeMiddleware` callers**: register
   `RequestLoggerMiddleware` and read the duration from its log line,
   or write a one-line custom timing middleware.
6. **`pkg/server/interceptor.go` consumers**: rewrite on
   `pkg/server/middleware.go` (`AuthMiddleware`,
   `RequestLoggerMiddleware`, `RateLimitMiddleware`).
7. **`pkg/websocket.WebSocketPool` consumers**: there were none in the
   examples tree because the type was broken-by-design for server-side
   upgrades.
8. **`pkg/mcp.LoggingCapability` / `PromptsCapability` /
   `SamplingCapability`**: these were never instantiated; references will
   not compile, but no working code path could have used them.
9. **`pkg/mcp.Handler.ProcessRequestDirect` callers** type-asserting to
   `map[string]any`: switch to `mcp.ToolResult`, `mcp.ToolInfo`,
   `mcp.ResourceInfo`, `mcp.ResourceContent` for `tools/call`,
   `tools/list`, `resources/list`, and `resources/read` payloads
   respectively.

## [0.25.1] - 2026-05-17

Patch release. Drives `staticcheck ./...` to zero across pre-existing findings the
0.25.0 `make check` gate exposed, and wires that gate into CI so it runs on every
push and PR. No API changes.

### Changed
- CI (`.github/workflows/ci.yml`) bumped to Go 1.25 (matches `go.mod`), now runs `make test` (which runs the full `check` gate then `go test`), installs `staticcheck` and `govulncheck` explicitly, and builds `./cmd/server` (was the stale `./cmd/hyperserve` path).
- `Makefile`: `test` target now depends on `check`, so local `make test` runs the gofmt + vet + staticcheck + govulncheck + modernize gate before the test suite.
- Error strings in `pkg/server/server.go` lowercased / unpunctuated per Go style (ST1005).

### Removed
- `internal/responsewriter` package — dead since the websocket package landed; zero callers.
- `pkg/server.trailingSlashMiddleware` — unreferenced and the inline TODO already noted it was likely obsolete.
- `pkg/server.(*Server).shutdownHealthServer` — unused (health server shutdown happens inline).
- `pkg/websocket.isWebSocketUpgrade` — unused; the `Upgrade` method does its own checking inline.
- Unused struct fields: `InterceptableResponse.{written,mu}`, `lowConn.closeErr`, `Conn.{pingInterval,pongTimeout}`.
- `spec/conformance.fetchResponse` and its `io` import — unused since the conformance suite landed.
- `.golangci.yml` plus its CI job — `make check` covers the same ground via tools pinned in `go.mod` (vet + staticcheck + modernize) plus govulncheck.

### Fixed
- `pkg/server/server.go` no longer registers `syscall.SIGKILL` with `signal.Notify` — it cannot be trapped (SA1016).
- Health-server `BaseContext` no longer attaches a string-keyed value that nothing reads (SA1029).
- Misc S1023 / S1039 cleanups (redundant return, unnecessary `fmt.Sprintf`) in `pkg/server` and `pkg/websocket`.

## [0.25.0] - 2026-05-17

Pre-1.0 cleanup release. This is a breaking-API change: import paths and several
exported names have moved. See the migration notes below.

### Added
- New package `pkg/mcp` — the MCP protocol surface (Handler, transports, discovery, namespaces, metrics, builders) now lives in its own package, depending only on `net/http`, `log/slog`, and `pkg/jsonrpc`.
- New package `pkg/mcp/builtin` — opt-in built-in MCP tools (Calculator, FileRead, HTTPRequest, ListDirectory) and resources (Config, Metrics, System, ServerLog, ServerHealth) plus the dev-mode toolkit (ServerControl, RouteInspector, RequestDebugger, DevGuide). Blank-import this package to wire `WithMCPBuiltinTools/Resources(true)` and the `MCPDev()` / `MCPObservability()` presets.
- `examples/deferred-init/` — end-to-end demo of `WithDeferredInit` + `WithOnReady` + `WithDeferredInitStopOnFailure`.
- `make check` — runs `vet`, `staticcheck`, `modernize`, `govulncheck`; installs each lazily on first use with pinned versions.
- Several test/observation accessors on `*server.Server`: `MCPHandler()`, `IsRunning()`, `IsReady()`, `ServerStart()`, `TotalRequests()`, `TotalResponseTime()`, `ClientLimiterCount()`, `MiddlewareRoutes()`, `Mux()`, `SetMetrics()`, `AddMetrics()` — required by tests and the builtin package now that they live outside `pkg/server`.

### Changed
- Minimum Go version bumped to **1.25** (was 1.24). Unlocks `sync.WaitGroup.Go(...)`, `testing/synctest`, and container-aware `GOMAXPROCS`. See [ADR-0006](docs/0006-go-minimum-version.md).
- `Server` struct shrunk from 33 fields to 22; deferred-init lifecycle fields extracted into `deferredLifecycle` sub-struct; rate-limiter pool fields extracted into `rateLimiterPool` sub-struct.
- `pkg/websocket` collapsed `internal/ws` back into the public package — the internal/external split was unnecessary indirection. (Subagent task.)
- README, ARCHITECTURE.md, PROJECT_STRUCTURE.md, SCAFFOLDING.md, PRODUCT_VISION.md, MIGRATION_GUIDE.md, RELEASE_NOTES.md, LESSONS_LEARNED.md, and ADR-0006/0009 rewritten to remove AI-generated stylistic tells (adjective stacks, emoji ladders, hollow superlatives) and to reflect the new package layout.

### Removed
- `WithEncryptedClientHello`, `Options.EnableECH`, `Options.ECHKeys` — the option captured keys but never installed them on `tls.Config.EncryptedClientHelloKeys`. Advertised security feature that did nothing. (Reintroduce with a real handshake test when needed.)
- `ChaosMode` / `ChaosMaxLatency` / `ChaosMinLatency` / `ChaosErrorRate` / `ChaosThrottleRate` / `ChaosPanicRate` fields, `ChaosMiddleware`, and `examples/chaos/` — no `WithChaos*` setter ever existed, so the feature was unreachable via the supported API.
- Deprecated MCP options: `WithMCPToolsDisabled()`, `WithMCPResourcesDisabled()`, `WithMCPServerInfo()` — zero callers; superseded by `WithMCPBuiltinTools(bool)`, `WithMCPBuiltinResources(bool)`, and the `name`/`version` args of `WithMCPSupport`.
- `LogResource` + `NewLogResource` + `AddLogEntry` — dead twin of `ServerLogResource` that was the version registered in MCP standard mode. The standard-mode log resource (`logs://server/recent`) now returns the working `ServerLogResource` payload.
- `pkg/server/jsonrpc_facade.go` — internal alias file with no external importers. All MCP code in `pkg/mcp` now imports `pkg/jsonrpc` directly.
- `pkg/server/websocket_facade.go` — internal alias file with one external example caller that already imported `pkg/websocket` directly. (Subagent task.)
- `internal/ws/` directory — merged into `pkg/websocket`. (Subagent task.)
- 10 MB tracked Mach-O binary at the repo root.

### Fixed
- Standard-mode MCP `logs://server/recent` resource is now wired to the working log buffer; previously always returned empty.
- Pre-existing race in `TestMiddlewareWithWebSocket` (handler-call atomic was asserted before the server goroutine could store it) — added a short bounded wait.

### Migration

1. **Imports.** Any code referencing `server.MCPHandler`, `server.MCPTool`, `server.MCPResource`, `server.NewMCPExtension`, `server.NewTool`, `server.NewResource`, etc. should import `mcp "github.com/osauer/hyperserve/pkg/mcp"` and use `mcp.Handler`, `mcp.Tool`, `mcp.Resource`, `mcp.NewExtension`, `mcp.NewTool`, `mcp.NewResource`. The full rename table is in the CHANGELOG history of `pkg/mcp/types.go` and in the migration guide.
2. **Built-in MCP tools.** Add `_ "github.com/osauer/hyperserve/pkg/mcp/builtin"` to your `main` package so the `WithMCPBuiltinTools(true)` / `MCPDev()` / `MCPObservability()` hooks fire. Without the blank import, the option calls are no-ops.
3. **Extension API.** `mcp.Extension.Configure` now takes `*mcp.Handler` instead of `*server.Server`. If your extension needs `*Server` access, refactor it to read the handler off the server (`srv.MCPHandler()`) at registration time.
4. **Removed surface.** Replace `WithMCPServerInfo(name, version)` with `WithMCPSupport(name, version)`; drop calls to `WithMCPToolsDisabled()` / `WithMCPResourcesDisabled()` (defaults are already off); remove any `WithEncryptedClientHello(...)` calls and any reliance on `Options.EnableECH/ECHKeys`; drop `ChaosMode` references.

## [0.24.0] - 2025-10-19

### Added
- `CompleteDeferredInit` helper for manually finalizing deferred bootstrap sequences without restarting the process.

### Fixed
- Ensured deferred-init readiness gating is respected by the running HTTP server and avoids matching health endpoint prefixes.
- Allowed OnReady hooks to rerun after failures and improved manual recovery paths for non-fatal bootstrap errors.

## [0.23.0] - 2025-10-19

### Added
- Deferred initialization lifecycle support via `WithDeferredInit`, readiness gating, and `WithOnReady` hooks for post-bootstrap route registration.
- Startup banner color control with `WithBannerColor` and the `HS_BANNER_COLOR` environment flag.

## [0.22.0] - 2025-09-29

### Changed
- Removed root compatibility facades; consumers now import `github.com/osauer/hyperserve/pkg/server` directly.
- Updated all binaries, examples, and documentation to the new package layout.
- Simplified Makefile targets to build via the `cmd/server` entry point.

## [0.21.0] - 2025-09-29

### Changed
- Moved server, WebSocket, and JSON-RPC implementations into versioned `pkg/` packages with compatibility facades.
- Simplified top-level repository layout and aligned architecture documentation.
- Reworked tests to live beside their implementation packages and pruned transient fixture directories.

## [0.20.1] - 2025-09-27

### Added
- Regression coverage for shutdown context propagation and WebSocket pool statistics snapshots.

### Changed
- Brought README, ADRs, guides, and API spec in sync with current behaviour and APIs.
- Hardened MCP SSE and middleware integration tests to use sandbox-friendly listeners or skip gracefully when networking is restricted.
- Documented repository hygiene expectations and broadened ignores for local tooling caches.

### Fixed
- Prevented double-closing of the rate-limiter cleanup ticker during repeated shutdowns.
- `go test ./...` no longer leaves stray `_test.txt` logs and avoids panicking when health-server ports are unavailable.

## [0.19.1] - 2025-07-22

### Added
- **JSON Response for MCP GET Requests** - Enhanced MCP handler for better tool integration (#78)
  - Added robust Accept header parsing with `isJSONAccepted()` function
  - MCP GET endpoint now returns JSON when Accept header contains `application/json` or `*/*`
  - JSON response includes server status, capabilities, endpoint, and transport information
  - Handles quality factors, case sensitivity, and complex Accept headers
  - Added error handling for JSON encoding with proper logging
  - Refactored capabilities into reusable `getCapabilities()` method
  - Added comprehensive test coverage for all Accept header scenarios
  - Improves compatibility with automated tools like Claude Code

## [0.19.0] - 2025-07-20

### Changed
- **Standardized MCP Tool Naming** - All tools now use consistent namespace prefixes
  - Built-in tools now use `mcp__hyperserve__` prefix (e.g., `mcp__hyperserve__calculator`)
  - Developer tools now use `mcp__hyperserve__` prefix (e.g., `mcp__hyperserve__server_control`)
  - External/custom tools maintain their existing namespace pattern (e.g., `mcp__daw__play`)
  - Empty namespace in `RegisterToolInNamespace()` now defaults to server name
  - Updated all tests and documentation to reflect new naming convention
  - This change improves API consistency and prevents naming conflicts

## [0.18.0] - 2025-07-20

### Added
- **Dynamic Discovery with Security Policies** - Enhanced MCP discovery with configurable security
  - Tools/resources can implement `IsDiscoverable()` to opt out of discovery
  - Added `DiscoveryPolicy` enum: Public, Count, Authenticated, None
  - Added `WithMCPDiscoveryPolicy()` and `WithMCPDiscoveryFilter()` server options
  - Discovery endpoints now return dynamic tool/resource lists based on policy
  - Default filtering hides dev tools (server_control, etc) in production
  - Custom filters enable RBAC integration via Authorization headers
  - Created examples/mcp-discovery demonstrating different policies

### Changed
- Discovery endpoints now show actual registered tools/resources instead of static capabilities
- Tool counts and lists respect discovery policies and custom filters

## [0.17.0] - 2025-07-20

### Added
- **MCP Discovery Endpoints** - Implemented Claude Code discovery mechanisms
  - Added `/.well-known/mcp.json` endpoint for standard MCP discovery
  - Added `/mcp/discover` endpoint as alternative discovery mechanism
  - Both endpoints return transport information, capabilities, and connection details
  - Enables automatic MCP server discovery without manual configuration
  - Updated CLAUDE.md and README.md with discovery endpoint documentation

## [0.16.0] - 2025-07-20

### Added
- **Enhanced SSE Documentation** - Improved discoverability of the unified MCP endpoint approach
  - Updated CLAUDE.md with clear SSE connection instructions for AI assistants
  - Added SSE capability to MCP initialize response for automatic discovery
  - Enhanced README.md with SSE support section explaining unified endpoint
  - Expanded MCP_GUIDE.md with detailed SSE documentation and examples
  - Created examples/mcp-sse directory with complete SSE client/server examples

### Changed
- **Simplified MCP Implementation** - Removed backward compatibility for cleaner code
  - `RegisterTool()` and `RegisterResource()` now register without namespace prefixing
  - `RegisterToolInNamespace()` and `RegisterResourceInNamespace()` always apply prefixes
  - Removed complex dual-mode logic, making the API more predictable
  - Net reduction of 17 lines while improving clarity

### Fixed
- **SSE Capability Reporting** - SSE support is now properly advertised in MCP capabilities
  - Added `SSECapability` struct to `MCPCapabilities`
  - Initialize response now includes SSE configuration details
  - Instructions updated to mention SSE support alongside regular HTTP

## [0.15.0] - 2025-07-19

### Added
- **MCP Namespace Support** - Organize tools and resources into logical namespaces
  - New methods: `RegisterMCPToolInNamespace`, `RegisterMCPResourceInNamespace`, `RegisterMCPNamespace`
  - Server option: `WithMCPNamespace` for namespace configuration
  - Tools/resources in namespaces are prefixed with `mcp__namespace__name` format
  - Backward compatibility maintained for non-namespaced registration
  - Default namespace uses server name when not specified
  - Comprehensive test coverage with 8 test scenarios
  - Updated MCP_GUIDE.md with namespace documentation and examples

### Changed
- MCP handler now tracks registered namespaces internally
- Tools and resources use flat maps with prefixed keys for efficient lookup

## [0.14.7] - 2025-07-19

### Fixed
- **MCP Calculator Tool** - Fixed missing calculator tool when builtin tools are enabled
  - Calculator tool is now properly registered with `WithMCPBuiltinTools(true)`
  - Fixes test failures in concurrent and default MCP tests

## [0.14.6] - 2025-07-19

### Fixed
- **Route Inspector Tool** - Now shows all registered routes instead of limiting to 5
  - Fixed iteration logic to properly display all routes in middleware registry
  - Improved route discovery for better debugging capabilities

### Changed
- **Project Structure** - Major cleanup and reorganization for better maintainability
  - Removed duplicate files (docs/CHANGELOG.md, README_NEW.md, outdated READMEs)
  - Consolidated MCP examples from 6 to 4 focused examples:
    - `mcp-basic` (merged mcp + mcp-sse) - Complete basic example with HTTP/SSE
    - `mcp-cli` (merged mcp-flags + mcp-development) - CLI configuration with dev mode
    - `mcp-extensions` - Advanced application integration patterns
    - `mcp-stdio` - Claude Desktop integration
  - Consolidated websocket examples into single `websocket-demo`
  - Cleaned up compiled binaries from examples directory
  - Updated .gitignore to comprehensively exclude all compiled binaries in examples
  - Created `docs/guides/` directory for future guide documents

### Improved
- **Documentation** - Enhanced CLAUDE.md with reference to MCP_GUIDE.md for better AI assistant discovery
- **Repository Hygiene** - Removed temporary test files and maintained single-package architecture for API stability

## [0.14.0] - 2025-07-13

### Added
- **MCP Discovery Improvements** - Enhanced discoverability for AI assistants
  - Prominent MCP discovery banner displayed on server startup in developer mode
  - Shows example curl command for `tools/list` discovery
  - Added `.mcp` marker file for project-level MCP detection
  - Updated CLAUDE.md with immediate action instructions for AI assistants
- **Git Ignore Enhancement**
  - Added `.claude/settings.local.json` to .gitignore to prevent tracking local settings

### Improved
- AI assistants now immediately recognize and utilize MCP capabilities when working with HyperServe projects
- Clear instructions in startup banner for MCP tool discovery

## [0.13.3] - 2025-07-13

### Changed
- **BREAKING**: Removed `SecureWebWithRateLimit` middleware to avoid API bloat
  - Use `SecureWeb` with separate `RateLimitMiddleware` instead
- Implemented "secure by default" approach for server timeouts
  - Default timeouts increased for better compatibility (30s read/write)
  - Automatic Slowloris protection with 10s ReadHeaderTimeout
  - No longer requires explicit configuration for basic security

### Security
- Server now starts with secure timeout defaults out of the box
- Slowloris attacks are mitigated by default with ReadHeaderTimeout

## [0.13.2] - 2025-07-13

### Fixed
- **MCP Request Debugger** - Fixed request capture middleware that was not intercepting HTTP requests
  - Added RequestCaptureMiddleware to actually intercept and store requests
  - Added CaptureRequest method with atomic ID generation
  - Added captureResponseWriter to capture response headers, status, and body
  - Automatic middleware registration in MCP dev mode
  - Memory management with 100 request limit and 64KB response body limit
  - Thread-safe operation using sync.Map

## [0.13.1] - 2025-07-13

### Added
- **Enhanced Security Middleware**
  - New `SecureWebWithRateLimit` middleware stack that combines security headers with optional rate limiting
  - Automatically includes rate limiting only when configured (`RateLimit > 0`)
- **WebSocket Telemetry**
  - WebSocket connections are now tracked in server telemetry
  - New `WebSocketUpgrader()` method on Server that automatically tracks WebSocket metrics
  - WebSocket connection count displayed in server shutdown metrics
  - Helper functions for WebSocket origin validation (`defaultCheckOrigin`, `checkOriginWithAllowedList`)

### Improved
- Enhanced middleware documentation with security examples
- Added comprehensive tests for new security features

## [0.13.0] - 2025-07-13

### Added
- **Enhanced Security Features**
  - Individual timeout configuration options: `WithReadTimeout`, `WithWriteTimeout`, `WithIdleTimeout`, `WithReadHeaderTimeout`
  - Automatic Slowloris attack protection via `ReadHeaderTimeout` (defaults to `ReadTimeout` if not set)
  - Comprehensive security documentation in README
  - Timeout configuration guide with recommendations
  - Integration tests for security features
- **Improved Error Handling**
  - Added `closeWithLog` helper for proper defer close error handling
  - Updated error comparisons to use `errors.Is` and `errors.As`
  - Added error wrapping for better context in external package errors
- **Documentation**
  - Added missing comments on exported types
  - Documented SHA1 usage in WebSocket as RFC 6455 requirement
  - Added security best practices section

### Fixed
- Integer overflow protection in WebSocket frame size handling
- Unchecked errors in defer close statements
- Health server now uses same timeout configuration as main server
- ReadHeaderTimeout properly applied to both main and health servers

### Security
- Mitigated Slowloris attacks with proper timeout configuration
- Protected against integer overflow in WebSocket frame parsing
- Improved error handling to prevent information leakage

## [0.12.2] - 2025-07-13

### Fixed
- MCP tool response formatting now properly handles different return types (strings, maps, arrays)
- Fixed Zod validation errors in Claude by correctly formatting tool responses with content arrays
- Tool responses returning maps/objects are now JSON-serialized to text content

### Added
- Comprehensive test coverage for MCP tool response formatting
- New `dev_guide` tool for better MCP developer experience
  - Interactive help system showing available tools and resources
  - Usage examples and common workflows
  - Topic-based documentation (overview, tools, resources, examples, workflows)

### Improved
- Enhanced tool descriptions with detailed parameter explanations
- Better discovery of MCP capabilities for AI assistants
- More helpful error messages and guidance in developer tools

## [0.12.1] - 2025-07-13

### Fixed
- Prevent duplicate MCP configuration messages when using WithMCPSupport with MCPDev()
- Auto-configuration now correctly skips when MCP is already configured programmatically

### Added
- Test coverage for MCP configuration scenarios to prevent regression

## [0.12.0] - 2025-07-13

### Added
- MCP configuration via command-line flags and environment variables
  - `--mcp`, `--mcp-dev`, `--mcp-observability`, `--mcp-transport` flags
  - `HS_MCP_*` environment variables for all MCP settings
- Auto-configuration of MCP from ServerOptions during server initialization
- Claude Code integration examples with HTTP transport
- Comprehensive MCP flags example showing different configuration methods

### Changed
- MCP can now be configured without hardcoding in source code
- Updated documentation to emphasize flag/environment configuration over code
- Enhanced README with Claude Code HTTP integration examples

### Security
- Development mode (`MCPDev()`) no longer needs to be hardcoded in production builds

## [0.11.0] - 2025-07-13

### Added
- MCP Developer Tools (`MCPDev()`) for AI-assisted development
  - Server restart and reload capabilities
  - Dynamic log level changes
  - Route inspection
  - HTTP request capture and replay
- MCP Observability (`MCPObservability()`) for production monitoring
  - Sanitized server configuration
  - Health metrics and uptime
  - Recent logs with circular buffer
- MCP Extensions API for building custom tools and resources
  - Fluent builder pattern
  - `SimpleTool` and `SimpleResource` helpers
  - `MCPExtension` for grouping functionality
- Comprehensive MCP Integration Guide
- DevOps support with environment variables
  - `HS_DEBUG` and `HS_LOG_LEVEL` for logging control
  - `WithDebugMode()` and `WithLogLevel()` options

### Changed
- **BREAKING**: MCP support now requires transport configuration (HTTP or STDIO)
- **BREAKING**: MCP built-in tools and resources now disabled by default
- Restructured MCP API for better separation of concerns
- Improved README with professional tone and Claude Desktop examples

### Security
- MCP DevOps resources explicitly exclude sensitive data
- Developer mode shows prominent warning in logs

## [0.10.0] - 2025-07-13

### Added
- WebSocket support (RFC 6455 compliant)
  - Zero-dependency implementation using standard library
  - Secure-by-default with origin validation
  - Configurable frame size limits
  - Ping/pong keepalive support
- WebSocket security features
  - Origin validation with `CheckOrigin`
  - `SameOriginCheck()` and `AllowedOriginsCheck()` helpers
  - Subprotocol negotiation
  - Extension support hooks
- Enhanced middleware compatibility
  - ResponseWriter interface preservation (Hijacker, Flusher, etc.)
  - Proper error handling in middleware chains
- Comprehensive WebSocket guide and examples

### Security
- WebSocket frame validation per RFC 6455
- Protection against frame injection attacks
- Secure defaults for origin checking

[1.1.0]: https://github.com/osauer/hyperserve/compare/v0.34.2...v1.1.0
[0.13.3]: https://github.com/osauer/hyperserve/compare/v0.13.2...v0.13.3
[0.13.2]: https://github.com/osauer/hyperserve/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/osauer/hyperserve/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/osauer/hyperserve/compare/v0.12.2...v0.13.0
[0.12.2]: https://github.com/osauer/hyperserve/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/osauer/hyperserve/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/osauer/hyperserve/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/osauer/hyperserve/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/osauer/hyperserve/compare/v0.9.0...v0.10.0
