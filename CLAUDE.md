# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Run

```bash
# Build the main server (no main package in root, use examples)
cd examples/chaos && go build
cd examples/htmx-dynamic && go build
cd examples/htmx-stream && go build

# Run examples
go run examples/chaos/main.go
go run examples/htmx-dynamic/main.go
go run examples/htmx-stream/main.go
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

## Architecture

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

### Current Issues

1. Test files have compilation errors due to undefined variables/functions
2. Some examples have import/type mismatches
3. Auth example is incomplete (marked as TODO)

When fixing tests, ensure that:

- All undefined variables are properly declared
- Handler functions match expected signatures
- Middleware functions receive correct parameter types