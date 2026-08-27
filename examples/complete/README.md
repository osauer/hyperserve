# Complete application example

This is the broad tour of HyperServe: templates and static files, health
checks, security headers, bearer authentication, rate limiting, SSE, file
uploads, recovery, and MCP in one process.

It is intentionally larger than the README quick start. Read
[`main.go`](./main.go) when you want to see how the features compose; start
with [hello-world](../hello-world/) when learning the basic server shape.

## Run

From this directory:

```sh
go run .
```

Public endpoints include `/`, `/static/`, and `/status`. Routes under `/api`
and the `/mcp` endpoint require one of the example bearer tokens:

```sh
curl -H "Authorization: Bearer demo-token-123" \
  http://localhost:8080/api/user
```

`demo-token-123` maps to `alice`; `demo-token-456` maps to `bob`.

The authentication flow is split into named steps:

```go
verifier := auth.TokenVerifierFunc(verifyToken)
apiIdentity := auth.Bearer(verifier)
requireIdentity := auth.Require(apiIdentity)

apiGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 100,
	Burst:             200,
})
if err != nil {
	log.Fatal(err)
}

app.UsePrefix("/api", requireIdentity, apiGate)
app.UsePrefix("/mcp", requireIdentity)
```

Authentication answers “who is this?” The `/api/user` handler reads the
resulting principal and remains responsible for the application decision about
what that subject may see or change. Middleware is a request wrapper; the
limiter is created as a gate and placed in front of `/api`. Its quota is owned
by this application rather than by the server lifecycle.
