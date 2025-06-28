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

Run health check endpoints on a separate HTTP server on a different port:
- Main server: User traffic on configured port (default 8080)
- Health server: Health checks on port+1 (default 8081)
- Endpoints: `/healthz`, `/livez`, `/readyz`

The health server is minimal with no middleware, ensuring reliable responses.

## Consequences

### Positive
- **Reliability**: Health checks work even when main server is overloaded
- **Simplicity**: No complex middleware bypass logic
- **Performance**: Zero impact on main server performance
- **Kubernetes-native**: Separate ports for liveness/readiness
- **Clean separation**: Health checks are operationally distinct

### Negative
- **Extra port**: Requires opening additional port in firewalls
- **More complex**: Two servers to manage instead of one
- **Port conflicts**: Health port might already be in use
- **Discovery**: Users might not know about health port

### Mitigation
- Automatic port selection (main port + 1)
- Clear logging of health server startup
- Documentation of health endpoints
- Allow disabling health server if not needed

## Implementation Details

- Health server starts automatically with main server
- Shares the same graceful shutdown mechanism
- Minimal HTTP server with no middleware
- Returns appropriate HTTP status codes:
  - `/healthz`: Generic health check (200 if healthy)
  - `/livez`: Liveness probe (200 if process is alive)
  - `/readyz`: Readiness probe (200 if ready for traffic)

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
        - containerPort: 8081  # Health checks
        livenessProbe:
          httpGet:
            path: /livez
            port: 8081
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
```

```go
// Server automatically starts health server
srv, _ := hyperserve.NewServer(
    hyperserve.WithPort(8080),  // Main server on 8080
    // Health server automatically on 8081
)

// Optional: Custom health checks
srv.SetReadinessCheck(func() bool {
    return database.IsConnected()
})
```