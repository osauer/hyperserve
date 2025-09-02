# HyperServe Examples

This directory contains examples demonstrating various features and use cases of HyperServe. 
Examples are numbered to suggest a learning path from simple to complex.

## 🎯 Quick Start Path

If you're new to HyperServe, follow this progression:

1. **[hello-world](hello-world/)** - Your first HyperServe server
2. **[static-files](static-files/)** - Serving HTML, CSS, and JavaScript
3. **[json-api](json-api/)** - Building a REST API with JSON
4. **[middleware-basics](middleware-basics/)** - Adding middleware layer by layer
5. **[configuration](configuration/)** - Configuration methods and precedence

## 📚 All Examples

### Beginner Examples

#### [hello-world](hello-world/) ⭐
The simplest possible HyperServe server. Start here!
- Single route returning "Hello, World!"
- Minimal code with detailed comments
- **Run:** `go run examples/hello-world/main.go`

#### [static-files](static-files/) ⭐
Serve a static website with HyperServe.
- HTML, CSS, and JavaScript files
- Directory structure best practices
- Security headers for static content
- **Run:** `go run examples/static-files/main.go`

#### [json-api](json-api/) ⭐⭐
Build a REST API with JSON request/response handling.
- CRUD operations for a TODO list
- Request parsing and validation
- Error handling patterns
- **Run:** `go run examples/json-api/main.go`

### Intermediate Examples

#### [middleware-basics](middleware-basics/) ⭐⭐
Learn how middleware works by building up a stack.
- Start with no middleware
- Add logging, rate limiting, CORS step by step
- Understand middleware ordering and composition
- **Run:** `go run examples/middleware-basics/main.go`

#### [configuration](configuration/) ⭐⭐
Master HyperServe's configuration system.
- Environment variables
- JSON configuration files
- Programmatic options
- Configuration precedence
- **Run:** `go run examples/configuration/main.go`

#### [auth](auth/) ⭐⭐⭐
Production-ready authentication with multiple methods.
- JWT (RS256), API Keys, and Basic Auth
- Rate limiting per token
- RBAC with roles and permissions
- Environment-specific configurations
- Comprehensive audit logging
- **Run:** `go run examples/auth/main.go`

#### [chaos](chaos/) ⭐⭐⭐
Test your application's resilience with chaos engineering.
- Simulated failures and latency
- Load testing with concurrent clients
- Observability during chaos
- **Run:** `go run examples/chaos/main.go`

### Advanced Examples

#### [htmx-dynamic](htmx-dynamic/) ⭐⭐⭐
Dynamic web applications with HTMX.
- Server-side rendering with templates
- HTMX attributes for interactivity
- No JavaScript framework needed
- **Run:** `go run examples/htmx-dynamic/main.go`

#### [htmx-stream](htmx-stream/) ⭐⭐⭐⭐
Real-time updates with Server-Sent Events.
- SSE for live data streaming
- HTMX integration for UI updates
- Graceful connection handling
- **Run:** `go run examples/htmx-stream/main.go`

#### [enterprise](enterprise/) ⭐⭐⭐⭐⭐
Enterprise-grade security features (Go 1.24+ required).
- FIPS 140-3 compliance
- TLS with ECH (Encrypted Client Hello)
- Post-quantum cryptography
- Full security middleware stack
- **Setup:** See [enterprise/README.md](enterprise/README.md) for certificate generation
- **Run:** `cd examples/enterprise && ./generate_certs.sh && go run main.go`

#### [mcp](mcp/) ⭐⭐⭐⭐
Model Context Protocol (MCP) support for AI assistants.
- JSON-RPC 2.0 protocol implementation
- Built-in tools (calculator, file operations, HTTP)
- Built-in resources (config, metrics, system info)
- Secure sandboxed file access
- **Run:** `go run examples/mcp/main.go`

## 🚀 Running Examples

All examples can be run directly:

```bash
# From the project root
go run examples/hello-world/main.go

# Or navigate to the example directory
cd examples/hello-world
go run main.go
```

Most examples run on port 8080 by default. The enterprise example uses 8443 for HTTPS.

## 📝 Testing Examples

Each example can be tested with curl:

```bash
# Hello World
curl http://localhost:8080/

# Static Files
curl http://localhost:8080/index.html

# JSON API
curl -X POST http://localhost:8080/todos \
  -H "Content-Type: application/json" \
  -d '{"title":"Learn HyperServe"}'
```

## 🎓 Learning Tips

1. **Start Simple**: Don't jump to advanced examples. The progression is designed to build on previous concepts.

2. **Read the Code**: Each example has extensive comments explaining not just what the code does, but why.

3. **Experiment**: Modify the examples. Break things. Change configurations. This is how you learn!

4. **Check the Logs**: HyperServe has excellent logging. Run with `HS_LOG_LEVEL=debug` for more details.

5. **Use the Docs**: Refer to the main [README.md](../README.md) and [docs/](../docs/) for deeper explanations.

## 🤝 Contributing

Have an idea for a new example? Please contribute! Good examples should:

- Demonstrate a specific HyperServe feature or use case
- Include clear comments explaining the code
- Have a README with setup instructions if needed
- Be as simple as possible while still being realistic
- Include curl commands or a simple client to test with

## 📋 Example Template

When creating a new example, consider this structure:

```
examples/your-example/
├── README.md          # What it does, how to run it, what to learn
├── main.go           # Well-commented implementation
├── static/           # (optional) Static assets
├── templates/        # (optional) HTML templates
└── client/           # (optional) Test client or curl commands
```

Happy learning! 🎉