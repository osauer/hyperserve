# Middleware basics

This example shows the middleware split HyperServe is designed for: production
defaults are installed once, while application policy is attached globally or
to a route prefix.

`hyperserve.New` already provides request metrics, structured request logs,
and panic recovery. The example adds three things:

- a normal `net/http` wrapper that marks every response;
- browser security headers for every route;
- rate limiting only for `/api` and its descendants.

```go
app.Use(exampleHeader)
app.Use(hyperserve.HeadersMiddleware(app.Options()))

apiGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 5,
	Burst:             10,
})
if err != nil {
	log.Fatal(err)
}
app.UsePrefix("/api", apiGate)
```

That route prefix is segment-aware: it matches `/api` and `/api/data`, but not
`/api2`. Middleware is a request wrapper. Rate limiting follows the same model:
create a gate, then place the gate in front of the path it protects.

## Run it

From the repository root:

```bash
go run ./examples/middleware-basics
```

In another terminal:

```bash
# Global custom and security headers, outside the API rate-limit gate.
curl -i http://localhost:8080/

# Route-scoped rate limiting plus the global middleware.
curl -i http://localhost:8080/api/data

# Default recovery turns the deliberate panic into a generic 500.
curl -i http://localhost:8080/api/crash

# Default metrics counted the preceding requests.
curl http://localhost:8080/stats
```

## Write custom middleware

HyperServe keeps the standard handler-wrapper model:

```go
func exampleHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Example-Middleware", "active")
		next.ServeHTTP(w, r)
	})
}
```

Middleware in a stack runs in registration order: the first wrapper sees the
request first and the response last. Keep cross-cutting mechanics global and
put authorization, rate limits, or other policy on the narrowest relevant
prefix.
