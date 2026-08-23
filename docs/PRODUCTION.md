# Production Deployment Guide

_Last updated: 2026-05-24 07:21 CEST (v1 line)._

How to put HyperServe behind a reverse proxy without getting bitten.

## Topology

Two HTTP servers ship in one binary: the main server (default `:8080`)
and an optional health server (default `:9080`, enabled via
`WithHealthServer()`). Bind the health port to an interface your
ingress does not expose. Its endpoints are for the orchestrator
(Kubernetes probes, target-group health checks), not for end users.

```
clients → CDN/WAF → reverse proxy (nginx/envoy/Caddy) → HyperServe :8080
                                                      → HyperServe :9080 (health, private)
```

```go
srv, _ := server.NewServer(
    server.WithAddr(":8080"),
    server.WithHealthServer(),                 // /healthz/, /readyz/, /livez/ on :9080
    server.WithHealthAddr(":9080"),            // override if needed
    server.WithTLS("cert.pem", "key.pem"),     // or terminate TLS at the proxy
)
```

## Reverse proxy and CDN

### Pass the client scheme through

When TLS terminates at the proxy, requests reach HyperServe over
plaintext, so `r.TLS` is `nil`. Discovery and any URL-building code
reads `X-Forwarded-Proto` to recover the client-facing scheme:

```go
// pkg/mcp/discovery.go
scheme := "http"
if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
    scheme = "https"
}
```

Set `X-Forwarded-Proto: https` on TLS-terminated traffic. Without it,
`/.well-known/mcp.json` advertises `http://` endpoints to AI clients
that reached you over HTTPS.

### Vary: Authorization

Discovery responses always carry `Vary: Authorization`, and switch to
`Cache-Control: private, max-age=60` when `MCPDiscoveryPolicy ==
DiscoveryAuthenticated`. The previous shape (`public, max-age=300`
everywhere) was a cache-poisoning bug fixed before the v1 line: a CDN keyed on
URL alone would store an authenticated response and replay the full
tool list to anonymous clients for the next 300 seconds.

Your CDN must honor `Vary`. CloudFront, Fastly, and Cloudflare do by
default. Some custom configs strip it for cache efficiency; if yours
does, set the policy to `DiscoveryCount` or `DiscoveryNone`. Those
return the same body regardless of `Authorization` and are safe to
cache by URL alone.

### Trusted proxies

No "trusted proxies" allow-list ships. If the service is reachable
directly, a client can spoof `X-Forwarded-Proto: https` and influence
URL generation. Pick one:

1. Bind HyperServe to loopback or a VPC-private interface.
2. Run the proxy as a sidecar over a Unix socket.
3. Don't depend on `X-Forwarded-Proto` for security decisions. Today
   it only affects URL display in discovery responses, so the blast
   radius is small.

## TLS

`WithTLS(certFile, keyFile)` gives you TLS 1.2 floor, ECDHE-only
suites, no static IVs. The exact cipher list lives in
[pkg/server/options.go](../pkg/server/options.go). TLS 1.0 and 1.1 are
not supported. If you need them for legacy clients, terminate at the
proxy.

### HSTS

