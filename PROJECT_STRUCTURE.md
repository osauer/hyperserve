# HyperServe Project Structure

## Directory layout

```
hyperserve/
├── .github/workflows/    # CI/CD
├── benchmarks/           # Performance benchmarks
├── cmd/
│   ├── example-server/   # Minimal binary used by benchmarks
│   ├── server/           # Feature-complete CLI wrapping the library
│   └── hyperserve-init/  # Project scaffolding CLI
├── configs/              # Configuration examples (JSON)
├── docs/                 # ADRs and guides
├── examples/             # Self-contained `go run .` examples
├── internal/
│   └── scaffold/         # Templates and generator backing hyperserve-init
├── pkg/
│   ├── jsonrpc/          # JSON-RPC 2.0 engine
│   ├── mcp/              # MCP protocol surface (Handler, transports, discovery, namespaces)
│   ├── mcp/builtin/      # Opt-in built-in MCP tools and resources
│   ├── server/           # HTTP server, middleware, deferred-init lifecycle, MCP wiring
│   └── websocket/        # RFC 6455 WebSocket implementation + pool
├── spec/                 # API spec + conformance tests
└── go.{mod,sum}
```

## Public packages

- `pkg/server` — HTTP server, middleware registry, interceptor chain, deferred-init lifecycle, MCP wiring options.
- `pkg/mcp` — Standalone MCP protocol surface. No dependency on `pkg/server`.
- `pkg/mcp/builtin` — Optional built-in MCP tools (Calculator, FileRead, HTTPRequest, ListDirectory) and resources (Config, Metrics, System, ServerLog, ServerHealth). Blank-import to wire the `WithMCPBuiltinTools/Resources(true)` and `MCPDev()` / `MCPObservability()` presets.
- `pkg/websocket` — WebSocket upgrader, low-level framing, connection pool, origin checks.
- `pkg/jsonrpc` — Standalone JSON-RPC 2.0 engine used by `pkg/mcp`.

## Import paths

```go
import (
    server   "github.com/osauer/hyperserve/pkg/server"
    mcp      "github.com/osauer/hyperserve/pkg/mcp"
    builtin  "github.com/osauer/hyperserve/pkg/mcp/builtin"   // blank-import if you use builtin presets
    ws       "github.com/osauer/hyperserve/pkg/websocket"
    jsonrpc  "github.com/osauer/hyperserve/pkg/jsonrpc"
)
```

## Root files

- `go.mod` / `go.sum` — Module + single transitive dependency (`golang.org/x/time`).
- `.golangci.yml` — Linter config.
- `Makefile` — `build` / `install` / `test` / `check` (runs `vet`, `staticcheck`, `modernize`, `govulncheck`).
- `README.md` — Overview and Quick Start.
- `ARCHITECTURE.md` — Design notes for the layered package layout.
- `CHANGELOG.md` — Release history.
- `CONTRIBUTING.md` — Contribution guidelines.

## Building & testing

```bash
make build         # builds cmd/server with version ldflags
make test          # go test -v ./...
make check         # vet + staticcheck + modernize + govulncheck
go test -bench=. ./pkg/server   # benchmarks
```
