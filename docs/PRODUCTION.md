# Production deployment

_Last updated: 2026-09-05 for HyperServe v2.1.3._

This guide covers process ownership, reverse proxies, TLS, health checks, MCP,
rate limiting, and the checks to run before deploying a HyperServe application.

## Topology

A common deployment terminates public TLS at a reverse proxy and sends HTTP
over a private hop to HyperServe. The optional health listener should remain
private.

```text
clients -> CDN/WAF -> reverse proxy -> HyperServe :8080
                                     -> HyperServe :9080 (health, private)
```

```go
app, err := hyperserve.New(
	// Loopback is appropriate when the proxy runs on the same host. Use a
	// private interface when a proxy reaches the process over a VPC network.
	hyperserve.WithAddr("127.0.0.1:8080"),
	hyperserve.WithHealthServer(),
	hyperserve.WithHealthAddr("127.0.0.1:9080"),
)
if err != nil {
	log.Fatal(err)
}
```

In this topology the proxy owns the certificate and client-facing TLS
connection. Do not also enable direct TLS on HyperServe's private listener.

## Process lifetime and shutdown

The application owns operating-system signals and the root context. HyperServe
follows that context; it does not install signal handlers.

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()

if err := app.Run(ctx); err != nil {
	log.Fatal(err)
}
```

The context passed to `Run` is a shutdown trigger. Its values are not copied
into HTTP requests. Handlers use `r.Context()` for request cancellation and
request-scoped values.

`Run` performs bounded graceful cleanup when its context is cancelled. A
larger application that coordinates shutdown itself can supply its own
deadline:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := app.Shutdown(shutdownCtx); err != nil {
	log.Printf("graceful shutdown: %v", err)
}
```

Do not run one `Server` concurrently or reuse it after `Run` returns.

## Reverse proxies and client identity

### Forward the public scheme

When TLS terminates at a proxy, requests reach HyperServe without `r.TLS`.
MCP discovery uses `X-Forwarded-Proto` when it builds public endpoint URLs. Set
that header at the trusted proxy so HTTPS clients receive HTTPS discovery URLs.

HyperServe does not ship a general trusted-proxy allow-list. Bind its listener
so clients cannot bypass the proxy, and do not use forwarding headers as an
authentication decision.

### Preserve discovery cache boundaries

MCP discovery responses vary on `Authorization`. Ensure the CDN preserves
`Vary: Authorization`. If that is impossible, choose `mcp.DiscoveryCount` or
`mcp.DiscoveryNone`, whose anonymous response does not disclose tool or
resource names.

`mcp.DiscoveryAuthenticated` changes discovery presentation; it is not access
control. Protect `/mcp` with application authentication and authorization.

### Rate-limit identity is a separate trust decision

The default `ratelimit` identity is the normalized transport peer from
`Request.RemoteAddr`. It never trusts `X-Forwarded-For` or another forwarding
header. Behind a known proxy, opt in with validated `netip.Prefix` ranges:

```go
proxyRange := netip.MustParsePrefix("127.0.0.0/8")
clientKey, err := ratelimit.TrustedProxyClientKey([]netip.Prefix{proxyRange})
if err != nil {
	log.Fatal(err)
}

apiGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 20,
	Burst:             40,
	ClientKey:         clientKey,
})
if err != nil {
	log.Fatal(err)
}
app.UsePrefix("/api", apiGate)
```

`TrustedProxyClientKey` accepts `X-Forwarded-For` only when the immediate
transport peer is trusted. It parses the complete chain and walks from right to
left through trusted proxies to the first untrusted client. Malformed headers,
headers from an untrusted peer, and all-trusted chains fail closed. Configure
the proxy to replace or sanitize inbound forwarding headers.

## TLS and security headers

