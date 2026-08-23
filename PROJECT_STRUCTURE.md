# HyperServe Project Structure

## Directory layout

```
hyperserve/
├── .github/workflows/    # CI/CD
├── benchmarks/           # Performance benchmarks (wrk-based)
├── cmd/
│   └── hyperserve-init/  # Project scaffolding CLI
├── docs/                 # ADRs and guides
├── examples/             # Self-contained `go run .` examples
├── internal/
│   ├── scaffold/         # Templates and generator backing hyperserve-init
│   └── validate/         # Struct-tag validator used by pkg/server.Validate
├── pkg/
│   ├── jsonrpc/          # JSON-RPC 2.0 engine
│   ├── mcp/              # MCP protocol surface (Handler, transports, discovery, namespaces)
│   ├── mcp/builtin/      # Opt-in built-in MCP tools and resources
│   ├── server/           # HTTP server, middleware, deferred-init lifecycle, MCP wiring
│   └── websocket/        # RFC 6455 WebSocket implementation
└── go.{mod,sum}
```

## Public packages

- `pkg/server` — HTTP server, middleware registry, deferred-init lifecycle, MCP wiring options.
- `pkg/mcp` — Standalone MCP protocol surface. No dependency on `pkg/server`.
- `pkg/mcp/builtin` — Optional built-in MCP tools (Calculator + sandboxed FileRead / ListDirectory when `WithMCPFileToolRoot` is set) and resources (Config, Metrics, System, ServerLog, ServerHealth). Blank-import to wire the `WithMCPBuiltinTools/Resources(true)` and `MCPDev()` / `MCPObservability()` presets. The previously-bundled `HTTPRequest` tool was removed (SSRF surface); `RequestDebuggerTool` was removed (credential capture).
- `pkg/websocket` — WebSocket server upgrader, outbound client, framing, and origin checks.
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

- `go.mod` / `go.sum` — Shipped module + single external dependency (`golang.org/x/time`).
- `tools/go.mod` / `tools/go.sum` — Developer-only modernize dependency graph.
- `Makefile` — `build` / `install` / `test` / `check` (runs `vet`, `staticcheck`, `modernize`, `govulncheck`).
- `README.md` — Overview and Quick Start.
- `ARCHITECTURE.md` — Design notes for the layered package layout.
- `CHANGELOG.md` — Release history.
- `CONTRIBUTING.md` — Contribution guidelines.

## Building & testing

```bash
make build         # builds cmd/hyperserve-init with version ldflags
make test          # go test -v ./...
make check         # vet + staticcheck + modernize + govulncheck
go test -bench=. ./pkg/server   # benchmarks
```