HSTS is sent only over TLS:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
```

Plaintext responses ship no HSTS header. Clients ignore HSTS over HTTP
by spec, and sending it both ways was a v0.32.0 bug. To preload your
domain, the header above is what hstspreload.org wants. Submit there.

## MCP security model

If `WithMCPSupport(...)` is enabled, three things matter.

### Discovery policy

`MCPDiscoveryPolicy` controls what `/.well-known/mcp.json` returns to
anonymous callers:

| Policy | Anonymous response |
|---|---|
| `DiscoveryPublic` (default) | Tool/resource names plus transports plus capabilities |
| `DiscoveryCount` | Counts only, no names |
| `DiscoveryAuthenticated` | Counts only without `Authorization`; full list with it |
| `DiscoveryNone` | Counts only, never names |

`DiscoveryAuthenticated` is not authorization. It only gates the
discovery payload. Anyone who calls `tools/list` over the MCP endpoint
can still enumerate. Use it to keep tool names out of casual scrapes
and CDN-cached payloads. Real access control belongs in auth
middleware on the MCP endpoint itself.

### Streamable HTTP Origin policy

MCP rejects a present browser `Origin` unless its scheme, host, and port match
the request. Requests from normal non-browser clients usually omit Origin and
remain valid. TLS-terminating proxies or authenticated cross-origin clients
can install an explicit `WithMCPOriginValidator` policy; do not treat Origin
as authentication.

Finite MCP requests return JSON. `subscriptions/listen` keeps its POST
response open as `text/event-stream`; configure reverse proxies to preserve
streaming, disable response transformation/buffering, and allow idle periods
longer than the 30-second keepalive. HyperServe sends
`Cache-Control: no-cache, no-transform` and `X-Accel-Buffering: no`, uses a
30-second deadline for each write, and bounds each stream to 32 queued events
and 128 requested resource URIs. Producers block when the queue is full; events
are never silently dropped.

The standard endpoint deliberately has no protocol sessions, resumability IDs,
or GET stream. GET and DELETE return `405` with `Allow: POST`. Malformed
metadata, Accept values, parameters, or transport-confusion headers return
`400`; oversized bodies return `413`; unsupported content types return `415`;
unsupported RPC methods return `404`; invalid Origins return `403`.

`Server.Stop` and the normal signal path call `(*mcp.Handler).Shutdown` before
canceling the HTTP base context so active listens can receive their final
`resultType: complete` response. Applications that mount an `mcp.Handler`
directly should call its idempotent `Shutdown(ctx)` during graceful shutdown.

### Legacy routed-SSE binding-token capability

The proprietary HyperServe compatibility stream (not MCP 2026-07-28
Streamable HTTP) is deprecated and disabled by default. Enable it temporarily
with `server.WithMCPLegacyRoutedSSE(true)`. It gives clients a `clientId` and a
`bindingToken` in the initial `connection` event. POSTs routed to that stream
must present both headers:

```
X-SSE-Client-ID: <clientId>
X-SSE-Binding:   <bindingToken>
```

Missing or wrong binding returns 403. The token is the capability.
Knowing a `clientId` (which can show up in proxy access logs) is not
enough to inject responses into another client's stream. The token is
generated with `crypto/rand`, compared in constant time, and fails
closed on rand errors. See
[pkg/mcp/transport_sse.go](../pkg/mcp/transport_sse.go).

Do not log the token. HyperServe doesn't, but if you wrap MCP behind
your own header-logging middleware, filter `X-SSE-Binding`.

### Built-in tools and resources

Off by default. Blank-import the package and flip the toggles:

```go
import _ "github.com/osauer/hyperserve/pkg/mcp/builtin"

server.WithMCPSupport("MyServer", "1.0.0")
server.WithMCPBuiltinTools(true)
server.WithMCPBuiltinResources(true)
server.WithMCPFileToolRoot("/srv/data")  // required for file tools
```

Security postures enforced at construction:

- File tools (`read_file`, `list_directory`) refuse to instantiate
  without `WithMCPFileToolRoot(...)`. There is no unsandboxed fallback.
  Inside the sandbox they use `os.Root` (Go 1.24+), so `../etc/passwd`
  cannot escape.
- The `http_request` tool was removed in an earlier release. It let
  any MCP caller make outbound HTTP from the server process, which is
  SSRF and cloud-metadata exfil. If you need outbound HTTP, register a
  domain-allowlisted tool from your own code. Don't restore the old
  shape.
- The `request_debugger` tool was removed in an earlier release. It
  captured every request's headers (Authorization, Cookie, API keys)
  into a process-wide store any MCP caller could read.

### Do not enable MCPDev in production

`server.MCPDev()` adds `server_control`, `route_inspector`, and
`dev_guide` tools that change log level and introspect routes.
HyperServe logs `⚠️ MCP DEVELOPER MODE ENABLED ⚠️` during construction.
Wire your production CI to fail whenever that line appears. This warning is
separate from the opt-in ASCII startup banner.

For production observability, use `server.MCPObservability()`. It
exposes `config://server/current`, `health://server/status`, and
`logs://server/recent` with no control-plane mutation.

## Static files

`HandleStaticChecked(pattern)` confines file serving to `Options.StaticDir`
with `os.OpenRoot`. If the root cannot be opened, it returns an error and does
not register the route:

```go
if err := srv.HandleStaticChecked("/static/"); err != nil {
    return fmt.Errorf("mount static files: %w", err)
}
```

`HandleStatic(pattern)` remains as a deprecated v1-compatible wrapper. It logs
the setup error and leaves the route unregistered. New code should use the
checked method so a missing or inaccessible root stops startup explicitly.
There is no `http.Dir` fallback.

