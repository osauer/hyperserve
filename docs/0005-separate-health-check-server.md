# ADR-0005: Separate Health Check Server

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Health checks are critical for container orchestrators (Kubernetes, Docker Swarm) and load balancers. They need to:
- Always respond quickly, even under load
- Not be affected by rate limiting
- Not require authentication
- Provide different check types (liveness, readiness)

Traditional approaches:
1. **Same server**: Health checks on main server can fail under load
2. **Special middleware bypass**: Complex logic to skip middleware
3. **External monitoring**: Requires additional infrastructure

## Decision

Run health endpoints on a separate HTTP server with its own address:
- Main server: user traffic on the configured address (default `:8080`)
- Health server: operational checks on the configured health address (default `:9080`)
- Endpoints: `/healthz/`, `/livez/`, `/readyz/`

The health server is minimal with no middleware, ensuring reliable responses.

## Consequences

### Positive
- **Reliability**: Health checks work even when main server is overloaded
- **Simplicity**: No complex middleware bypass logic
- **Isolation**: Health checks use a separate listener and mux from main traffic
- **Kubernetes-native**: Separate ports for liveness/readiness
- **Clean separation**: Health checks are operationally distinct

### Negative
- **Extra port**: Requires opening additional port in firewalls
- **More complex**: Two servers to manage instead of one
- **Port conflicts**: Health port might already be in use
- **Discovery**: Users might not know about health port

### Mitigation
- Deterministic default address
- Explicit override through `WithHealthAddr`
- Health server disabled until `WithHealthServer` is passed

## Implementation Details

- Health server starts when explicitly enabled via WithHealthServer()
- Shares the same graceful shutdown mechanism
- Minimal HTTP server with no middleware
- Returns appropriate HTTP status codes:
  - `/healthz/`: Generic health check (200 if healthy)
  - `/livez/`: Liveness probe (200 if process is alive)
  - `/readyz/`: Readiness probe (200 if ready for traffic)

## Examples

```yaml
# Kubernetes deployment
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        ports:
        - containerPort: 8080  # Main traffic
        - containerPort: 9080  # Health checks
        livenessProbe:
          httpGet:
            path: /livez/
            port: 9080
        readinessProbe:
          httpGet:
            path: /readyz/
            port: 9080
```

```go
srv, err := server.NewServer(
	server.WithAddr("127.0.0.1:8080"),
	server.WithHealthServer(),
	server.WithHealthAddr("127.0.0.1:9080"),
)
```

Use `WithDeferredInit` when readiness depends on a database, cache, or other
startup dependency. HyperServe keeps readiness false until that callback and
the registered `WithOnReady` hooks succeed.
