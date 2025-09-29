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
- Use Go's `context.Context` for cancellation propagation
- Configurable shutdown timeout (default 30 seconds)
- Coordinate shutdown of both main and health servers
- Clean up all resources (rate limiters, templates, etc.)

```go
// Shutdown sequence:
// 1. Stop accepting new connections
// 2. Wait for in-flight requests (up to timeout)
// 3. Force close remaining connections
// 4. Clean up resources
// 5. Return
```

## Consequences

### Positive
- **No dropped requests**: In-flight requests complete normally
- **Clean termination**: Resources properly released
- **Predictable behavior**: Timeout ensures eventual termination
- **Kubernetes-friendly**: Works with termination grace periods
- **Testable**: Can verify shutdown behavior

### Negative
- **Complexity**: Shutdown logic touches many components
- **Timeout tradeoffs**: Too short drops requests, too long delays deployments
- **Goroutine management**: Must track all background tasks
- **Error handling**: Shutdown errors need special handling

### Mitigation
- Default timeout of 30 seconds works for most cases
- Allow timeout customization via options
- Clear logging during shutdown phases
- Unit tests for shutdown scenarios

## Implementation Details

```go
func (s *Server) Stop() error {
    // Create shutdown context with timeout
    ctx, cancel := context.WithTimeout(
        context.Background(), 
        s.opts.ShutdownTimeout,
    )
    defer cancel()
    
    // Shutdown main server
    if err := s.httpServer.Shutdown(ctx); err != nil {
        log.Error("Main server shutdown error", "error", err)
    }
    
    // Shutdown health server
    if s.healthServer != nil {
        s.healthServer.Shutdown(ctx)
    }
    
    // Stop rate limiter cleanup
    if s.rateLimiterCancel != nil {
        s.rateLimiterCancel()
    }
    
    return nil
}
```

## Examples

```go
// Basic usage
srv, _ := server.NewServer()
go srv.Run()

// Graceful shutdown on SIGTERM
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
<-sigChan

log.Info("Shutting down gracefully...")
if err := srv.Stop(); err != nil {
    log.Error("Shutdown error", "error", err)
}

// Custom shutdown timeout
srv, _ := server.NewServer(
    server.WithShutdownTimeout(60 * time.Second),
)

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