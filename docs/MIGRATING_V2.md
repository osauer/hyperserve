# Migrating to HyperServe v2

HyperServe v2 combines the intended v1.7 cleanup with the breaking changes
that need a major module path. The result removes transitional names instead
of asking applications to migrate twice.

## 1. Change the module path

```sh
go get github.com/osauer/hyperserve/v2@latest
```

Update HyperServe imports to include `/v2`:

```go
import "github.com/osauer/hyperserve/v2/pkg/server"
```

## 2. Move lifecycle ownership to the application

`Run` now requires a context. HyperServe no longer installs process-signal
handlers inside the library:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

if err := srv.Run(ctx); err != nil {
	log.Fatal(err)
}
```

Use `RunStdio()` for MCP standard input/output and `Shutdown(ctx)` for an
explicit caller-owned shutdown deadline.

| v1 | v2 |
| --- | --- |
| `srv.Run()` | `srv.Run(ctx)` |
| `srv.RunContext(ctx)` | `srv.Run(ctx)` |
| `srv.Stop()` | `srv.Shutdown(ctx)` |
| `srv.Run()` with MCP stdio | `srv.RunStdio()` |

## 3. Use standard middleware shapes

Middleware now has the ordinary `net/http` signature:

```go
type Middleware func(http.Handler) http.Handler
```

Register middleware before serving requests:

| v1 | v2 |
| --- | --- |
| `srv.AddMiddleware("*", m)` | `srv.Use(m)` |
| `srv.AddMiddlewareStack("*", stack)` | `srv.Use(stack...)` |
| `srv.AddMiddleware("/api", m)` | `srv.UsePrefix("/api", m)` |
| `func(http.Handler) http.HandlerFunc` | `func(http.Handler) http.Handler` |

Prefix matching stops at a path boundary: `/api` matches `/api/users`, not
`/apiv2`.

## 4. Treat options as construction input

The shorter names are the v2 public surface:

| v1 | v2 |
| --- | --- |
| `ServerOptionFunc` | `Option` |
| `ServerOptions` | `Options` |
| `DefaultServerOptions()` | `DefaultOptions()` |
| mutate `srv.Options` | pass options to `NewServer`; inspect `srv.Options()` |
| `srv.HandleStaticChecked(pattern)` | `srv.HandleStatic(pattern)` |
| `WithSuppressBanner(false)` | `WithStartupBanner()` |
| `WithHardenedMode()` | omit `WithServerHeader` |
| `srv.Mux()` | `srv.Handler()` |
| call `DefaultMiddleware` | remove it; `NewServer` installs the defaults |
| `EnsureTrailingSlash` | use normal `path/filepath` or `strings` handling |

`Options()` returns an independent snapshot. Mutating it does not reconfigure
a live server. Zero values passed to `WithTimeouts` are now preserved, matching
`http.Server`; a zero timeout deliberately disables that deadline.

## 5. Inject logging per server

Replace process-global HyperServe logger changes with construction-time
injection:

```go
srv, err := server.NewServer(server.WithLogger(appLogger))
```

| v1 | v2 |
| --- | --- |
| `server.SetDefaultLogger(appLogger)` | `server.WithLogger(appLogger)` passed to `NewServer` |

`WithLogger` and `WithLogLevel` do not replace `slog.Default`, so two embedded
servers can use different logging policies safely.

## 6. Compose authentication explicitly

The server package no longer owns a bearer-token callback or a `SecureAPI`
bundle. Use the provider-neutral `auth` package and name each policy step:

```go
verifier := auth.TokenVerifierFunc(verifyToken)
bearerIdentity := auth.Bearer(verifier)
requireIdentity := auth.Require(bearerIdentity)

srv.UsePrefix("/api", requireIdentity, server.RateLimitMiddleware(srv))
```

`auth.Require` stores an `auth.Principal` identified by issuer and subject.
Your application still owns roles, resource permissions, browser sessions,
login redirects, and logout. See the
[federated authentication example](../examples/auth/) for an OpenID Connect
adapter implemented as a named type.

## Verification

After the mechanical import and name changes, run:

```sh
go test -race ./...
```

Pay particular attention to application shutdown, custom middleware return
types, direct `srv.Options` mutation, and any old `SecureAPI` assumptions.
