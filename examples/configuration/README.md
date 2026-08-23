# Configuration precedence

HyperServe can load defaults, a JSON file, environment variables, and
functional options. This example answers the important question: when the same
field appears in several places, which value wins?

From highest to lowest priority:

1. Functional options passed to `server.NewServer`
2. Environment variables
3. A JSON configuration file
4. HyperServe defaults

Functional options run last. Use them for application invariants; use files and
environment variables for deployment-owned values that the application leaves
open.

## The demonstrated conflict

The example intentionally gives the address three values:

| Source | Address | Rate | Burst |
|---|---:|---:|---:|
| JSON file | `:8084` | 75 | 150 |
| Environment | `:8085` | — | — |
| Functional options | `:8086` | 10 | 20 |

First, `NewServerOptions` loads the file and environment. Then `NewServer`
applies the functional options:

```go
loaded := server.NewServerOptions()
// loaded.Addr == ":8085"; environment overrides the file.

srv, err := server.NewServer(
    server.WithAddr(":8086"),
    server.WithRateLimit(10, 20),
)
// srv.Options.Addr == ":8086"; functional options run last.
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

Set `HS_CONFIG_PATH` to load a named JSON file; otherwise HyperServe checks
`options.json`:

```json
{
  "addr": ":8080",
  "rate_limit": 100,
  "burst": 200,
  "read_timeout": 30000000000
}
```

Durations in JSON are nanoseconds because they map to Go `time.Duration`.
Environment duration values use Go syntax such as `30s`:

```bash
export HS_PORT=8080
export HS_RATE_LIMIT=100
export HS_BURST_LIMIT=200
export HS_READ_TIMEOUT=30s
```

Prefer functional options when a value must not be changed by ambient process
configuration:

```go
srv, err := server.NewServer(
    server.WithAddr(":8080"),
    server.WithTimeouts(30*time.Second, 30*time.Second, 2*time.Minute),
)
```

See [`pkg/server/options.go`](../../pkg/server/options.go) for the complete JSON
fields, environment variables, and functional options.
