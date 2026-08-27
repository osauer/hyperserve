# Migrating from HyperServe v1

This guide moves a v1 application directly to the current v2.1 API. If an
application already uses v2.0.x, use
[Migrating to v2.1](./MIGRATING_V2_1.md) instead.

> [!WARNING]
> v2.1.0 also contains a dated, controlled breaking reset inside `/v2`, outside
> ordinary semantic-version compatibility. This guide targets the final API so
> a v1 application migrates only once. Existing v2 users must follow the v2.1
> guide before upgrading; their rollback pin is
> `github.com/osauer/hyperserve/v2@v2.0.3`. Future breaking changes require a
> new major module path.

## 1. Change the module requirement

```sh
go get github.com/osauer/hyperserve/v2@v2.1.0
```

Replace v1 imports with the canonical v2 packages:

| v1 | v2.1 |
|---|---|
| `github.com/osauer/hyperserve/pkg/server` | `github.com/osauer/hyperserve/v2` |
| `github.com/osauer/hyperserve/pkg/auth` | `github.com/osauer/hyperserve/v2/auth` |
| `github.com/osauer/hyperserve/pkg/jsonrpc` | `github.com/osauer/hyperserve/v2/jsonrpc` |
| `github.com/osauer/hyperserve/pkg/mcp` | `github.com/osauer/hyperserve/v2/mcp` |
| `github.com/osauer/hyperserve/pkg/mcp/builtin` | `github.com/osauer/hyperserve/v2/mcp/builtin` |
| `github.com/osauer/hyperserve/pkg/websocket` | `github.com/osauer/hyperserve/v2/websocket` |

Then remove any local replacement and run:

```sh
GOWORK=off go mod tidy
GOWORK=off go test -mod=readonly ./...
```

## 2. Construct the root package

Before:

```go
options := server.DefaultServerOptions()
options.Addr = "127.0.0.1:8080"

server.SetDefaultLogger(appLogger)
appServer, err := server.NewServer(server.WithOptions(options))
```

After:

```go
options := hyperserve.DefaultOptions()
options.Addr = "127.0.0.1:8080"

app, err := hyperserve.New(
    hyperserve.WithOptions(options),
    hyperserve.WithLogger(appLogger),
)
```

Logger authority is instance-scoped. There is no process-wide
`SetDefaultLogger` mutation.

Other v1 names map directly to the final root package:

| v1 | v2.1 |
|---|---|
| `ServerOptionFunc` | `hyperserve.Option` |
| `ServerOptions` | `hyperserve.Options` |
| `DefaultServerOptions()` | `hyperserve.DefaultOptions()` |
| mutate `appServer.Options` | pass options at construction; inspect `app.Options()` |
| `HandleStaticChecked(pattern)` | `app.HandleStatic(pattern)` |
| `WithSuppressBanner(false)` | `hyperserve.WithStartupBanner()` |
| `WithHardenedMode()` | omit `WithServerHeader` |
| `appServer.Mux()` | `app.Handler()` |
| call `DefaultMiddleware` | remove it; `hyperserve.New` installs the defaults |
| `EnsureTrailingSlash` | use normal `path/filepath` or `strings` handling |

`Options()` returns an independent snapshot. Mutating that value does not
reconfigure the application. Zero values passed to `WithTimeouts` retain the
ordinary `http.Server` meaning of disabling that deadline.

## 3. Make middleware placement explicit

| v1 | v2.1 |
|---|---|
| `AddMiddleware("*", middleware)` | `Use(middleware)` |
| `AddMiddlewareStack("*", stack)` | `Use(stack...)` |
| `AddMiddleware("/api", middleware)` | `UsePrefix("/api", middleware)` |
| `AddMiddlewareStack(GlobalMiddlewareRoute, middleware)` | `Use(middleware)` |
| `AddMiddlewareStack("/api", middleware)` | `UsePrefix("/api", middleware)` |
| `func(http.Handler) http.HandlerFunc` | `func(http.Handler) http.Handler` |
| mutable `Options` field | independent `Options()` snapshot |

```go
app.Use(hyperserve.SecureWeb(app.Options()))
app.UsePrefix("/api", requireIdentity)
```

`UsePrefix` is segment-aware: `/api` matches `/api/users`, not
`/apiv2`.

## 4. Move rate limiting to its package

The root `Server` no longer owns rate, burst, client maps, or a cleanup
goroutine. Create a gate and place it in front of a path:

```go
apiLimit, err := ratelimit.New(ratelimit.Config{
    RequestsPerSecond: 20,
    Burst:             40,
})
if err != nil {
    return err
}

app.UsePrefix("/api", apiLimit)
```

Import `github.com/osauer/hyperserve/v2/ratelimit`. Reuse one returned
middleware value to share quotas; call `New` again for a separate namespace.

Do not mechanically add a limiter during migration. Decide which paths share a
quota and what identifies a client. The default key uses `RemoteAddr` and
does not trust forwarding headers.

## 5. Compose authentication and rate policy explicitly

The root package does not own a bearer callback, an authorization bundle, or a
limiter. Establish identity with `auth`, then place application authorization
and the gate from the previous step in the intended order:

```go
verifier := auth.TokenVerifierFunc(verifyToken)
requireIdentity := auth.Require(auth.Bearer(verifier))

app.UsePrefix("/api", requireIdentity, apiLimit)
```

`auth.Require` stores an issuer/subject `auth.Principal`. The application still
owns roles, resource permissions, sessions, login, and logout. See the
[federated authentication example](../examples/auth/) for an OpenID Connect
adapter.

## 6. Hand lifecycle authority to the application

| v1 | v2.1 |
|---|---|
| `Run()` | `Run(ctx)` |
| `RunContext(ctx)` | `Run(ctx)` |
| `Stop()` | `Shutdown(ctx)` |
| `Run()` with MCP stdio | `RunStdio()` |
| library-owned signal behavior | application-owned signal/root context |

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

if err := app.Run(ctx); err != nil {
    return err
}
```

Tests should create a fresh bounded cleanup context for `Shutdown`; do not
reuse a context that is already cancelled.

## 7. Preserve request behavior

The migration does not require replacing application-owned HTTP semantics:

- handlers still receive `r.Context()`;
- `Handler()` remains an ordinary `http.Handler`;
- SSE writers must still check writes and flushes;
- WebSocket upgrade and outbound-dial policy remain explicit;
- ambient `HS_*` values are ignored unless `WithEnvironment` is selected.

Retired server-owned limiter inputs fail visibly when their source is selected:
`rate_limit`, `burst`, `HS_RATE_LIMIT`, and `HS_BURST_LIMIT`. Translate
them into application-owned `ratelimit.Config`.

## 8. Verify the dependency graph

```sh
go list -m all | grep hyperserve
rg 'hyperserve(/v2)?/pkg/|(?:hyperserve|server|serverpkg)\.NewServer\(|RunContext|SetDefaultLogger' --glob '*.go'
GOWORK=off go test -race -mod=readonly ./...
```

The final graph should contain one HyperServe module,
`github.com/osauer/hyperserve/v2 v2.1.0`, with no v1 module and no local
`replace`.