`WithFIPSMode()` selects AES-GCM suites for TLS 1.2 and P-256/P-384 curves.
It does not restrict TLS 1.3 cipher suites or enable a validated crypto module.
Applications requiring approved algorithms across TLS versions must configure
[Go FIPS mode](https://go.dev/doc/security/fips140) at build and process startup.
HyperServe does not change that process-wide policy.

`WithTLS(certFile, keyFile)` enables the direct TLS listener and validates that
both files exist during construction:

```go
app, err := hyperserve.New(
	hyperserve.WithTLS("cert.pem", "key.pem"),
)
if err != nil {
	log.Fatal(err)
}
```

Direct TLS listens on `Options.TLSAddr` (`:8443` by default), not the plaintext
`Addr`. The current TLS configuration is implemented in
[`options.go`](../options.go). If a reverse proxy terminates TLS, configure
HSTS there. If HyperServe terminates TLS, install `SecureWeb(app.Options())`
or the specific header middleware your application requires.

Review `includeSubDomains` and preload policy before sending an HSTS header;
both affect names outside this one process.

## Health and readiness

`WithHealthServer()` starts a separate listener with these endpoints:

| Path | `200` means | `503` means |
|---|---|---|
| `/healthz/` | process is alive | not used for ordinary dependency health |
| `/readyz/` | the application is ready | deferred initialization is incomplete |
| `/livez/` | shutdown has not begun | cancellation or shutdown has begun |

Application routes and `/.well-known/mcp.json` stay on the main listener.
Keep the health listener outside public ingress.

### Deferred initialization

Use deferred initialization for startup work that may be slow while the health
listener remains available:

```go
app, err := hyperserve.New(
	hyperserve.WithDeferredInit(func(ctx context.Context, app *hyperserve.Server) error {
		return slowBootstrap(ctx)
	}),
	hyperserve.WithOnReady(func(ctx context.Context, app *hyperserve.Server) error {
		log.Println("application is ready")
		return nil
	}),
)
```

Until initialization and ready hooks succeed, `/readyz/` and application
routes return `503`; health and liveness remain available. The default is to
stop if deferred initialization fails.

## MCP production boundary

Enable MCP explicitly and protect the endpoint like any other privileged API.
The full protocol and transport behavior is in the [MCP guide](./MCP_GUIDE.md).

### Discovery and Origin

Browser requests with an `Origin` header must match the request scheme, host,
and port by default. A trusted cross-origin client can use an explicit
validator:

```go
hyperserve.WithMCPOriginValidator(func(r *http.Request) bool {
	return r.Header.Get("Origin") == "https://trusted.example"
})
```

Origin validation is not authentication.

Streamable HTTP uses POST on `/mcp`. Finite requests return JSON;
`subscriptions/listen` returns request-scoped SSE. Configure reverse proxies
to preserve streaming, avoid response buffering or transformation, and allow
idle periods beyond the stream keepalive interval.

The proprietary routed-SSE compatibility transport is deprecated and disabled
by default. Enable `WithMCPLegacyRoutedSSE(true)` only for an existing client
while it migrates. Its `bindingToken` is a capability: never log
`X-SSE-Binding`. The implementation is in
[`mcp/transport_sse.go`](../mcp/transport_sse.go).

### Built-in tools and resources

Built-ins are off by default and require an explicit blank import. That import
registers hooks without making the root package depend on `mcp/builtin`:

```go
import (
	"github.com/osauer/hyperserve/v2"
	_ "github.com/osauer/hyperserve/v2/mcp/builtin"
)

app, err := hyperserve.New(
	hyperserve.WithMCPSupport("service", "1.0.0"),
	hyperserve.WithMCPBuiltinTools(true),
	hyperserve.WithMCPBuiltinResources(true),
	hyperserve.WithMCPFileToolRoot("/srv/data"),
)
```

File tools require `WithMCPFileToolRoot`; there is no unrestricted fallback.
The built-in outbound HTTP and request-debugger shapes were removed because
they would have exposed SSRF and credential-capture capabilities. Applications
that need outbound access should register a narrowly allow-listed tool.

Do not use `MCPDev()` in production. It exposes runtime status, registered
routes, middleware layout, and development logs. For narrower read-only
operational resources, configure `MCPObservability()` and still apply normal
endpoint authorization.

## Static files

Static serving is capability-scoped to the configured root:

```go
app, err := hyperserve.New(hyperserve.WithStaticDir("./static"))
if err != nil {
	return err
}
if err := app.HandleStatic("/static/"); err != nil {
	return fmt.Errorf("mount static files: %w", err)
}
```

An absent or inaccessible root fails registration. There is no working-directory
fallback. Static serving accepts GET and HEAD; other methods receive `405`.

## Rate limiting and authentication

HyperServe's root package does not own a limiter. Create a gate with
`ratelimit.New`, then place it with `Use` or `UsePrefix`. One returned gate is
one quota namespace: reusing it deliberately shares quotas, while separate
`New` calls isolate them. Mounting the same gate on overlapping prefixes
charges a request at most once.

The store is bounded. Zero `IdleTTL` and `MaxClients` select finite defaults of
10 minutes and 10,000 clients. `IdleTTL` is a minimum: a bucket remains until
its full burst could refill, so pruning cannot reset a slow quota early.
Expired entries are pruned opportunistically; there is no cleanup goroutine or
`Close`. At capacity, a new key receives `429` instead of evicting an active
bucket, while existing keys continue. Quota retry/reset information follows
the actual token schedule; capacity rejection uses the effective retention as
a conservative backoff.

The retired root configuration keys `rate_limit` and `burst`, and environment
variables `HS_RATE_LIMIT` and `HS_BURST_LIMIT`, fail during construction when
their source is bound. Move policy into `ratelimit.Config`; do not delete an
old setting and assume throttling still exists.

The [`auth`](../auth/) package provides a bearer-principal boundary. The
application still owns identity provider integration, roles, permissions,
sessions, and login flows. If authentication lives at the proxy, constrain
direct access to HyperServe and make the header handoff explicit.

## Logging

HyperServe uses `log/slog`. Inject a server-specific logger at construction:

```go
handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
})
appLogger := slog.New(handler)

app, err := hyperserve.New(hyperserve.WithLogger(appLogger))
```

`WithLogger` affects that HyperServe instance and its MCP handler. It does not
replace the process-wide `slog.Default()`. Request correlation is application
middleware: propagate or create an ID, then place it in `r.Context()` and log
records.

## Pre-deploy checklist

- [ ] `make check` passes.
- [ ] `go test -race ./...` passes.
- [ ] TLS terminates exactly where intended.
- [ ] Direct access cannot bypass the trusted reverse proxy.
- [ ] Forwarding headers are replaced or sanitized at that proxy.
- [ ] `Vary: Authorization` is preserved for MCP discovery.
- [ ] The health listener is private.
- [ ] `MCPDev()` is absent from production configuration.
- [ ] Built-in file tools have an explicit safe root.
- [ ] Every static-route registration error is handled.
- [ ] Each rate-limit gate has an intentional path and client identity.
- [ ] Retired rate settings have been migrated rather than removed silently.
- [ ] Logging and request correlation match operational requirements.

## See also

- [API stability](./API_STABILITY.md)
- [MCP integration](./MCP_GUIDE.md)
- [WebSocket guide](./WEBSOCKET_GUIDE.md)
- [Security policy](../SECURITY.md)
- [v2.1 migration](./MIGRATING_V2_1.md)
