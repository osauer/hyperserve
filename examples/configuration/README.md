# Configuration precedence

HyperServe starts from deterministic defaults. An application may explicitly
bind a JSON file, the process environment, or a complete `Options` value. This
example answers the useful question: when the same server field appears in
several places, which value wins?

Server options apply from left to right; later options win:

1. `hyperserve.New` begins with deterministic defaults.
2. `WithConfigFile` overlays fields from the chosen JSON file.
3. `WithEnvironment` overlays its supported environment variables.
4. Functional options placed last establish application-owned values.

Put deployment-owned sources first and application invariants last. A bare
`hyperserve.New()` never reads `options.json`, `HS_CONFIG_PATH`, or `HS_*`.

## The demonstrated conflict

The example gives the listen address three values:

| Source | Address |
|---|---:|
| JSON file | `:8084` |
| Environment | `:8085` |
| Functional option | `:8086` |

```go
app, err := hyperserve.New(
	hyperserve.WithConfigFile("options.json"), // Application chooses the file.
	hyperserve.WithEnvironment(),              // Deployment may override it.
	hyperserve.WithAddr(":8086"),               // Application invariant wins.
)
```

Run it from the repository root:

```sh
go run ./examples/configuration
```

Expected values:

```text
After defaults, file, and environment:
  address: :8085

After the application-owned address option:
  address: :8086
  /api gate: 10 requests/second, burst 20
```

## Rate limiting is application policy

Middleware is a request wrapper. To limit a path, create a gate and then place
that gate in front of the path:

```go
apiPolicy := ratelimit.Config{
	RequestsPerSecond: 10,
	Burst:             20,
}
apiGate, err := ratelimit.New(apiPolicy)
if err != nil {
	log.Fatal(err)
}
app.UsePrefix("/api", apiGate)
```

One gate owns one quota namespace. Reuse it when paths should share quotas;
call `ratelimit.New` again when they should be isolated.

### Migrating retired limiter settings

Old server-owned `rate_limit` and `burst` JSON fields are rejected with a
migration error instead of being ignored. `WithEnvironment` likewise rejects
retired `HS_RATE_LIMIT` and `HS_BURST_LIMIT` variables when they are present.
Move those values into application configuration and translate them explicitly
to `ratelimit.Config`.

## Server configuration forms

A server JSON file can contain supported fields such as:

```json
{
  "addr": ":8080",
  "read_timeout": 30000000000
}
```

Durations in JSON are nanoseconds because they map to Go `time.Duration`.
`WithEnvironment()` can bind deployment values such as `HS_PORT`:

```sh
export HS_PORT=8080
```

Timeouts do not have environment bindings; keep them in JSON or set them with
`WithTimeouts` so their units and ownership remain visible in code.

See [`options.go`](../../options.go) for the complete server configuration
surface.
