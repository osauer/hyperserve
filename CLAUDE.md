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

# Run examples
go run examples/chaos/main.go
go run examples/htmx-dynamic/main.go
go run examples/htmx-stream/main.go
go run examples/enterprise/main.go

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
    └── htmx-stream/       # Server-sent events with HTMX
```

This structure reduces root directory clutter and follows standard Go project organization patterns.

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

1. Applied globally to all routes
2. Applied to specific route patterns
3. Excluded from specific routes

Middleware execution order follows the registration sequence, with route-specific middleware executing after global
middleware.

### Key Design Principles

1. **Zero External Dependencies**: Only uses `golang.org/x/time` for rate limiting
2. **Simplicity**: Straightforward API with sensible defaults
3. **Flexibility**: Configurable via multiple methods
4. **Testability**: Designed with testing in mind (though tests need fixes)
5. **Production Ready**: Health checks, metrics, rate limiting built-in

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

### Common Patterns

```go
// Creating a secure API server
srv, _ := hyperserve.NewServer(
    hyperserve.WithFIPSMode(),
    hyperserve.WithTLS("cert.pem", "key.pem"),
    hyperserve.WithAuthTokenValidator(validateFunc),
)

// Adding middleware selectively
srv.AddMiddleware("/api", hyperserve.RateLimitMiddleware(srv))
srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))

// Graceful shutdown
go srv.Run()
// ... later ...
srv.Stop()
```