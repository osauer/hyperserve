package mcp

import (
	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

// Transport defines the interface for MCP communication transports.
type Transport interface {
	Send(response *jsonrpc.Response) error
	Receive() (*jsonrpc.Request, error)
	Close() error
}

// TransportConfig configures MCP transport options.
type TransportConfig func(*TransportOptions)

// TransportOptions holds transport configuration. It is exported so callers
// (most notably pkg/server) can inspect the resolved transport selection
// after applying a series of TransportConfig functions.
type TransportOptions struct {
	Transport         TransportType
	Endpoint          string
	ObservabilityMode bool
	DeveloperMode     bool
}

// OverHTTP configures MCP to use HTTP transport with the specified endpoint.
func OverHTTP(endpoint string) TransportConfig {
	return func(o *TransportOptions) {
		o.Transport = HTTPTransport
		o.Endpoint = endpoint
	}
}

// OverStdio configures MCP to use stdio transport.
func OverStdio() TransportConfig {
	return func(o *TransportOptions) {
		o.Transport = StdioTransport
	}
}

// OverSSE configures MCP to use SSE transport. SSE shares the HTTP endpoint;
// transport selection happens per-request based on the Accept header.
func OverSSE(endpoint string) TransportConfig {
	return func(o *TransportOptions) {
		o.Transport = HTTPTransport
		o.Endpoint = endpoint
	}
}

// WithObservabilityMode marks the transport options as opting into the
// observability preset. The preset itself is implemented by callers (e.g.
// pkg/mcp/builtin) which read this flag.
func WithObservabilityMode() TransportConfig {
	return func(o *TransportOptions) {
		o.ObservabilityMode = true
	}
}

// WithDeveloperMode marks the transport options as opting into the developer
// preset.
func WithDeveloperMode() TransportConfig {
	return func(o *TransportOptions) {
		o.DeveloperMode = true
	}
}
