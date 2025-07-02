# Hyperserve Examples Guide

This guide defines the idiomatic principles that all hyperserve examples must follow.

## Core Principles

### 1. Leverage Defaults
```go
// ❌ BAD
srv, _ := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),      // Already default
    hyperserve.WithHealthServer(),     // Already default
)

// ✅ GOOD  
srv, _ := hyperserve.NewServer()      // Uses sensible defaults
```

### 2. Functional Options Only
```go
// ❌ BAD
server, _ := hyperserve.NewServer()
server.Options.TemplateDir = "./templates"

// ✅ GOOD
srv, _ := hyperserve.NewServer(
    hyperserve.WithTemplateDir("./templates"),
)
```

### 3. Use Built-in Handlers
```go
// ❌ BAD
srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    html := `<!DOCTYPE html>...`
    fmt.Fprint(w, html)
})

// ✅ GOOD
srv.HandleTemplate("/", "index.html", pageData)
```

### 4. Simple Shutdown
```go
// ❌ BAD: Manual signal handling
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT)
// ... complex shutdown

// ✅ GOOD: Run() handles everything
srv.Run()
```

### 5. Show Middleware When Relevant
```go
// Only add middleware that demonstrates the feature
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))
```

### 6. Separate Concerns
```go
func main() {
    setupSampleData()  // Separate function
    
    srv, _ := hyperserve.NewServer(...)
    srv.HandleTemplate("/", "home.html", nil)
    srv.Run()
}
```

### 7. Minimal Error Handling
```go
if err := srv.Run(); err != nil {
    log.Fatal("Server failed:", err)
}
```

### 8. Feature-Focused
- Each example demonstrates ONE primary hyperserve feature
- Keep code minimal and focused
- Use comments to explain non-obvious behavior

### 9. Use Hyperserve Types
```go
// ❌ BAD
fmt.Fprintf(w, "data: %s\n\n", json)

// ✅ GOOD  
msg := hyperserve.NewSSEMessage(data)
fmt.Fprint(w, msg)
```

### 10. Document Environment Options
```go
// Can be configured via environment:
// HS_PORT=9000 HS_TLS_CERT_FILE=cert.pem ./example
```

## Example Structure

```
examples/feature-name/
├── main.go              # Minimal, focused implementation
├── templates/           # If using templates
│   └── index.html      # Clean, simple templates
└── README.md           # Brief description + usage
```

## Checklist for Examples

- [ ] Only sets non-default options
- [ ] Uses functional options pattern
- [ ] Leverages built-in handlers
- [ ] Demonstrates specific feature clearly
- [ ] Minimal lines of code
- [ ] Clean separation of concerns
- [ ] Uses hyperserve helpers/types
- [ ] Simple error handling
- [ ] No manual shutdown orchestration
- [ ] Comments explain the feature, not Go basics