Static serving is GET/HEAD only. POST returns 405.

## Health endpoints

`WithHealthServer()` registers three endpoints on the health port:

| Path | 200 means | 503 means |
|---|---|---|
| `/healthz/` | Process is alive | Never returns 503 (process-level liveness only) |
| `/readyz/` | `isReady` is set | Deferred init not yet complete |
| `/livez/` | Server has not started shutdown | After `Stop()` is called |

`isReady` is set immediately after `NewServer` returns. The exception
is `WithDeferredInit(...)`. Under that option, `/readyz/` returns 503
until the deferred init callback returns nil, the `OnReady` hooks run,
and `CompleteDeferredInit(...)` is called (or it completes
automatically when init returns).

`/.well-known/mcp.json` and application routes live on the main
`Addr`, not the health port. The health port is a separate listener so
the orchestrator can probe without competing for the ingress queue.

### Deferred init

For applications that need slow startup work (cache warming, DB pool
open, remote config fetch) while serving `/healthz/` immediately:

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

- `/healthz/` and `/livez/` return 200.
- `/readyz/` returns 503.
- Application routes return 503 with a `Retry-After` header.
- Kubernetes keeps the pod out of the load balancer until readiness
  flips.

This is the standard Kubernetes split between "is this pod alive?"
(liveness; restart it if not) and "is this pod ready for traffic?"
(readiness; don't route to it yet).

## Rate limiting and auth

`WithRateLimit(rps, burst)` enables a per-remote-IP token bucket.
Per-client limiters live in a `sync.Map` that HyperServe sweeps every
5 minutes for entries not seen in the last 10. There is no global
limiter. Add one as middleware if you need one.

For application-level auth, validator hooks accept Bearer, Basic, and
API-key. See [examples/auth/](../examples/auth/) for a JWT example.
Validators run inside `subtle.WithDataIndependentTiming` to prevent
timing oracles.

If auth lives upstream (JWT-validating reverse proxy, Envoy filter,
OAuth2 proxy), HyperServe just passes headers through. No auth is
required by default; endpoints are public until you wire something.

## Logging

`log/slog` is the default. Plain text, stderr, INFO. For JSON in
production:

```go
import "log/slog"

handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
slog.SetDefault(slog.New(handler))
server.SetDefaultLogger(slog.New(handler))
```

Set both. The HyperServe package-level logger is separate from
`slog.Default()` so callers can silence framework chatter without
affecting application logs.

No request-correlation middleware ships in core. Write one if you
need it: set or propagate `X-Request-ID`, add it to the request
context. The old `TraceMiddleware` was deleted in v0.32 because it was
never wired into any preset, and the `trace_id` field it populated was
empty in every real deployment.

## Pre-deploy checklist

- [ ] `make check` passes (gofmt, vet, staticcheck, govulncheck,
      modernize, plus per-example govulncheck)
- [ ] `go test -race ./...` passes
- [ ] TLS enabled either in HyperServe (`WithTLS`) or the proxy
- [ ] `X-Forwarded-Proto: https` set by the proxy if TLS terminates
      upstream
- [ ] CDN honors `Vary: Authorization`, or `MCPDiscoveryPolicy` is
      `DiscoveryCount` / `DiscoveryNone`
- [ ] HSTS preload submitted at https://hstspreload.org/ if wanted
- [ ] Health server bound to an interface the ingress does NOT expose
- [ ] `MCPDev()` not present in any production preset
- [ ] `WithMCPFileToolRoot` set whenever `WithMCPBuiltinTools(true)` is
      set; otherwise file tools are skipped with a WARN log
- [ ] Every `HandleStaticChecked` error is handled during startup
- [ ] JSON logging configured if your aggregator needs it
- [ ] Request-correlation middleware added if you need one (none ships)

## See also

- [API_STABILITY.md](./API_STABILITY.md) — what's promised pre- and post-1.0
- [MCP_GUIDE.md](./MCP_GUIDE.md) — full MCP reference, namespaces, presets, SSE flow
- [WEBSOCKET_GUIDE.md](./WEBSOCKET_GUIDE.md) — origin checking, Sec-WebSocket-Key, frame limits
- [SECURITY.md](../SECURITY.md) — vulnerability reporting
- [examples/auth/](../examples/auth/) — JWT, Bearer, Basic, API-key, role gating
