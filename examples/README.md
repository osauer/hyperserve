# HyperServe examples

Each subdirectory is a self-contained `main` package. Run with `go run ./examples/<name>`.

## Getting started

| Example | What it shows |
|---|---|
| [hello-world](./hello-world/) | The smallest possible server — one route, default options. |
| [static-files](./static-files/) | Serving HTML/CSS/JS with security headers. |
| [json-api](./json-api/) | REST API: CRUD over an in-memory TODO list with request parsing and error handling. |
| [middleware-basics](./middleware-basics/) | Building a middleware stack (logging, rate limiting, CORS) step by step. |
| [configuration](./configuration/) | Env vars, JSON config files, programmatic `With*` options, and their precedence. |

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
| [mcp-cli](./mcp-cli/) | Command-line MCP client hitting an MCP server over HTTP. |
| [mcp-sse](./mcp-sse/) | The unified `/mcp` endpoint serving both regular HTTP and SSE. |
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
| [websocket-demo](./websocket-demo/) | RFC 6455 upgrade + echo handler + minimal browser client. |
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
