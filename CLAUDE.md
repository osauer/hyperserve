# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Run

```bash
# Build the main server (no main package in root, use examples)
cd examples/chaos && go build
cd examples/htmx-dynamic && go build
cd examples/htmx-stream && go build
cd examples/enterprise && go build
cd examples/mcp && go build

# Run examples
go run examples/chaos/main.go
go run examples/htmx-dynamic/main.go
go run examples/htmx-stream/main.go
go run examples/enterprise/main.go
go run examples/mcp/main.go

# Generate certificates for enterprise example
cd examples/enterprise && ./generate_certs.sh
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v -run TestServerCreation

# Run tests with race detection
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem
go test -bench=BenchmarkBaseline -benchmem -benchtime=10s

# Run benchmarks without noisy logs
go test -run=^$ -bench=. -benchmem 2>/dev/null
```

### Code Quality

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run Qodana analysis (requires Docker)
docker run --rm -it -v $(pwd):/data/project -p 8080:8080 jetbrains/qodana-go:latest
```

## Directory Structure

The repository follows Go project layout best practices with organized directories:

```
hyperserve/
├── README.md               # Project overview and quick start
├── LICENSE                 # MIT license
├── CONTRIBUTING.md         # Contribution guidelines
├── CLAUDE.md              # This file - AI assistance guidance
├── go.mod, go.sum         # Go module files
├── *.go, *_test.go        # Main library source and test files
├── docs/                  # Documentation files
│   ├── API_STABILITY.md   # API stability commitments
│   ├── CHANGELOG.md       # Version history and changes
│   ├── LESSONS_LEARNED.md # Development insights and patterns
│   ├── MIGRATION_GUIDE.md # Go 1.24 migration instructions
│   ├── PERFORMANCE.md     # Performance guide and benchmarks
│   ├── PUBLISH_CHECKLIST.md # Pre-publication checklist
│   └── RELEASE_NOTES.md   # Detailed release information
├── configs/               # Configuration files
│   ├── github_api.yaml    # GitHub API OpenAPI specification
│   ├── htmx-spec.json     # HTMX attributes configuration
│   └── qodana.yaml        # Qodana code analysis configuration
├── benchmarks/            # Benchmark results and analysis
└── examples/              # Example applications
    ├── auth/              # Authentication example
    ├── chaos/             # Chaos engineering example
    ├── enterprise/        # FIPS and security features
    ├── htmx-dynamic/      # Dynamic HTMX content
    ├── htmx-stream/       # Server-sent events with HTMX
    └── mcp/               # Model Context Protocol (MCP) example
