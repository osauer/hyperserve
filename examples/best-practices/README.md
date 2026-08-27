# Composition reference

This example combines the major server features in one application: explicit
configuration precedence, health checks, middleware, bearer authentication,
templates, SSE, and MCP. It is a reference application, not the next step after
hello world; use the focused examples when learning one feature.

The useful patterns are about ownership:

- the application turns Ctrl+C or `SIGTERM` into context cancellation;
- HyperServe drains and closes the server resources it starts;
- deployment configuration is read only through `WithEnvironment`;
- authentication is composed from named pieces, and application handlers keep
  authorization;
- rate limiting is an application-owned gate placed in front of `/api`;
- long-lived SSE handlers stop when the request context is cancelled;
- the MCP endpoint is protected by the same identity middleware as the API.

Run from this directory, which contains the example templates:

```sh
go run .
```

Try a protected request:

```sh
curl -H "Authorization: Bearer secret-token-123" \
  http://localhost:8080/api/data
```

The example token verifier is deliberately local and tiny. For a federated
provider, use the [OpenID Connect example](../auth/) instead.

Middleware is a request wrapper. The limiter follows that same shape: create a
gate, then place the gate in front of the path it protects.

```go
apiGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 100,
	Burst:             200,
})
if err != nil {
	log.Fatal(err)
}
app.UsePrefix("/api", requireIdentity, apiGate)
```

Reusing `apiGate` elsewhere would deliberately share this quota pool. A second
call to `ratelimit.New` would create an independent pool.

The context at the top of `main` describes the lifetime of the complete
service. A larger host can supply that parent context instead. Request handlers
still use `r.Context()` for the lifetime of one HTTP request.
