# Production Deployment Guide

_Last updated: 2026-05-18 21:28 CEST (v0.33.1)._

What you need to know to put HyperServe behind a real reverse proxy
and not get bitten. Concrete, no marketing — every section names the
exact option, header, or failure mode it's about.

## Scope

In scope: TLS, reverse-proxy / CDN gotchas, MCP discovery security
model, static file serving, health endpoints, deferred init, the
binding-token capability for SSE clients.

Out of scope: capacity planning, OS tuning, container build
recipes, observability backend integration (OTLP is on the
[roadmap](./ROADMAP.md)).

If you're new to the framework, read [README.md](../README.md) and
[ARCHITECTURE.md](../ARCHITECTURE.md) first. This document assumes
you have a HyperServe binary that compiles and serves traffic
locally.

## Topology

A typical production deployment looks like:

```
clients → CDN / WAF → reverse proxy (nginx / envoy / Caddy) → HyperServe
                                                            → HyperServe (health on :9080)
```

The framework ships two HTTP servers in one binary: the main server
(default `:8080`) and an optional health server (default `:9080`,
enabled via `WithHealthServer()`). Run the health server on a port
that is **not** exposed by your ingress — the health endpoints are
for your orchestrator (Kubernetes liveness/readiness, AWS target
group health checks), not for end users.

```go
srv, _ := server.NewServer(
    server.WithAddr(":8080"),
    server.WithHealthServer(),                 // /healthz/, /readyz/, /livez/ on :9080
    server.WithHealthAddr(":9080"),            // override if needed
    server.WithTLS("cert.pem", "key.pem"),     // terminate TLS at the app, OR
                                                // terminate at the proxy and skip this
)
```

## Reverse proxy and CDN

### Trust the proxy's scheme header

When TLS terminates at the proxy, requests reach HyperServe over
plaintext, and `r.TLS` is `nil`. The discovery endpoint (and any
URL-building code) reads `X-Forwarded-Proto` to recover the
client-facing scheme:

```go
// pkg/mcp/discovery.go
scheme := "http"
if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
    scheme = "https"
}
```

Configure your proxy to set `X-Forwarded-Proto: https` for
TLS-terminated traffic. If you don't, `/.well-known/mcp.json` will
advertise `http://` endpoints to AI clients that found you over
HTTPS.

### Vary: Authorization and the cache-poisoning trap

HyperServe sets `Vary: Authorization` on every discovery response,
and switches `Cache-Control` to `private, max-age=60` when
`MCPDiscoveryPolicy == DiscoveryAuthenticated`. This closes a
specific failure mode (fixed in v0.33.0): a CDN keyed on URL alone
would cache an authenticated discovery response and replay the full
tool list to anonymous clients within the TTL.

**Your CDN config must honor `Vary`.** CloudFront, Fastly, and
Cloudflare all do by default; some custom configurations strip
`Vary` for cache efficiency. If yours does, switch the policy to
`DiscoveryCount` or `DiscoveryNone` — these emit the same body
regardless of the `Authorization` header, so caching by URL alone is
safe.

### Trusted proxies

HyperServe does **not** ship a "trusted proxies" allow-list for
`X-Forwarded-*` headers. If your service is reachable directly
(without going through your proxy), clients can spoof
`X-Forwarded-Proto: https` and influence URL generation. Use one of:

1. Bind HyperServe to a loopback or VPC-private interface so only
   the proxy can reach it.
2. Run your proxy in a sidecar / unix-socket pattern.
3. Don't rely on `X-Forwarded-Proto` for security decisions (it's
   only used for URL display in discovery responses today).

## TLS

`WithTLS(certFile, keyFile)` enables TLS with a hardened config:
TLS 1.2 minimum, ECDHE-only suites, no static IVs. See
[pkg/server/options.go](../pkg/server/options.go) for the exact
list. There is no support for legacy TLS 1.0 / 1.1 — if you need to
terminate older clients, do it at your proxy.

### HSTS

