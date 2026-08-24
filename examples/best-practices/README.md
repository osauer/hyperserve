# HyperServe composition example

This example combines the major server features in one application: explicit
configuration precedence, health checks, middleware, bearer authentication,
templates, static files, SSE, and MCP.

The useful patterns are about ownership:

- the application turns Ctrl+C into context cancellation;
- HyperServe drains and closes the server resources it starts;
- deployment configuration is read only through `WithEnvironment`;
- authentication is composed from named pieces, and application handlers keep
  authorization;
- long-lived SSE handlers stop when the request context is cancelled;
- the MCP endpoint is protected by the same identity middleware as the API.

Run from this directory, which contains the example templates and static root:

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