```

This structure reduces root directory clutter and follows standard Go project organization patterns.

## Development Guidelines

- **Testing and Documentation**
  * Always test your changes thoroughly
  * When building new features or updating existing ones, update:
    - Examples
    - Documentation
    - Metadata files
  * Ensure comprehensive test coverage for new functionality

## Common Anti-Patterns and How to Avoid Them

When using hyperserve in your projects, avoid these common mistakes:

### 1. **DON'T Reimplement Built-in Features**

❌ **Bad: Custom graceful shutdown**
```go
// DON'T DO THIS - hyperserve already handles signals
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
go func() {
    <-sigChan
    fmt.Println("\nShutting down...")
    os.Exit(0)
}()
```

✅ **Good: Use built-in graceful shutdown**
```go
srv, _ := hyperserve.NewServer()
srv.Run() // Handles SIGINT/SIGTERM automatically
// Or use Stop() for programmatic shutdown:
// go srv.Run()
// srv.Stop()
```

### 2. **DON'T Create Custom Logging Middleware**

❌ **Bad: Custom request logger**
```go
// DON'T create your own logging middleware
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}
func StructuredLogger(next http.HandlerFunc) http.HandlerFunc { ... }
```

✅ **Good: Use built-in structured logging**
```go
// RequestLoggerMiddleware is automatically applied by default
// Configure log level via environment variable:
// HS_LOG_LEVEL=debug ./myapp
```

### 3. **DON'T Reimplement MCP Support**

❌ **Bad: Custom MCP handler**
```go
// DON'T create your own MCP implementation
type MCPHandler struct { ... }
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) { ... }
```

✅ **Good: Use built-in MCP support**
```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPFileToolRoot("/safe/path"),
)
// Register custom tools if needed:
// srv.mcpHandler.RegisterTool(&MyCustomTool{})
```

### 4. **DON'T Manually Implement SSE**

❌ **Bad: Manual SSE formatting**
```go
// DON'T manually format SSE messages
fmt.Fprintf(w, "event: %s\n", event)
fmt.Fprintf(w, "data: %s\n\n", jsonData)
flusher.Flush()
```

✅ **Good: Use SSE helpers**
```go
msg := hyperserve.NewSSEMessage("event-name", data)
fmt.Fprint(w, msg)
flusher.Flush()
```

### 5. **DON'T Skip Middleware Stacks**

❌ **Bad: Manual middleware application**
```go
// DON'T apply middleware one by one
srv.AddMiddleware("/api", RateLimitMiddleware)
srv.AddMiddleware("/api", AuthMiddleware)
srv.AddMiddleware("/api", HeadersMiddleware)
```

✅ **Good: Use pre-configured stacks**
```go
// Use built-in stacks for common patterns
srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))     // Auth + rate limiting for API routes
srv.AddMiddlewareStack("/", hyperserve.SecureWeb(srv.Options)) // Security headers for web routes
// Note: Route-specific middleware only applies to paths starting with the specified prefix
```

### 6. **DON'T Ignore Configuration System**

❌ **Bad: Hardcoded configuration**
```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithRateLimit(100, 200),
)
```

✅ **Good: Use configuration hierarchy**
```go
// Set via environment variables:
// HS_PORT=8080 HS_RATE_LIMIT=100 ./myapp
// Or use JSON config file:
// HS_CONFIG_PATH=config.json ./myapp
srv, _ := hyperserve.NewServer() // Uses env vars/config automatically
```

## API Design Principles

When designing new APIs or features for hyperserve, follow these principles:

### 1. **Sensible Defaults with Zero Configuration**
- APIs should work with minimal configuration
- Common use cases should require little to no boilerplate
- Example: `WithMCPSupport()` defaults to HTTP transport on `/mcp`

### 2. **Progressive Disclosure**
- Simple things should be simple
- Complex things should be possible
- Start with the simplest API that could work
- Add complexity only when needed
- Example: MCP transport can be as simple as `WithMCPSupport()` or configured with `WithMCPSupport(MCPOverStdio())`

### 3. **Consistency Across Features**
- Similar features should have similar APIs
- Transport mechanisms should feel idiomatic regardless of type
- Example: Both HTTP and stdio MCP servers use the same `NewServer()` and configuration pattern

### 4. **Parameters Over Separate Types**
- Prefer configuration through parameters rather than separate types
- Use functional options to customize behavior
- Example: Transport configuration via `WithMCPSupport(MCPOverHTTP("/api"))` rather than separate server types

### 5. **Export What Users Need**
- Export types that users might need to reference or create
- Protocol types (JSON-RPC, MCP) should be public for client code
- Internal implementation details remain unexported

### 6. **Maintain Low Barrier to Entry**
- Examples should be minimal and focused
- Avoid unnecessary abstractions
- Documentation should show the simplest path first

## Architecture

### Architecture Decision Records (ADRs)

Key architecture decisions are documented in [`docs/adr/`](docs/adr/):

- [ADR-0001: Minimal External Dependencies](docs/adr/0001-minimal-external-dependencies.md) - Only use `golang.org/x/time`, implement everything else
- [ADR-0002: Functional Options Pattern](docs/adr/0002-functional-options-pattern.md) - Use `WithX()` functions for configuration
- [ADR-0003: Layered Middleware Architecture](docs/adr/0003-layered-middleware-architecture.md) - Global, route-specific, and exclusion system
- [ADR-0004: Configuration Precedence](docs/adr/0004-configuration-precedence-hierarchy.md) - Env vars > JSON > defaults
- [ADR-0005: Separate Health Check Server](docs/adr/0005-separate-health-check-server.md) - Health endpoints on dedicated port
- [ADR-0006: Go 1.24 Minimum Version](docs/adr/0006-go-1-24-minimum-version.md) - Leverage modern Go features
- [ADR-0007: Template System Integration](docs/adr/0007-template-system-integration.md) - Optional HTML templating support
- [ADR-0008: Graceful Shutdown Design](docs/adr/0008-graceful-shutdown-design.md) - Context-based shutdown with timeout
- [ADR-0009: Single Package Architecture](docs/adr/0009-single-package-architecture.md) - Everything in one package
- [ADR-0010: Server-Sent Events Support](docs/adr/0010-server-sent-events-support.md) - SSE as first-class feature
- [ADR-0011: MCP Protocol Support](docs/adr/0011-mcp-protocol-support.md) - Model Context Protocol for AI assistant integration

### Core Components

**Server (`server.go`)**: Main HTTP server implementation with:

- Configuration via environment variables, JSON files, or defaults
- Graceful shutdown support
- TLS support
- Configurable timeouts (read/write/idle)
- Middleware chain management with route-specific stacks

**Middleware (`middleware.go`)**: Flexible middleware system featuring:

- Request ID generation
- Logging with structured logs (slog)
- Rate limiting using `golang.org/x/time/rate`
- Metrics collection (request count, latency)
- CORS support
- Authentication (token-based)
- Chaos mode for testing resilience

**Handlers (`handlers.go`)**: Built-in HTTP handlers:

- Health check endpoints (`/healthz`, `/readyz`, `/livez`)
- Template rendering with HTML templates
- Static file serving
- Server-Sent Events (SSE) support

**Options (`options.go`)**: Server configuration system:

- Environment variable parsing with `HS_` prefix
- JSON configuration file support
- Default values for all settings
- Configuration precedence: env vars > JSON file > defaults

### Middleware Architecture

The server uses a layered middleware approach where middleware can be:

1. Applied globally to all routes using `"*"` as the route pattern
2. Applied to specific route patterns (e.g., `"/api"`, `"/static"`)
3. Excluded from specific routes using `WithOutStack()`

Middleware execution order:
- Global middleware (`"*"`) runs first
- Route-specific middleware runs after global middleware
- Multiple middleware for the same route are executed in registration order
- Middleware is applied based on URL path prefix matching

### Key Design Principles

1. **Zero External Dependencies**: Only uses `golang.org/x/time` for rate limiting
2. **Simplicity**: Straightforward API with sensible defaults
3. **Flexibility**: Configurable via multiple methods
4. **Testability**: Designed with testing in mind (though tests need fixes)
5. **Production Ready**: Health checks, metrics, rate limiting built-in

## Graceful Shutdown

Hyperserve provides automatic graceful shutdown handling:

### Signal Handling
- **Signals**: Automatically handles `SIGINT` (Ctrl+C) and `SIGTERM`
- **Timeout**: Default 30-second shutdown timeout (configurable via `WithShutdownTimeout()`)
- **Process**: 
  1. Stops accepting new connections
  2. Waits for active requests to complete
  3. Closes all resources cleanly

### Usage Examples

```go
// Basic usage - Run() blocks and handles shutdown
srv, _ := hyperserve.NewServer()
srv.Run() // Blocks until shutdown signal received

