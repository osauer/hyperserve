// Package hyperserve provides a net/http-shaped Go server with lifecycle,
// middleware, typed request binding, WebSocket integration, and optional Model
// Context Protocol (MCP) endpoints. Routes use http.ServeMux patterns, and
// handlers remain http.Handler values.
//
// Use net/http directly when routes plus JSON are sufficient. HyperServe is
// useful when an application would otherwise assemble the same timeout,
// recovery, graceful shutdown, readiness, input, and protocol plumbing itself.
package hyperserve
