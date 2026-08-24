# ADR-0008: Context-Based Graceful Shutdown

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Production servers need graceful shutdown to:
- Complete in-flight requests before terminating
- Close database connections cleanly
- Flush buffered logs and metrics
- Notify load balancers before disappearing
- Prevent data loss or corruption

Challenges include:
- Managing multiple servers (main + health)
- Coordinating goroutine shutdown
- Handling long-running requests
- Dealing with misbehaving clients

## Decision

Implement context-based graceful shutdown:
- Use the context passed to `Run` for cancellation propagation
- Let an explicit `Shutdown` caller choose its deadline
- Coordinate shutdown of both main and health servers
- Clean up all resources (rate limiters, templates, etc.)

```go
// Shutdown sequence:
// 1. Stop accepting new connections
// 2. Wait for in-flight requests (up to timeout)
// 3. Clean up workers, transports, and rooted filesystems
// 4. Return the original startup/serve error plus any shutdown error
```

## Consequences

### Positive
- **No dropped requests**: In-flight requests complete normally
- **Clean termination**: Resources properly released
- **Predictable behavior**: The application controls the deadline
- **Kubernetes-friendly**: Works with termination grace periods
- **Testable**: Can verify shutdown behavior

### Negative
- **Complexity**: Shutdown logic touches many components
- **Timeout tradeoffs**: Too short drops requests, too long delays deployments
- **Goroutine management**: Must track all background tasks
- **Error handling**: Shutdown errors need special handling

### Mitigation
- `Run` uses a bounded internal cleanup budget after cancellation
- `Shutdown(ctx)` accepts an application-owned deadline
- Clear logging during shutdown phases
- Unit tests for shutdown scenarios

## Implementation Details

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

if err := srv.Run(ctx); err != nil {
    log.Error("server stopped", "error", err)
}
```

## Examples

```go
// The application owns appCtx and cancels it when its complete process
// lifecycle should stop. HyperServe drains requests and releases resources
// before Run returns.
srv, _ := server.NewServer()
if err := srv.Run(appCtx); err != nil {
    log.Error("Server stopped", "error", err)
}

shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
if err := srv.Shutdown(shutdownCtx); err != nil {
    log.Error("shutdown", "error", err)
}

// Kubernetes pod termination
// Set terminationGracePeriodSeconds > shutdown timeout
```

## Kubernetes Integration

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      terminationGracePeriodSeconds: 60
      containers:
      - name: app
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 5"]
        env:
        - name: HS_SHUTDOWN_TIMEOUT
          value: "45s"
```

This ensures:
1. Kubernetes removes pod from service endpoints
2. 5-second sleep allows load balancer updates
3. 45-second shutdown timeout for requests
4. 60-second pod termination grace period as safety net
