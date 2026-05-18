# Deferred-init lifecycle

Demonstrates serving `/healthz` immediately while a long-running bootstrap
(`warmCaches`) runs in the background. Application routes return **503 Service
Unavailable** until both the deferred init and the `OnReady` hook succeed; then
the server flips to ready and traffic flows.

The pattern is for processes that need to register with an orchestrator (k8s
readiness, ALB target health, etc.) *before* they finish their slow startup
work — warming a cache, opening a long-lived database connection, fetching
config from a remote secret store.

## Run

```bash
go run ./examples/deferred-init &
```

In another shell:

```bash
curl -i http://localhost:8080/healthz      # 200 immediately
curl -i http://localhost:8080/api/users    # 503 until ready (~3s), then 200
```

The 503 is the contract — your orchestrator probes `/healthz` for liveness
and `/api/...` for traffic readiness. Don't probe traffic routes with
liveness intervals.