// Advanced usage - Non-blocking with manual control
srv, _ := hyperserve.NewServer(
    hyperserve.WithShutdownTimeout(10 * time.Second),
)

// Start server in goroutine
go func() {
    if err := srv.Run(); err != nil {
        log.Fatal(err)
    }
}()

// Later, trigger graceful shutdown programmatically
srv.Stop() // Initiates graceful shutdown
```

### What Happens During Shutdown

1. **Health endpoints** return unhealthy status
2. **New connections** are rejected
3. **Active requests** continue processing until completion or timeout
4. **Middleware cleanup** functions are called
5. **Server resources** are released

### Best Practices

- Don't implement your own signal handling - hyperserve handles it
- Use `WithShutdownTimeout()` to adjust timeout for your needs
- Long-running handlers should respect context cancellation
- Health check server also shuts down gracefully

## Debug Logging

Hyperserve uses Go's structured logging (`slog`) for all logging:

### Enabling Debug Logs

```bash
# Set log level via environment variable
HS_LOG_LEVEL=debug ./myapp

# Available levels: debug, info, warn, error
HS_LOG_LEVEL=warn ./myapp
```

### What Gets Logged

**Debug level** includes:
- Middleware execution order
- Configuration loading details
- Request routing decisions
- Rate limit decisions
- MCP protocol messages

**Info level** (default) includes:
- Server startup/shutdown
- Request logs with duration
- Configuration summary
- Health check status changes

**Warn level** includes:
- Rate limit violations
- Authentication failures
- Resource constraints

**Error level** includes:
- Handler panics (recovered)
- TLS errors
- Fatal configuration issues

### Structured Log Format

```json
{
  "time": "2024-03-14T10:30:45.123Z",
  "level": "INFO",
  "msg": "request completed",
  "method": "GET",
  "path": "/api/users",
  "status": 200,
  "duration": "125.3ms",
  "ip": "192.168.1.100"
}
```

### Custom Logger Configuration

```go
// Use custom slog handler
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

