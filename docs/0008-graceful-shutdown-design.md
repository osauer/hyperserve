# ADR-0008: Context-Based Graceful Shutdown

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Production servers need graceful shutdown to:
- Give in-flight requests time to complete before terminating
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
- Leave process-signal policy with the application
- Let an explicit `Shutdown` caller choose its deadline
- Coordinate shutdown of both main and health servers
- Clean up all resources (rate limiters, templates, etc.)

The ownership boundary is deliberately small:

| Owner | Responsibility |
| --- | --- |
| Application entry point | Chooses the parent context and which process signals cancel it |
| HyperServe | Observes `Run(ctx)` and drains the listeners, workers, and roots it started |
| Request handler | Uses `r.Context()` for one request's cancellation and values |

The context passed to `Run` is a shutdown trigger. HyperServe does not install
its values as the base context for HTTP requests; applications should add
request-scoped data through middleware.

```go
// Shutdown sequence:
// 1. Stop accepting new connections
// 2. Wait for in-flight requests (up to timeout)
// 3. Clean up workers, transports, and rooted filesystems
// 4. Return the original startup/serve error plus any shutdown error
```

## Consequences

### Positive
- **Orderly draining**: In-flight requests get a bounded opportunity to finish
- **Clean termination**: Resources properly released
- **Explicit deadlines**: An application coordinator can bound `Shutdown(ctx)`
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
ctx, stop := signal.NotifyContext(
    context.Background(),
    os.Interrupt,
    syscall.SIGTERM,
)
defer stop()

if err := srv.Run(ctx); err != nil {
    log.Error("server stopped", "error", err)
}
```

`context.Background()` is the root in a standalone executable. When another
component already owns the application lifetime, pass its context to `Run`, or
use it as the parent passed to `signal.NotifyContext`. The returned `stop`
function unregisters the signal behavior and releases its resources.

## Examples

For the usual application-owned lifetime, cancel the context passed to `Run`:

```go
// The application owns appCtx and cancels it when the complete process should
// stop. HyperServe drains requests and releases its resources before Run returns.
srv, _ := server.NewServer()
if err := srv.Run(appCtx); err != nil {
    log.Error("Server stopped", "error", err)
}
```

When another component coordinates shutdown directly, it can choose the
deadline:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
if err := srv.Shutdown(shutdownCtx); err != nil {
    log.Error("shutdown", "error", err)
}
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
```

The pod grace period covers both the `preStop` hook and application shutdown.
The short pause gives routing changes time to reach load balancers. Kubernetes
then sends `SIGTERM`, which cancels the application context; `Run` uses its
bounded internal cleanup budget to drain the server. The 60-second pod budget
leaves room for both phases before Kubernetes forces termination. See the
[Kubernetes pod termination flow](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-termination-flow)
for the ordering and grace-period rules.
