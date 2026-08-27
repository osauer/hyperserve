# Migrating to HyperServe v2.1

> [!WARNING]
> v2.1.0 is an explicitly controlled breaking reset within the existing
> `github.com/osauer/hyperserve/v2` module path. It is outside ordinary
> semantic-version compatibility. Complete this migration before upgrading.
> To roll back, pin `github.com/osauer/hyperserve/v2@v2.0.3`.

The reset removes the intermediate `pkg/...` public layout, gives the root
package the `hyperserve` name, renames the constructor to `New`, and makes
rate limiting standalone. No forwarding packages, aliases, or deprecated
facades remain.

## Upgrade only after changing source

```sh
go get github.com/osauer/hyperserve/v2@v2.1.0
GOWORK=off go mod tidy
```

Future breaking changes require a new major module path; v2.1.0 is not a new
minor-release policy.

## Package moves

| v2.0.3 import | v2.1.0 import |
|---|---|
| `github.com/osauer/hyperserve/v2/pkg/server` | `github.com/osauer/hyperserve/v2` |
| `github.com/osauer/hyperserve/v2/pkg/auth` | `github.com/osauer/hyperserve/v2/auth` |
| `github.com/osauer/hyperserve/v2/pkg/jsonrpc` | `github.com/osauer/hyperserve/v2/jsonrpc` |
| `github.com/osauer/hyperserve/v2/pkg/mcp` | `github.com/osauer/hyperserve/v2/mcp` |
| `github.com/osauer/hyperserve/v2/pkg/mcp/builtin` | `github.com/osauer/hyperserve/v2/mcp/builtin` |
| `github.com/osauer/hyperserve/v2/pkg/websocket` | `github.com/osauer/hyperserve/v2/websocket` |

Rate limiting is new at
`github.com/osauer/hyperserve/v2/ratelimit`.

Canonical imports:

```go
import (
    "github.com/osauer/hyperserve/v2"
    "github.com/osauer/hyperserve/v2/auth"
    "github.com/osauer/hyperserve/v2/jsonrpc"
    "github.com/osauer/hyperserve/v2/mcp"
    _ "github.com/osauer/hyperserve/v2/mcp/builtin"
    "github.com/osauer/hyperserve/v2/ratelimit"
    "github.com/osauer/hyperserve/v2/websocket"
)
```

Only import the packages an application uses. The `mcp/builtin` blank import
is required only when built-in presets or resources are enabled.

## Constructor

Before:

```go
app, err := server.NewServer(server.WithAddr(":8080"))
```

After:

```go
app, err := hyperserve.New(hyperserve.WithAddr(":8080"))
```

`NewServer` has no alias. `Server`, `Option`, `Options`, and
`Middleware` retain their roles in the root package.

An application-owned factory such as `app.NewServer(cfg)` may keep its name.
Its implementation must call `hyperserve.New`; it is not a compatibility
constructor in HyperServe.

## Standalone rate-limit gate

The following v2.0.3 surfaces are removed:

- `RateLimit`
- `WithRateLimit`
- `RateLimitMiddleware`
- `ClientLimiterCount`
- `Options.RateLimit` and `Options.Burst`
- server-owned client buckets and cleanup lifecycle

Replace them with:

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

Not attaching the returned middleware means disabled. Zero rate or burst is an
error, not a silent disable switch.

One returned value is one quota namespace. Reusing it intentionally shares
quotas, including across overlapping prefixes; the same gate charges at most
once per request. Separate `ratelimit.New` calls isolate quotas.

The default key is the normalized transport peer from `Request.RemoteAddr`
and ignores forwarding headers. A deployment behind known reverse proxies can
build an explicit key function:

```go
clientKey, err := ratelimit.TrustedProxyClientKey([]netip.Prefix{
    netip.MustParsePrefix("10.0.0.0/8"),
    netip.MustParsePrefix("2001:db8:1234::/48"),
})
if err != nil {
    return err
}

apiLimit, err := ratelimit.New(ratelimit.Config{
    RequestsPerSecond: 20,
    Burst:             40,
    ClientKey:         clientKey,
})
```

The helper accepts `X-Forwarded-For` only from a trusted immediate peer and
walks the chain from the trusted side. Malformed chains, a forwarding header
from an untrusted peer, and chains with no untrusted client fail closed.

Zero `IdleTTL` and `MaxClients` select finite defaults of 10 minutes and
10,000 clients. Cleanup is opportunistic; the gate starts no goroutine and has
no `Close`.

## Retired configuration fails visibly

`WithConfigFile` rejects top-level `rate_limit` and `burst` keys.
`WithEnvironment` rejects present `HS_RATE_LIMIT` and `HS_BURST_LIMIT`
variables, including empty values. The error points to `ratelimit.New`.

Move those values into application configuration and construct
`ratelimit.Config` explicitly. Generated applications can retain their own
rate and burst fields because the application, not HyperServe's `Server`,
owns that policy.

## MCP builtins

Imports move to `mcp` and `mcp/builtin`. The cycle-free registration
boundary is unchanged: the root package never imports builtins.

```go
import (
    "github.com/osauer/hyperserve/v2"
    _ "github.com/osauer/hyperserve/v2/mcp/builtin"
)

app, err := hyperserve.New(
    hyperserve.WithMCPSupport("MyApp", "1.0.0"),
    hyperserve.WithMCPBuiltinResources(true),
)
```

`MCPDev` and `MCPObservability` do not import builtins automatically.

## Lifecycle and middleware

The v2 lifecycle introduced before this reset remains:

- applications own signals and the root context;
- `Run(ctx)` follows that context;
- handlers use `r.Context()`;
- `Shutdown(ctx)` uses a caller-provided deadline;
- `Use` is global and `UsePrefix` is segment-aware.

No limiter cleanup is part of server shutdown.

## Verification

```sh
rg 'github.com/osauer/hyperserve/v2/pkg/|server\.NewServer|WithRateLimit|RateLimitMiddleware' --glob '*.go'
go list -m all | grep hyperserve
GOWORK=off go test -race -mod=readonly ./...
```

Expected dependency evidence:

- exactly `github.com/osauer/hyperserve/v2 v2.1.0`;
- no v1 HyperServe module;
- no local `replace`;
- no old `pkg/...` imports.

If migration must be reversed while investigating, pin the immutable previous
release:

```sh
go get github.com/osauer/hyperserve/v2@v2.0.3
```