srv, _ := hyperserve.NewServer(
    hyperserve.WithLogger(logger),
)
```

### Debugging Tips

1. **Enable debug logs** when troubleshooting:
   ```bash
   HS_LOG_LEVEL=debug ./myapp 2>&1 | jq .
   ```

2. **Filter logs** by component:
   ```bash
   HS_LOG_LEVEL=debug ./myapp 2>&1 | grep '"component":"middleware"'
   ```

3. **Performance debugging**:
   - Request logger shows duration for every request
   - Metrics middleware tracks request counts and latencies
   - Use `/metrics` endpoint for Prometheus-compatible metrics

4. **MCP debugging**:
   ```bash
   # See all MCP protocol messages
   HS_LOG_LEVEL=debug ./myapp 2>&1 | grep '"component":"mcp"'
   ```

### Configuration

The server reads configuration in this order:

1. Environment variables (prefixed with `HS_`)
2. JSON configuration file (if specified via `HS_CONFIG_PATH`)
3. Built-in defaults

Key configuration options:

- `HS_PORT`: Server port (default: 8080)
- `HS_RATE_LIMIT`: Requests per second (default: 100)
- `HS_BURST_LIMIT`: Burst capacity (default: 200)
- `HS_LOG_LEVEL`: Logging level
- `HS_CHAOS_MODE`: Enable chaos testing features
- `HS_TLS_CERT_FILE` / `HS_TLS_KEY_FILE`: TLS configuration

### Go 1.24 Features

The project leverages several Go 1.24 features:

1. **FIPS 140-3 Mode**: Use `WithFIPSMode()` for government compliance
2. **os.Root**: Secure file serving with automatic sandboxing
3. **Swiss Tables**: Rate limiter uses faster map implementation
4. **ECH Support**: Encrypted Client Hello for privacy
5. **Post-Quantum**: X25519MLKEM768 enabled by default (non-FIPS)

### Important Implementation Notes

1. **Middleware Signatures**: Always use `*ServerOptions` (pointer) not value types
2. **Server Fields**: Most server fields are unexported - use provided methods
3. **Benchmarks**: Place in main package to access unexported fields
4. **Rate Limiter**: Uses regular map with RWMutex, not sync.Map
5. **SSE**: Use `NewSSEMessage()` helper, avoid double fmt.Sprintf

### Testing Gotchas

1. **Parallel Tests**: Use unique directory names with timestamps/PID
2. **Cleanup**: Always defer cleanup of test directories
3. **Health Server**: Runs on separate port (:8081 by default)
4. **Template Tests**: Create actual template files, not just directories
5. **Middleware Tests**: Test with actual server instance, not in isolation

### Performance Considerations

1. **Allocations**: Baseline is 10 allocations per request - maintain this
2. **Middleware Overhead**: Aim for <30% total overhead
3. **Logging**: Most expensive middleware due to I/O
4. **Static Files**: Currently 31 allocations - optimization opportunity

## Quick Decision Guide for LLMs

When implementing a feature, ask yourself:

1. **Does hyperserve already have this?**
   - Logging? ✅ Use `RequestLoggerMiddleware` (applied by default)
   - Rate limiting? ✅ Use `WithRateLimit()` or `RateLimitMiddleware`
   - Auth? ✅ Use `WithAuthTokenValidator()` or `AuthMiddleware`
   - CORS? ✅ Use `HeadersMiddleware` with CORS headers
   - Health checks? ✅ Use `WithHealthServer()` (runs on :8081)
   - Graceful shutdown? ✅ Built into `Run()`
   - MCP support? ✅ Use `WithMCPSupport()`
   - SSE support? ✅ Use `NewSSEMessage()`
   - Static files? ✅ Use `HandleStatic()`
   - Templates? ✅ Use `HandleTemplate()` or `HandleTemplateFunc()`

2. **Am I following the pattern?**
   - Server creation uses `NewServer()` with functional options
   - Middleware uses the layered architecture (global → route-specific)
   - Configuration uses env vars (`HS_*`) → JSON → defaults
   - All handlers are `http.HandlerFunc` compatible

3. **Common mistakes to avoid:**
   - ❌ Don't create custom logging middleware
   - ❌ Don't implement your own graceful shutdown
   - ❌ Don't create custom MCP handlers
   - ❌ Don't manually format SSE messages
   - ❌ Don't hardcode configuration values
   - ❌ Don't create separate health check endpoints

### Common Patterns

```go
// Creating a secure API server
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
    hyperserve.WithTLS("cert.pem", "key.pem"),
    hyperserve.WithAuthTokenValidator(validateFunc),
)

