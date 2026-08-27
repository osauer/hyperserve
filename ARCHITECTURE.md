# HyperServe Architecture

HyperServe is an integrated HTTP boundary built on `net/http`. The root
`hyperserve` package coordinates routing, middleware, configuration,
readiness, and lifecycle. Concern-specific public packages remain independently
usable where coupling them to `Server` would blur ownership.

The package layout is established by
[ADR-0014](./docs/0014-root-package-and-concern-subpackages.md).

## Design principles

### Standard-library shapes

- Routes use `http.ServeMux` patterns and handlers remain `http.Handler`
  values.
- Middleware is `func(http.Handler) http.Handler`; third-party wrappers need
  no adapter.
- TLS uses `crypto/tls`, cancellation uses `context.Context`, and static
  files are confined with `os.Root`.
- WebSocket and JSON-RPC are implemented in this repository.

### Caller-owned authority

- `hyperserve.New` is deterministic. Files and environment affect
  construction only through explicit `WithConfigFile` and `WithEnvironment`
  options.
- The application owns signals and the root context. `Run(ctx)` follows that
  context; handlers use `r.Context()`.
- Authentication establishes identity. Sessions, roles, resource
  authorization, and identity-provider setup remain application policy.
- Browser headers, rate limiting, MCP, filesystem roots, and health listeners
  are explicit capabilities.

### Small shipped graph

The runtime module has one external dependency:
`golang.org/x/time/rate`, used only by the `ratelimit` package. Developer
tools and the official MCP conformance dependency live in `tools/go.mod` and
do not enter an application's runtime graph.

This reduces dependency assembly for callers, but it also means HyperServe owns
more protocol and security code. Repository tests and conformance checks are
part of that trade-off.

## Public package graph

The canonical public packages are:

- `github.com/osauer/hyperserve/v2` — HTTP server, middleware, lifecycle,
  binding, templates, static files, SSE, and MCP wiring.
- `github.com/osauer/hyperserve/v2/auth` — provider-neutral authentication
  and request principals.
- `github.com/osauer/hyperserve/v2/jsonrpc` — standalone JSON-RPC 2.0.
- `github.com/osauer/hyperserve/v2/mcp` — MCP handler, transports, discovery,
  tools, resources, and namespaces.
- `github.com/osauer/hyperserve/v2/mcp/builtin` — opt-in built-in tools and
  resources.
- `github.com/osauer/hyperserve/v2/ratelimit` — bounded HTTP rate-limit
  middleware.
- `github.com/osauer/hyperserve/v2/websocket` — RFC 6455 server and client.

The load-bearing dependency direction is:

```text
hyperserve root ──> mcp ──> jsonrpc
       │
       └──────────> websocket

mcp/builtin ──> hyperserve root + mcp

ratelimit ──> standard library + golang.org/x/time/rate
auth      ──> standard library
```

The root package does not import `mcp/builtin` or `ratelimit`. Builtins
avoid an import cycle by registering hooks from `mcp/builtin.init`; an
application enabling builtins must explicitly import that package, commonly
with a blank import.

## HTTP server boundary

`Server` owns:

- main and optional health listeners;
- the `ServeMux`, route registry, and compiled middleware plan;
- readiness and deferred-initialization state;
- server-local logging and request/WebSocket metrics;
- template and static-root handles opened by explicit configuration;
- the optional MCP handler it constructs.

`Server` does not own process signals, application goroutines, application
authorization, sessions, reconnection policy, or rate-limit quota state.

Construction starts with `DefaultOptions()`, applies options left to right,
normalizes the final snapshot, and then creates only the resources required by
that snapshot. Constructing a server starts no limiter cleanup goroutine.

## Middleware and rate limiting

The middleware registry stores global wrappers and segment-aware path-prefix
wrappers. The first registered wrapper is the outermost one. `UsePrefix("/api",
...)` matches `/api` and `/api/users`, not `/apiv2`.

Rate limiting is a separate gate:

1. `ratelimit.New(Config)` validates a policy and returns middleware.
2. The application places that middleware with `Use` or `UsePrefix`.
3. Reusing the same returned value shares one quota namespace; separate
   `New` calls isolate quotas.

The gate keys clients by normalized transport peer unless the application
explicitly installs `TrustedProxyClientKey` with validated proxy prefixes.
State is finite, stale entries are pruned opportunistically, and no background
goroutine or `Close` method exists.

## MCP boundary

MCP is optional. The root package constructs `mcp.Handler` only after MCP is
enabled. The protocol package remains independently testable and depends on
`jsonrpc`, not on the root server.

Current Streamable HTTP uses finite JSON responses and request-scoped SSE for
`subscriptions/listen`; stdio uses the same handler. The proprietary routed
SSE transport is deprecated, disabled by default, and exists only for
HyperServe-specific compatibility.

Discovery filtering controls visibility, not authorization. Authentication and
authorization middleware must protect the MCP endpoint itself. Built-in tools
and resources are demonstrations and operations aids, not an authorization
policy.

## Security boundaries

- TLS defaults to 1.2 or newer. `WithFIPSMode` narrows TLS primitives; it does
  not by itself make a deployment FIPS 140-3 compliant.
- Static-file serving fails closed when its configured `os.Root` cannot be
  opened.
- The default limiter identity ignores forwarding headers. Trusted proxy
  parsing is an explicit application decision.
- WebSocket origin checks and MCP browser-origin validation are protocol
  defenses, not user authentication.
- Long-lived handlers and protocol operations must observe their supplied
  contexts and deadlines.

See [Production](./docs/PRODUCTION.md) and
[Security](./SECURITY.md) for deployment guidance.

## Evidence and change control

Repository benchmarks compare revisions under a recorded workload; they are not
universal throughput claims. See [Performance](./docs/PERFORMANCE.md).

Package-boundary or lifecycle changes require an ADR update, tests at the
changed boundary, and a consumer witness where public behavior is involved.
Run `make check`, `make test-race`, and `make fuzz-smoke` before proposing
such a change.
