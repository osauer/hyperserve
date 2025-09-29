# ADR-0009: Single Package Architecture

**Status:** Superseded  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Go projects often organize code into multiple packages:
- `server/` - Core server code
- `middleware/` - Middleware implementations
- `handlers/` - Built-in handlers
- `config/` - Configuration types

This creates a decision point about package organization with tradeoffs between modularity and simplicity.

## Decision

Adopt a layered package layout:
- Public APIs live under `pkg/server`, `pkg/websocket`, `pkg/jsonrpc`
- Internal helpers stay under `internal/`
- Root module only hosts binaries, docs, and tooling

```go
import (
    server "github.com/osauer/hyperserve/pkg/server"
    websocket "github.com/osauer/hyperserve/pkg/websocket"
)
```

## Consequences

### Positive
- **Clear ownership**: Server, JSON-RPC, and WebSocket surfaces evolve independently
- **Lean root**: Avoids dozens of top-level files and keeps binaries/examples obvious
- **Better docs**: Package README/API docs map directly to Go imports
- **Safer API design**: Explicit exports per package limit accidental surface growth
- **Better documentation**: Single godoc page

### Negative
- **Namespace pollution**: Everything in same namespace
- **Less modularity**: Can't import just middleware
- **Larger compile unit**: Entire package recompiles on changes
- **Testing complexity**: Test files in same package
- **Convention breaking**: Most Go projects use multiple packages

### Mitigation
- Clear naming conventions (MiddlewareFunc, HandlerFunc)
- Thoughtful API design to minimize exports
- Good documentation organization
- Consider splitting if package exceeds 5000 lines

## Implementation Details

File organization within single package:
```
hyperserve/
├── server.go        # Core server implementation
├── middleware.go    # All middleware functions
├── handlers.go      # Built-in handlers
├── options.go       # Configuration options
├── sse.go          # Server-sent events
├── doc.go          # Package documentation
└── *_test.go       # Test files
```

Naming conventions:
- Types: `Server`, `ServerOptions`, `MiddlewareFunc`
- Constructors: `NewServer`, `NewSSEMessage`
- Options: `WithPort`, `WithTLS`, `WithRateLimit`
- Middleware: `LoggingMiddleware`, `AuthMiddleware`

## Examples

```go
// Clean, single import
import "github.com/example/hyperserve"

func main() {
    // Everything available from server.*
    srv, err := server.NewServer(
        server.WithPort(8080),
        server.WithRateLimit(100, 200),
    )
    
    // Middleware from same package
    srv.AddMiddleware("*", server.LoggingMiddleware(srv.Options))
    srv.AddMiddleware("/api", server.AuthMiddleware(srv.Options))
    
    // Types from same package
    var handler server.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
        // ...
    }
    
    srv.Run()
}
```

## Future Considerations

If the package grows too large (>5000 lines), consider:
1. Moving examples to separate module
2. Creating `hyperserve/contrib` for optional middleware
3. Extracting utilities to `hyperserve/internal`

For now, the benefits of simplicity outweigh modularity concerns.