HSTS is sent **only over TLS**, with the two-year preload value:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
```

Plaintext responses ship no HSTS header. This is intentional — sending
HSTS over HTTP is a no-op the spec forbids (clients ignore it), and
the prior shape (overridden by a static security-headers table
incorrectly) was a v0.32.0 fix.

If you want to submit your domain to the HSTS preload list, the
header above is the configuration HSTS preload requires. Submit at
https://hstspreload.org/.

## MCP security model

If `WithMCPSupport(...)` is enabled, three concerns matter:

### Discovery policy

The `MCPDiscoveryPolicy` option controls what `/.well-known/mcp.json`
returns:

| Policy | What anonymous clients see |
|---|---|
| `DiscoveryPublic` (default) | Tool/resource names + transports + capabilities |
| `DiscoveryCount` | Counts only — no names |
| `DiscoveryAuthenticated` | Counts only without `Authorization`; full list with it |
| `DiscoveryNone` | Counts only; no names ever |

**`DiscoveryAuthenticated` is not authorization** — it only gates the
discovery payload. Anyone who calls `tools/list` over the MCP endpoint
can still enumerate tools. Use it to keep your tool/resource names out
of casual scrapes (and out of CDN-cached payloads served to anonymous
clients), not as an access-control mechanism. Real authorization
belongs in your auth middleware on the MCP endpoint itself.

### SSE binding token capability

SSE clients receive a `clientId` and a `bindingToken` in the initial
`connection` event. Subsequent POSTs that should be delivered via
that SSE stream must present **both** headers:

```
X-SSE-Client-ID: <clientId>
X-SSE-Binding:   <bindingToken>
```

Missing or wrong binding token → 403. The binding token is the
capability — knowing a `clientId` (which may be observable to anyone
on the same network or in proxy logs) is not enough to inject
responses into another client's stream. The token is generated with
`crypto/rand`, compared in constant time, and fail-closed on rand
errors. See [pkg/mcp/transport_sse.go](../pkg/mcp/transport_sse.go).

Do not log the binding token. The framework doesn't, but if you
wrap MCP behind your own middleware that logs request headers,
filter `X-SSE-Binding`.

### Built-in tools / resources

Off by default. To enable, **blank-import** the package and turn the
toggles on:

```go
import _ "github.com/osauer/hyperserve/pkg/mcp/builtin"

server.WithMCPSupport("MyServer", "1.0.0")
server.WithMCPBuiltinTools(true)
server.WithMCPBuiltinResources(true)
server.WithMCPFileToolRoot("/srv/data")  // required for file tools
```

Notable security postures, all enforced at construction:

- **File tools (`read_file`, `list_directory`)** refuse to instantiate
  without `WithMCPFileToolRoot(...)` — there is no unsandboxed fallback.
  Inside the sandbox they use `os.Root` (Go 1.24+), so traversal
  attempts (`../etc/passwd`) cannot escape the configured directory.
- **`http_request` tool was removed in an earlier release** — it allowed
  any MCP caller to make outbound HTTP from the server process (SSRF).
  If you need outbound HTTP, register a domain-allow-listed tool from
  your own code; don't restore the old shape.
- **`request_debugger` tool was removed in an earlier release** — it
  captured request headers (Authorization, Cookie, API keys) into a
  process-wide store any MCP caller could read.

### Do not enable `MCPDev` in production

`server.MCPDev()` enables `server_control`, `route_inspector`, and
`dev_guide` tools that allow log-level changes and route
introspection. The framework prints a `⚠️ MCP DEVELOPER MODE ENABLED ⚠️`
warning on startup. Treat that warning as a build-failure signal in
your CI for production targets.

For production observability, use `server.MCPObservability()` — it
exposes `config://server/current`, `health://server/status`, and
`logs://server/recent` with no control-plane mutation.

## Static files

`HandleStatic(pattern)` uses `os.OpenRoot` to sandbox file serving to
`Options.StaticDir`. If `os.OpenRoot` fails at startup (directory
missing, permission denied), it falls back to `http.FileServer(http.Dir(...))`
and logs:

```
WARN  Failed to open static root directory, falling back to http.Dir
```

**Treat this warning as a deployment failure.** The fallback is for
local development convenience. In production, an unsandboxed
file server is one path-traversal bug away from leaking
`/etc/passwd`-class data. Fix the directory permissions / existence
issue instead of running on the fallback path.

Static serving is GET/HEAD only — POST returns 405.

## Health endpoints

`WithHealthServer()` registers three endpoints on the health port:

| Path | When 200 | When 503 |
|---|---|---|
| `/healthz/` | Server process is alive | Never returns 503 — process-level liveness only |
| `/readyz/` | `isReady` flag is `true` | Deferred init not yet complete |
| `/livez/` | Server hasn't called `Stop()` | After graceful shutdown begins |

`isReady` is `true` immediately after `NewServer` returns, **unless**
you used `WithDeferredInit(...)`. In that case `/readyz/` returns 503
until the deferred init callback returns nil, the `OnReady` hooks run,
and `CompleteDeferredInit(...)` is called (or it completes
automatically on init return).

