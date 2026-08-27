# HyperServe or Gin: choose by boundary, not by line count

Gin and HyperServe can both host Go HTTP handlers, but they optimize for
different applications. Gin is an established web framework with its own
request context and a broad middleware ecosystem. HyperServe stays close to
`net/http` and adds an embeddable MCP endpoint, typed helpers, explicit
lifecycle, and small concern-specific packages.

The useful question is not which framework wins in the abstract. It is which
boundary your application needs to own.

## A small HyperServe application

> **v2.1.0 compatibility notice:** this release makes a dated, controlled
> breaking change inside the existing `/v2` module. Read the
> [v2.1 migration guide](../MIGRATING_V2_1.md) before upgrading an existing v2
> application. To roll back, pin
> `github.com/osauer/hyperserve/v2@v2.0.3`. Future breaking changes require a
> new major version and matching major module path.

```bash
go get github.com/osauer/hyperserve/v2@v2.1.0
```

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/osauer/hyperserve/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app, err := hyperserve.New()
	if err != nil {
		log.Fatal(err)
	}

	app.GET("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

The handler is the standard-library shape. Request cancellation and values
remain on `r.Context()`. The application owns signals and gives `Run` the
lifetime it should follow.

## Where HyperServe adds value

### MCP in the same process

HyperServe can expose HTTP routes and MCP tools through one application
lifecycle:

```go
import (
	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
)

app, err := hyperserve.New(
	hyperserve.WithMCPSupport("catalog", "1.0.0"),
)
if err != nil {
	return err
}

if err := app.RegisterMCPTool(mcp.NewTypedTool(
	"search",
	"Search the product catalog.",
	searchCatalog,
)); err != nil {
	return err
}
```

The `mcp` package owns the protocol types and handler. The root package owns
integration with the configured application. Optional built-in resources live
in `mcp/builtin` and require an explicit import, keeping that capability out of
applications that do not ask for it.

Gin can front a separate MCP implementation, but the integration, lifecycle,
and authorization boundary belong to the application. That may be the right
choice when an existing Gin service already has those conventions.

### Typed HTTP helpers without a custom context

For a validation-and-echo endpoint:

```go
type CreateUser struct {
	Name  string `json:"name"  validate:"required,min=2,max=64"`
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role"  validate:"required,oneof=admin user guest"`
}

app.POST("/users", hyperserve.JSONEcho[CreateUser]())
```

`JSONEcho` binds JSON, validates the value, and writes it back. When the output
differs from the input, `JSONHandler` wraps a typed business function:

```go
func createUser(ctx context.Context, input CreateUser) (User, error) {
	return users.Create(ctx, input)
}

app.POST("/users", hyperserve.JSONHandler(createUser))
```

The lower-level `BindJSON`, `BindQuery`, `BindForm`, and `Validate` functions
remain available when a handler needs custom headers, streaming, or a different
response envelope.

Gin's binding and validation ecosystem is broader. Choose it when you need its
formats, validators, or middleware integrations rather than rebuilding them.

### Explicit request gates

Middleware in HyperServe is a standard request wrapper. Concern-specific
policy stays outside the root package. Rate limiting, for example, is: create a
gate, then place the gate in front of a path.

```go
apiGate, err := ratelimit.New(ratelimit.Config{
	RequestsPerSecond: 20,
	Burst:             40,
})
if err != nil {
	return err
}

app.UsePrefix("/api", apiGate)
```

The application decides which paths share a quota and which client identity is
trusted. The root package does not start a limiter cleanup goroutine or keep
limiter state merely because an application was constructed.

## Where Gin is the clearer fit

Choose Gin when its established conventions are already part of the service:

- handlers and middleware are written around `*gin.Context`;
- the application depends on a Gin-specific binding, rendering, or validation
  feature;
- a required observability, authentication, or transport integration already
  exists in the Gin ecosystem;
- the team values ecosystem familiarity more than a standard-library-shaped
  boundary.

Those are architectural advantages, not shortcomings to work around.

## Where plain `net/http` is enough

Modern `net/http.ServeMux` supports method and path patterns, and `log/slog`
provides structured logging. A service that needs only a few routes, its own
middleware, and explicit `http.Server` shutdown may not need either framework.

Starting with the standard library is especially reasonable when:

- MCP is not part of the service;
- the application already owns routing, validation, and lifecycle helpers;
- adding a framework would only rename standard-library concepts.

HyperServe is most useful when its MCP integration or coordinated HTTP
features remove code the application would otherwise maintain.

## Performance claims require a workload

HyperServe builds on `net/http.ServeMux`; Gin uses its own routing and handler
abstractions. That design difference alone is not a benchmark result. Route
shape, middleware depth, payload work, connection reuse, and concurrency can
change which cost matters.

Use HyperServe's [benchmark guide](../PERFORMANCE.md) to compare the exact route
and middleware stack you will deploy. Do not infer production latency from a
hello-world line count or an unrelated framework benchmark.

## Decision summary

| Need | Start with |
|---|---|
| Embeddable MCP plus ordinary `net/http` handlers | HyperServe |
| Existing Gin codebase or Gin-specific ecosystem integration | Gin |
| A small HTTP service with application-owned helpers | `net/http` |
| Proven hot-path routing requirement | Benchmark the real workload |

For HyperServe, continue with the [README](../../README.md),
[MCP guide](../MCP_GUIDE.md), and runnable [`examples`](../../examples/).
