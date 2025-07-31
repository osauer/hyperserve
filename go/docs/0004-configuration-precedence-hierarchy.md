# ADR-0004: Configuration Precedence Hierarchy

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Server configuration needs to come from multiple sources:
- **Development**: Developers want easy local configuration
- **Docker/K8s**: Containers need environment variable configuration
- **Production**: Operations teams prefer configuration files
- **Defaults**: Should work out-of-the-box with zero configuration

The challenge is determining which configuration source takes precedence when multiple sources provide the same setting.

## Decision

Implement a three-level configuration hierarchy with clear precedence:
1. **Environment variables** (highest priority) - prefixed with `HS_`
2. **JSON configuration file** - specified via `HS_CONFIG_PATH`
3. **Built-in defaults** (lowest priority)

Environment variables use uppercase with underscores:
- `HS_PORT=8080`
- `HS_RATE_LIMIT=100`
- `HS_TLS_CERT_FILE=/path/to/cert`

## Consequences

### Positive
- **12-factor compliance**: Environment variables for configuration
- **Container-friendly**: Easy to configure in Docker/Kubernetes
- **Development ease**: JSON files for complex local setups
- **Zero-config start**: Works immediately with defaults
- **Clear precedence**: No ambiguity about which value wins
- **Secure**: Sensitive values can use environment variables

### Negative
- **No runtime changes**: Must restart to apply new configuration
- **Limited formats**: Only JSON for file-based config
- **Prefix requirement**: All env vars need `HS_` prefix
- **Type conversions**: String env vars must be parsed

### Mitigation
- Clear documentation of all configuration options
- Validation with helpful error messages
- Log configuration sources on startup
- Examples for common scenarios

## Implementation Details

Configuration loading order:
1. Initialize with built-in defaults
2. If `HS_CONFIG_PATH` is set, load and merge JSON file
3. Override with any `HS_*` environment variables
4. Validate final configuration

## Examples

```bash
# Development: Use defaults
./server

# Development: Custom config file
HS_CONFIG_PATH=dev.json ./server

# Production: Environment variables
HS_PORT=80 HS_RATE_LIMIT=1000 ./server

# Docker: Mix of env vars and config
docker run -e HS_PORT=8080 -e HS_CONFIG_PATH=/config/prod.json myapp
```

Config file example:
```json
{
  "port": 8080,
  "rateLimit": 1000,
  "burstLimit": 2000,
  "templateDir": "./templates",
  "tls": {
    "certFile": "/certs/server.crt",
    "keyFile": "/certs/server.key"
  }
}
```