// Adding middleware selectively (route-specific)
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))    // Only applies to /api/*
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options)) // Only applies to /api/*
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options)) // Applies to all routes

// Graceful shutdown
go srv.Run()
// ... later ...
srv.Stop()
```

## MCP (Model Context Protocol) Support

Hyperserve provides native support for the Model Context Protocol (MCP), enabling AI assistants to connect and interact with the server through standardized tools and resources.

### What is MCP?

The Model Context Protocol is an open standard that allows AI assistants to:
- **Execute Tools**: Perform operations like file reading, calculations, HTTP requests
- **Access Resources**: Read configuration, metrics, and system information
- **Secure Communication**: JSON-RPC 2.0 with capability negotiation
- **Sandboxed Operations**: Safe file access using Go 1.24's os.Root

### Quick Start

Enable MCP support in your server:

```go
srv, err := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithMCPSupport(),                    // Enable MCP
    hyperserve.WithMCPEndpoint("/mcp"),             // Custom endpoint (default: /mcp)
    hyperserve.WithMCPServerInfo("my-app", "1.0"),  // Server identification
    hyperserve.WithMCPFileToolRoot("./sandbox"),    // Secure file access root
)
```

### Built-in Tools

MCP includes these tools out of the box:

- **`calculator`**: Basic math operations (add, subtract, multiply, divide)
- **`read_file`**: Read file contents (sandboxed to configured root)
- **`list_directory`**: List directory contents (sandboxed)
- **`http_request`**: Make HTTP requests to external services

### Built-in Resources

Access server information through these resources:

- **`config://server/options`**: Server configuration (sanitized)
- **`metrics://server/stats`**: Performance metrics and statistics
- **`system://runtime/info`**: Go runtime and system information
- **`logs://server/recent`**: Recent log entries (if enabled)

### Configuration Options

```go
type ServerOptions struct {
    MCPEnabled             bool     // Enable/disable MCP support
    MCPEndpoint            string   // HTTP endpoint for MCP (default: "/mcp")
    MCPServerName          string   // Server name for identification
    MCPServerVersion       string   // Server version for identification
    MCPToolsEnabled        bool     // Enable/disable tools (default: true)
    MCPResourcesEnabled    bool     // Enable/disable resources (default: true)
    MCPFileToolRoot        string   // Root directory for file tools (optional)
}
```

### Environment Variables

Configure MCP via environment variables:

- `HS_MCP_ENABLED`: Enable MCP support (true/false)
- `HS_MCP_ENDPOINT`: MCP endpoint path
- `HS_MCP_SERVER_NAME`: Server identification name
- `HS_MCP_SERVER_VERSION`: Server version
- `HS_MCP_FILE_TOOL_ROOT`: Root directory for file operations

### Security Features

1. **Sandboxed File Access**: Uses Go 1.24's `os.Root` for secure file operations
2. **Configurable Root**: File tools restricted to specified directory
3. **Sanitized Configuration**: Sensitive data excluded from config resource
4. **Optional Authentication**: Integrates with existing auth middleware
5. **Rate Limiting**: Standard rate limiting applies to MCP endpoints

### Example Usage

#### Initialize MCP Connection

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "my-client", "version": "1.0.0"}
    },
    "id": 1
  }'
```

#### Use Calculator Tool

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "calculator",
      "arguments": {"operation": "multiply", "a": 15, "b": 4}
    },
    "id": 2
  }'
```

#### Read System Information

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0", 
    "method": "resources/read",
    "params": {"uri": "system://runtime/info"},
    "id": 3
  }'
```

### Custom Tools and Resources

Extend MCP with custom capabilities:

```go
import (
    "context"
    "encoding/json"
)

// MCP Tool interface
type MCPTool interface {
    Name() string
    Description() string
    Schema() map[string]interface{}
    Execute(params map[string]interface{}) (interface{}, error)
}

