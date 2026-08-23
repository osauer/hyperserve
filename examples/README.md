# HyperServe examples

Each subdirectory is a self-contained `main` package. Run with `go run ./examples/<name>`.

## Getting started

| Example | What it shows |
|---|---|
| [hello-world](./hello-world/) | The smallest possible server — one route, default options. |
| [static-files](./static-files/) | Serving HTML/CSS/JS with security headers. |
| [json-api](./json-api/) | Method-aware CRUD API using bounded `BindJSON` input and structured errors. |
| [middleware-basics](./middleware-basics/) | Default middleware plus global security headers and route-scoped rate limiting. |
| [configuration](./configuration/) | The exact precedence of defaults, JSON, environment, and functional options. |
| [binding](./binding/) | `BindJSON`/`BindQuery`/`BindForm` + struct-tag validation, with structured 400 responses. |

## HTMX / templating

| Example | What it shows |
|---|---|
| [htmx-dynamic](./htmx-dynamic/) | Server-rendered templates with HTMX-driven partial updates. |
| [htmx-stream](./htmx-stream/) | Live HTMX updates over Server-Sent Events. |

## Auth & enterprise

| Example | What it shows |
|---|---|
| [auth](./auth/) | JWT (RS256), API keys, Basic auth, per-token rate limiting, RBAC. |
| [enterprise](./enterprise/) | FIPS-approved TLS cipher suites + post-quantum key exchange. Generate certs first (`./generate_certs.sh`). |

## MCP (Model Context Protocol)

| Example | What it shows |
|---|---|
| [mcp-basic](./mcp-basic/) | Smallest MCP server: enable, expose built-in tools/resources. |
| [mcp-cli](./mcp-cli/) | An MCP server configured by application-owned flags for HTTP or stdio. |
| [mcp-sse](./mcp-sse/) | Current Streamable HTTP plus isolated legacy routed-SSE compatibility on `/mcp`. |
| [mcp-stdio](./mcp-stdio/) | MCP over stdio for embedding in editors / process-supervised hosts. |
| [mcp-discovery](./mcp-discovery/) | `/.well-known/mcp.json` discovery with policy filtering. |
| [mcp-extensions](./mcp-extensions/) | Custom MCP tools and resources beyond the built-ins. |

## Operations / lifecycle

| Example | What it shows |
|---|---|
| [deferred-init](./deferred-init/) | `WithDeferredInit` + `WithOnReady` — serve `/healthz` while bootstrap runs. |
| [devops](./devops/) | Health endpoints + metrics + graceful shutdown wired together. |
| [best-practices](./best-practices/) | Idiomatic patterns: error handling, logging, structured config. |
| [complete](./complete/) | Multiple features in one binary, useful as a reference implementation. |

## WebSocket / browser

| Example | What it shows |
|---|---|
| [websocket-demo](./websocket-demo/) | Server-tracked RFC 6455 upgrade, bounded echo loop, and embedded browser client. |
| [web-worker-csp](./web-worker-csp/) | `WithCSPWebWorkerSupport()` for blob:-URL Web Workers (Tone.js, PDF.js, etc.). |

## Running

```bash
go run ./examples/hello-world
```

Most examples listen on `:8080`. The `enterprise` example uses `:8443` for HTTPS and
requires generated certs.

Test a running example with `curl`:

```bash
curl http://localhost:8080/
```