The main `/.well-known/mcp.json` and any application routes are NOT
on the health port — they're on the main `Addr`. The health port
exists so your orchestrator can health-check without competing for the
same ingress queue.

### Deferred init

If your application needs to do slow startup work (warm a cache, open
DB pools, fetch config from a remote service) and you want to serve
`/healthz/` immediately, use:

```go
srv, _ := server.NewServer(
    server.WithDeferredInit(func(ctx context.Context, srv *server.Server) error {
        return slowBootstrap(ctx)
    }),
    server.WithOnReady(func(ctx context.Context) error {
        log.Println("Server is ready")
        return nil
    }),
)
```

While deferred init runs:
- `/healthz/` and `/livez/` return 200 immediately
- `/readyz/` returns 503
- Application routes return 503 with a `Retry-After` header
- The orchestrator (Kubernetes) keeps you out of the load balancer until
  readiness flips

This pattern lets you separate "is this pod alive?" (liveness — restart
me if I'm not) from "is this pod ready for traffic?" (readiness —
don't send me requests yet).

## Rate limiting and auth

`WithRateLimit(rps, burst)` enables a per-client (by remote IP)
rate limiter. Per-client limiters are kept in a sync.Map that the
framework cleans up every 5 minutes for limiters not seen in the last
10 minutes. There is no global rate limiter — if you need one,
register one as middleware.

For auth, the framework provides Bearer / Basic / API-key validator
hooks; see [examples/auth/](../examples/auth/) for a JWT example. The
validator runs inside `subtle.WithDataIndependentTiming` to prevent
timing oracles.

If your auth middleware is upstream (a JWT-validating reverse proxy,
an Envoy filter, an OAuth2 proxy), HyperServe accepts the headers and
your handler reads them. The framework does **not** require auth — if
you skip it, your endpoints are public.

## Logging

The framework uses `log/slog`. The default logger writes plain text
to stderr at INFO level. To use JSON in production:

```go
import "log/slog"

handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
slog.SetDefault(slog.New(handler))
server.SetDefaultLogger(slog.New(handler))
```

Set both — the framework's package-level logger is separate from
`slog.Default()` for callers that want to silence framework chatter
without affecting application logs.

There is **no built-in request-correlation ID middleware** as of
v0.33. If you need one, write a small middleware that sets/propagates
`X-Request-ID` and adds it to the request context. (The `TraceMiddleware`
that used to ship was deleted in v0.32 — it was never wired into any
preset and the trace_id field it populated was empty in all real
deployments.)

## Pre-deploy checklist

Before you ship:

- [ ] `make check` passes — gofmt, vet, staticcheck, govulncheck,
      modernize, plus per-example govulncheck after v0.33.1
- [ ] `go test -race ./...` passes
- [ ] TLS enabled either in HyperServe (`WithTLS`) or in your proxy
- [ ] HSTS preload submitted (if you want it) at
      https://hstspreload.org/
- [ ] `X-Forwarded-Proto: https` set by your proxy if TLS terminates
      upstream
- [ ] CDN config honors `Vary: Authorization` (or
      `MCPDiscoveryPolicy` set to `DiscoveryCount` / `DiscoveryNone`)
- [ ] Health server bound to an interface your ingress does NOT expose
- [ ] `MCPDev()` is **not** present in any production preset list
- [ ] `WithMCPFileToolRoot` set if `WithMCPBuiltinTools(true)` and you
      want the file tools — otherwise they're skipped with a WARN log
- [ ] `staticRoot` log line shows "using secure os.Root" (not "falling
      back to http.Dir")
- [ ] Structured (JSON) logging configured if your log aggregator
      requires it
- [ ] Request-correlation middleware added if you need it (none ships)

## Where to look next

- [API_STABILITY.md](./API_STABILITY.md) — what we promise pre- and
  post-1.0.
- [MCP_GUIDE.md](./MCP_GUIDE.md) — the full MCP reference including
  namespaces, presets, and the SSE flow with sample shells.
- [WEBSOCKET_GUIDE.md](./WEBSOCKET_GUIDE.md) — WebSocket security
  posture (origin checking, Sec-WebSocket-Key, frame limits).
- [SECURITY.md](../SECURITY.md) — how to report vulnerabilities.
- [examples/auth/](../examples/auth/) — Bearer / Basic / API-key
  validator examples with JWT + role gating.
