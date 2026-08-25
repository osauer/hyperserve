# HyperServe Scaffolding

`hyperserve-init` generates a runnable HyperServe service: `cmd/server`, a config
loader, security headers and rate-limit middleware applied per route, optional
MCP, and a Distroless Dockerfile. The output compiles and `go test ./...` passes.
Generation downloads the complete module graph and writes `go.sum`; it therefore
needs network access unless the modules are already cached or a local replacement
is used.

## Install the CLI

```bash
go install github.com/osauer/hyperserve/v2/cmd/hyperserve-init@latest
```

## Generate a Service

```bash
hyperserve-init \
  --module github.com/acme/payments \
  --name "Acme Payments" \
  --out payments

cd payments
go run ./cmd/server
```

### Flags

- `--module` *(required)* – Go module path for the new project.
- `--name` – Human-friendly display name (defaults to the module tail).
- `--out` – Output directory (defaults to the service name).
- `--with-mcp` – Enable MCP surfaces (defaults to `false`). The generated
  endpoint includes operational built-ins; add application authorization
  middleware around `/mcp` before exposing it in production.
- `--force` – Allow generation into a non-empty directory.
- `--local-replace` – Add a `replace` directive pointing at a local HyperServe checkout (useful for development and the automated tests).

## Generated Layout

```
├── cmd/server/main.go        # Entry point wiring config, middleware, and routes
├── internal/app/config.go    # JSON + environment configuration loader
├── internal/app/server.go    # HyperServe setup with hardened defaults
├── internal/app/routes.go    # Example HTML + JSON endpoints
├── configs/default.json      # Opinionated defaults (addr, MCP, rate limits)
├── Makefile                  # run/build/test/docker recipes
├── Dockerfile                # Distroless image builder
├── go.mod / go.sum           # Ready for go modules (with X/time pre-pinned)
└── README.md                 # Getting started instructions
```

## Testing the Scaffold

- `go test ./internal/scaffold` runs the generator integration test, which verifies the CLI builds a compilable project and that `go test ./...` succeeds inside the scaffolded tree.
- The test suite uses `--local-replace` to avoid fetching HyperServe itself; you
  can mirror that locally via `hyperserve-init --local-replace $(pwd)` when
  running from the repository root.

## Next Templates

Additional templates (e.g. OTLP exporters, MCP runtime control bundles, or full application bundles) can live alongside the default in `internal/scaffold/templates`. Each template participates automatically in the CLI once added to the embedded filesystem.
