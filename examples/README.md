# HyperServe examples

Start with [the document catalogue](./catalogue/) to see an integrated
application: one collection, HTML, JSON, MCP, authenticated SSE, health checks,
and bounded uploads. It uses the standard library and HyperServe only.

The focused examples below isolate individual lessons.

## Start here

These programs run from the repository root with
`go run ./examples/<name>`:

| Order | Example | What it teaches |
|---:|---|---|
| 1 | [hello-world](./hello-world/) | One ordinary `net/http` handler and application-owned shutdown. |
| 2 | [middleware-basics](./middleware-basics/) | Global middleware, prefix policy, ordering, and HyperServe's defaults. |
| 3 | [binding](./binding/) | Focused typed JSON binding, validation, and manual escape hatches. |
| 4 | [configuration](./configuration/) | Deterministic defaults and left-to-right configuration precedence. |
| 5 | [deferred-init](./deferred-init/) | Health, readiness, and dependency initialization under one lifecycle. |

The [JSON API](./json-api/) is a larger CRUD application to read after the
focused binding example.

## Pages and browser applications

These examples use files relative to their own directories. Their READMEs show
the exact `cd` and `go run .` command.

| Example | What it shows |
|---|---|
| [static-files](./static-files/) | An `os.Root`-confined asset directory beside an API route. |
| [htmx-dynamic](./htmx-dynamic/) | Server-rendered templates with HTMX partial updates. |
| [htmx-stream](./htmx-stream/) | HTMX updates over Server-Sent Events. |
| [enterprise](./enterprise/) | TLS and security-header configuration; no enterprise service dependencies. |
| [web-worker-csp](./web-worker-csp/) | The explicit CSP allowance required by blob-backed Web Workers. |

## Authentication and long-lived connections

| Example | What it shows |
|---|---|
| [auth](./auth/) | An OpenID Connect verifier adapted to HyperServe's issuer/subject principal. This is a separate Go module; run it from `examples/auth`. |
| [websocket-demo](./websocket-demo/) | A server-tracked WebSocket upgrade, bounded echo loop, and embedded browser client. |

## MCP

| Example | What it shows |
|---|---|
| [mcp-basic](./mcp-basic/) | A small MCP server with explicitly enabled demonstration capabilities. |
| [mcp-cli](./mcp-cli/) | Application-owned flags selecting HTTP or stdio transport. |
| [mcp-stdio](./mcp-stdio/) | MCP over stdio for a process-supervised host. |
| [mcp-discovery](./mcp-discovery/) | Discovery visibility policies. |
| [mcp-extensions](./mcp-extensions/) | Typed application tools, resources, and subscriptions. |
| [devops](./devops/) | Opt-in MCP observability resources. |
| [mcp-sse](./mcp-sse/) | Deprecated routed-SSE compatibility. |