// MCP Resource interface
type MCPResource interface {
    URI() string
    Name() string
    Description() string
    MimeType() string
    Read(ctx context.Context) ([]byte, error)
}

// Custom tool implementation
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Custom tool description" }
func (t *MyTool) Schema() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "param1": map[string]interface{}{
                "type": "string",
                "description": "First parameter",
            },
        },
        "required": []string{"param1"},
    }
}
func (t *MyTool) Execute(params map[string]interface{}) (interface{}, error) {
    param1, _ := params["param1"].(string)
    // Tool implementation
    return map[string]interface{}{"result": param1}, nil
}

// Custom resource implementation
type MyResource struct{}

func (r *MyResource) URI() string { return "myapp://custom/data" }
func (r *MyResource) Name() string { return "Custom Data" }
func (r *MyResource) Description() string { return "Application-specific data" }
func (r *MyResource) MimeType() string { return "application/json" }
func (r *MyResource) Read(ctx context.Context) ([]byte, error) {
    data := map[string]interface{}{"data": "value"}
    return json.Marshal(data)
}

// Register custom tool and resource
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
)

// Must register after server creation, before Run()
if err := srv.RegisterMCPTool(&MyTool{}); err != nil {
    log.Fatal(err)
}
if err := srv.RegisterMCPResource(&MyResource{}); err != nil {
    log.Fatal(err)
}

srv.Run()
```

### Testing MCP Implementation

```bash
# Run MCP-specific tests
go test -v -run TestMCP

# Run integration tests
go test -v -run TestMCPIntegration

# Run all tests including MCP
go test ./... -v
```

### Example Application

See the complete MCP example:

```bash
# Run the MCP example server
cd examples/mcp
go run main.go

# Open browser to see documentation
open http://localhost:8080
```

### Performance Impact

- **Zero Overhead**: No performance impact when MCP is disabled
- **Lazy Initialization**: MCP components only created when enabled
- **Efficient Routing**: Direct handler registration, not middleware-based
- **Memory Management**: Proper cleanup and resource management

### Common Patterns

```go
// Secure AI assistant server
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPFileToolRoot("/safe/directory"),
    hyperserve.WithAuthTokenValidator(validateToken),
    hyperserve.WithRateLimit(50, 100), // Protect against abuse
)

// MCP with custom tools
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPResourcesDisabled(), // Only tools, no resources
)

// Register custom tools after server creation
if err := srv.RegisterMCPTool(&MyCustomTool{}); err != nil {
    log.Fatal(err)
}

// Minimal MCP server
srv, _ := hyperserve.NewServer(
    hyperserve.WithMCPSupport(),
    hyperserve.WithMCPEndpoint("/ai"),
)
```

### MCP STDIO Transport Security

The STDIO transport provides an alternative to HTTP for MCP communication, using standard input/output streams. This is particularly useful for:
- Local AI assistant integrations
- Command-line tools
- Embedded scenarios

#### Security Considerations

1. **Process Isolation**: STDIO transport inherits the security context of the parent process
   - Ensure the parent process runs with appropriate permissions
   - Consider using OS-level sandboxing (e.g., containers, VMs)

2. **File System Access**: When using file tools with STDIO transport
   - Always configure `WithMCPFileToolRoot()` to limit file access
   - The sandbox applies regardless of transport type
   - Never run with elevated privileges unless absolutely necessary

3. **Input Validation**: STDIO transport processes all input from stdin
   - Malformed JSON will be rejected with appropriate errors
   - Large payloads are limited by available memory
   - Consider implementing request size limits in production

4. **Deployment Recommendations**:
   ```go
   // Secure STDIO server with sandboxed file access
   srv, _ := hyperserve.NewServer(
       hyperserve.WithMCPSupport(hyperserve.MCPOverStdio()),
       hyperserve.WithMCPFileToolRoot("/restricted/path"),
       hyperserve.WithMCPToolsEnabled(true),
       hyperserve.WithMCPResourcesEnabled(false), // Limit exposure
   )
   ```

5. **Logging and Monitoring**: STDIO transport logs all operations
   - Errors are logged but not exposed to the client
   - Monitor logs for suspicious activity
   - Consider implementing rate limiting at the OS level

6. **Best Practices**:
   - Run STDIO servers as non-privileged users
   - Use process managers that can enforce resource limits
   - Implement timeouts for long-running operations
   - Regularly update to latest security patches