# Configuration Example

This example demonstrates HyperServe's flexible configuration system and how different configuration sources interact through precedence rules.

## What This Example Shows

- Three configuration methods: programmatic, JSON file, and environment variables
- Configuration precedence hierarchy
- How to override specific settings
- Best practices for production configuration

## Configuration Methods

### 1. Programmatic Configuration (Lowest Priority)

```go
server, err := hyperserve.NewServer(
    hyperserve.WithAddr(":8080"),
    hyperserve.WithRateLimit(100, 200),
)
```

Use when:
- You have static configuration needs
- Configuration is computed at runtime
- Setting defaults in your application

### 2. JSON Configuration File (Medium Priority)

```json
{
  "addr": ":8080",
  "rate_limit": 100,
  "burst": 200
}
```

Use when:
- You want file-based configuration
- Different configs for different environments
- Configuration managed by ops teams

### 3. Environment Variables (Highest Priority)

```bash
export HS_PORT=8080
export HS_RATE_LIMIT=100
export HS_BURST_LIMIT=200
```

Use when:
- Running in containers (Docker, Kubernetes)
- Need to override settings without rebuilding
- Following 12-factor app principles

## Running the Example

```bash
go run main.go
```

The example runs through all configuration methods interactively, showing:
1. Programmatic configuration only
2. JSON file configuration
3. Environment variable configuration
4. Combined configuration demonstrating precedence

## Configuration Precedence

The precedence order is:
1. **Environment Variables** (highest)
2. **JSON Configuration File**
3. **Programmatic Options** (lowest)

This means:
- Environment variables always win
- JSON config overrides programmatic options
- Programmatic options provide defaults

## Available Configuration Options

| Option | Environment Variable | JSON Field | Programmatic Option |
|--------|---------------------|------------|-------------------|
| Server Address | `HS_PORT` or `HS_ADDR` | `addr` | `WithAddr()` |
| Rate Limit | `HS_RATE_LIMIT` | `rate_limit` | `WithRateLimit()` |
| Burst Limit | `HS_BURST_LIMIT` | `burst` | `WithRateLimit()` |
| Read Timeout | `HS_READ_TIMEOUT` | `read_timeout` | `WithTimeouts()` |
| Write Timeout | `HS_WRITE_TIMEOUT` | `write_timeout` | `WithTimeouts()` |
| TLS Certificate | `HS_TLS_CERT_FILE` | `tls_cert_file` | `WithTLS()` |
| TLS Key | `HS_TLS_KEY_FILE` | `tls_key_file` | `WithTLS()` |
| Static Directory | `HS_STATIC_DIR` | `static_dir` | `WithStaticDir()` |
| Template Directory | `HS_TEMPLATE_DIR` | `template_dir` | `WithTemplateDir()` |

## JSON Configuration Example

Create a file `config.json`:

```json
{
  "addr": ":8080",
  "rate_limit": 100,
  "burst": 200,
  "read_timeout": "30s",
  "write_timeout": "30s",
  "idle_timeout": "120s",
  "static_dir": "./static",
  "template_dir": "./templates"
}
```

Then either:
```bash
# Set via environment variable
export HS_CONFIG_PATH=config.json
go run myapp.go

# Or load programmatically
server, err := hyperserve.NewServer(
    hyperserve.WithConfigFile("config.json"),
)
```

## Environment Variables Example

```bash
# Basic configuration
export HS_PORT=8080

# Rate limiting
export HS_RATE_LIMIT=50
export HS_BURST_LIMIT=100

# Timeouts (use Go duration format)
export HS_READ_TIMEOUT=30s
export HS_WRITE_TIMEOUT=30s

# TLS
export HS_TLS_CERT_FILE=/path/to/cert.pem
export HS_TLS_KEY_FILE=/path/to/key.pem

# Directories
export HS_STATIC_DIR=/var/www/static
export HS_TEMPLATE_DIR=/var/www/templates
```

## Production Best Practices

### 1. Use Environment Variables for Secrets
```bash
# Never put secrets in JSON files
export HS_AUTH_TOKEN_SECRET=your-secret-key
export HS_TLS_KEY_FILE=/secure/path/key.pem
```

### 2. Layer Your Configuration
```go
// Base configuration in code
server, err := hyperserve.NewServer(
    hyperserve.WithRateLimit(100, 200),  // Defaults
)

// Override with environment-specific JSON
// config/production.json, config/staging.json, etc.

// Final overrides with environment variables
// HS_PORT, HS_LOG_LEVEL, etc.
```

### 3. Validate Configuration
```go
if server.Options.RateLimit < 10 {
    log.Fatal("Rate limit too low for production")
}
```

### 4. Document Your Configuration
Create a `.env.example` file:
```bash
# Server Configuration
HS_PORT=8080

# Rate Limiting
HS_RATE_LIMIT=100
HS_BURST_LIMIT=200

# Timeouts
HS_READ_TIMEOUT=30s
HS_WRITE_TIMEOUT=30s
```

## Docker Example

```dockerfile
FROM golang:1.24 AS builder
WORKDIR /app
COPY . .
RUN go build -o server .

FROM alpine:latest
COPY --from=builder /app/server /server

# Default configuration
ENV HS_PORT=8080

EXPOSE 8080
CMD ["/server"]
```

Then override at runtime:
```bash
docker run -e HS_PORT=9090 -e HS_RATE_LIMIT=200 myapp
```

## Kubernetes Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hyperserve-config
data:
  server-config.json: |
    {
      "rate_limit": 100,
      "burst": 200
    }
---
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: HS_CONFIG_PATH
          value: /config/server-config.json
        - name: HS_PORT
          value: "8080"
        volumeMounts:
        - name: config
          mountPath: /config
      volumes:
      - name: config
        configMap:
          name: hyperserve-config
```

## Debugging Configuration

Check the loaded configuration:
```go
fmt.Printf("Loaded config: %+v\n", server.Options)
```

Or create a debug endpoint:
```go
server.HandleFunc("/debug/config", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(server.Options)
})
```

## What's Next?

You've now learned the fundamentals of HyperServe! Consider exploring:
- The [enterprise example](../enterprise/) for advanced security features
- The [HTMX examples](../htmx-dynamic/) for modern web apps
- The main [documentation](../../docs/) for deeper topics