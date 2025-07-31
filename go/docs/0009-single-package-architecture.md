# ADR-0009: Single Package Architecture

**Status:** Accepted  
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

Keep all hyperserve code in a single package:
- Everything exports from the `hyperserve` package
- No internal sub-packages
- All types and functions at the top level

```go
// Single import for everything
import "github.com/example/hyperserve"

// Not multiple imports
// import "github.com/example/hyperserve/server"
// import "github.com/example/hyperserve/middleware"
```

## Consequences

### Positive
- **Simple imports**: One import gives access to everything
- **Discoverability**: All APIs visible in one place
- **No circular dependencies**: Common problem with multi-package
- **Easier testing**: Can test unexported functions
- **Smaller API surface**: Forces thoughtful exports
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
    // Everything available from hyperserve.*
    srv, err := hyperserve.NewServer(
        hyperserve.WithPort(8080),
        hyperserve.WithRateLimit(100, 200),
    )
    
    // Middleware from same package
    srv.AddMiddleware("*", hyperserve.LoggingMiddleware(srv.Options))
    srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))
    
    // Types from same package
    var handler hyperserve.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
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