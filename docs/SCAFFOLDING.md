# Scaffolding

`hyperserve-init` creates an application module with explicit lifecycle,
configuration, routes, security headers, and an application-owned rate-limit
gate.

## Install and generate

```sh
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@v2.1.4
hyperserve-init --module github.com/acme/payments
cd payments
go run ./cmd/server
```

Use `--out` to choose a directory and `--force` only when intentionally
replacing generated files. `--local-replace` is for HyperServe development
and release smoke tests; a generated project intended for commit or deployment
must not retain that replacement.

## Generated layout

```text
payments/
├── cmd/server/main.go
├── internal/app/
│   ├── config.go
│   ├── config_test.go
│   ├── routes.go
│   ├── server.go
│   └── server_test.go
├── configs/default.json
├── Dockerfile
├── README.md
├── go.mod
└── go.sum
```

The generated application may keep an application-level
`app.NewServer(cfg)` factory. That name belongs to the generated module; its
implementation constructs HyperServe with `hyperserve.New`.

## Rate-limit ownership

The generated application owns limiter configuration and translates it into
`ratelimit.Config`:

```go
apiLimit, err := ratelimit.New(ratelimit.Config{
    RequestsPerSecond: float64(cfg.RateLimit),
    Burst:             cfg.RateBurst,
})
if err != nil {
    return err
}

app.UsePrefix("/api", apiLimit)
```

Generated environment bindings use exactly:

- `HS_RATE_LIMIT`
- `HS_RATE_BURST`

`HS_BURST_LIMIT` is not a scaffold alias. It is a retired HyperServe
server-owned input and `hyperserve.WithEnvironment` rejects it. The generated
application parses its own limiter variables and must not pass them through as
root-server configuration.

The default peer-IP identity is appropriate only when the transport peer is the
client. A generated application placed behind a proxy must define reviewed
trusted proxy ranges and use `ratelimit.TrustedProxyClientKey`; the generator
cannot infer that trust boundary.

## MCP defaults

MCP is disabled by default. Enabling an MCP endpoint does not automatically
enable built-in tools or resources. Builtins additionally require an explicit
`mcp/builtin` import and an application authorization decision.

## Verification

A generated project should pass without workspace or replacement help:

```sh
GOWORK=off go test -mod=readonly ./...
grep -n '^replace ' go.mod
```

The second command should print nothing. Release verification additionally
generates from the public tag in a fresh module cache and confirms the module
graph resolves `github.com/osauer/hyperserve/v2 v2.1.4`.
