# Deferred initialization

This example keeps liveness separate from traffic readiness while a slow
startup task runs. The process is alive immediately, but application traffic
does not become ready until `WithDeferredInit` and every `WithOnReady` hook
succeeds.

## Run

```bash
go run ./examples/deferred-init
```

From another terminal while it starts:

```bash
curl -i http://localhost:9080/healthz/     # 200 immediately
curl -i http://localhost:9080/readyz/      # 503, then 200 after about 3 seconds
curl -i http://localhost:8080/api/users    # 503, then the JSON response
```

The separate `:9080` listener is the stable probe surface. Liveness answers
whether the process should be restarted; readiness answers whether it should
receive traffic. The `/api/users` route is registered by an `OnReady` hook to
show that route setup can depend on successful initialization.
