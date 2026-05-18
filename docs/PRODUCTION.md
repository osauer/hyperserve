# Production Deployment Guide

_Last updated: 2026-05-18 21:28 CEST (v0.33.1)._

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
everywhere) was a cache-poisoning bug fixed in v0.33.0: a CDN keyed on
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

### SSE binding-token capability

SSE clients receive a `clientId` and a `bindingToken` in the initial
`connection` event. POSTs that should be delivered via that SSE stream
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
HyperServe prints a startup banner: `⚠️ MCP DEVELOPER MODE ENABLED ⚠️`.
Wire your production CI to fail the build whenever that line appears.

For production observability, use `server.MCPObservability()`. It
exposes `config://server/current`, `health://server/status`, and
`logs://server/recent` with no control-plane mutation.

## Static files

`HandleStatic(pattern)` sandboxes file serving to `Options.StaticDir`
via `os.OpenRoot`. If `OpenRoot` fails at startup (directory missing,
permission denied), it falls back to `http.FileServer(http.Dir(...))`
and logs:

```
WARN  Failed to open static root directory, falling back to http.Dir
```

Don't ship if you see that line. The fallback exists for local
development. An unsandboxed file server is one path-traversal bug away
from leaking `/etc/passwd`. Fix the directory and start over.

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

No request-correlation middleware ships in v0.33. Write one if you
need it: set or propagate `X-Request-ID`, add it to the request
context. The old `TraceMiddleware` was deleted in v0.32 because it was
never wired into any preset, and the `trace_id` field it populated was
empty in every real deployment.

## Pre-deploy checklist

- [ ] `make check` passes (gofmt, vet, staticcheck, govulncheck,
      modernize, plus per-example govulncheck since v0.33.1)
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
- [ ] Startup log shows "Static file serving using secure os.Root",
      not "falling back to http.Dir"
- [ ] JSON logging configured if your aggregator needs it
- [ ] Request-correlation middleware added if you need one (none ships)

## See also

- [API_STABILITY.md](./API_STABILITY.md) — what's promised pre- and post-1.0
- [MCP_GUIDE.md](./MCP_GUIDE.md) — full MCP reference, namespaces, presets, SSE flow
- [WEBSOCKET_GUIDE.md](./WEBSOCKET_GUIDE.md) — origin checking, Sec-WebSocket-Key, frame limits
- [SECURITY.md](../SECURITY.md) — vulnerability reporting
- [examples/auth/](../examples/auth/) — JWT, Bearer, Basic, API-key, role gating
