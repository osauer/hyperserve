# Configuration precedence

HyperServe starts from deterministic defaults. An application may then bind a
JSON file, process environment, or a complete `ServerOptions` value explicitly.
This example answers the important question: when the same field appears in
several places, which value wins?

Options apply from left to right; later options win:

1. `NewServer` begins with deterministic defaults.
2. `WithConfigFile` overlays the fields present in the chosen JSON file.
3. `WithEnvironment` overlays its supported environment variables.
4. Functional options placed last establish application-owned values.

Put deployment-owned sources first and application invariants last. A bare
`NewServer()` never reads `options.json`, `HS_CONFIG_PATH`, or `HS_*`.

## The demonstrated conflict

The example intentionally gives the address three values:

| Source | Address | Rate | Burst |
|---|---:|---:|---:|
| JSON file | `:8084` | 75 | 150 |
| Environment | `:8085` | — | — |
| Functional options | `:8086` | 10 | 20 |

The application names both external sources, then applies its fixed values:

```go
srv, err := server.NewServer(
	server.WithConfigFile("options.json"), // Baseline chosen by the application.
	server.WithEnvironment(),              // Deployment may override the baseline.
	server.WithAddr(":8086"),
	server.WithRateLimit(10, 20),
)
// Address is :8086 because the application invariant runs last.
```

Run it from the repository root:

```bash
go run ./examples/configuration
```

Expected values:

```text
After defaults, file, and environment:
  address: :8085
  rate:    75 requests/second
  burst:   150

After programmatic options:
  address: :8086
  rate:    10 requests/second
  burst:   20
```

## Configuration forms

Pass a chosen path to `WithConfigFile`:

```json
{
  "addr": ":8080",
  "rate_limit": 100,
  "burst": 200,
  "read_timeout": 30000000000
}
```

Durations in JSON are nanoseconds because they map to Go `time.Duration`.
Pass `WithEnvironment()` to bind supported process variables:

```bash
export HS_PORT=8080
export HS_RATE_LIMIT=100
export HS_BURST_LIMIT=200
```

Timeouts do not have environment bindings; keep them in JSON or set them with
`WithTimeouts` so their units and ownership remain visible in code.

Prefer functional options after external sources when a value must not be
changed by deployment configuration:

```go
srv, err := server.NewServer(
	server.WithConfigFile(configPath),
	server.WithEnvironment(),
	server.WithAddr(":8080"),
	server.WithTimeouts(30*time.Second, 30*time.Second, 2*time.Minute),
)
```

See [`pkg/server/options.go`](../../pkg/server/options.go) for the complete JSON
fields, environment variables, and functional options.
