# HyperServe Scaffolding

`hyperserve-init` bootstraps a production-grade HyperServe service with hardened defaults, MCP tooling enabled, and batteries-included middleware.

## Install the CLI

```bash
go install github.com/osauer/hyperserve/cmd/hyperserve-init@latest
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
- `--with-mcp` – Toggle MCP surfaces (defaults to `true`).
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
- The test suite uses `--local-replace` to avoid network fetches; you can mirror that locally via `hyperserve-init --local-replace $(pwd)` when running from the repository root.

## Next Templates

Additional templates (e.g. OTLP exporters, MCP runtime control bundles, or full application bundles) can live alongside the default in `internal/scaffold/templates`. Each template participates automatically in the CLI once added to the embedded filesystem.
