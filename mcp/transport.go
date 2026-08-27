package mcp

import (
	jsonrpc "github.com/osauer/hyperserve/v2/jsonrpc"
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
// (most notably the root hyperserve package) can inspect the resolved transport selection
// after applying a series of TransportConfig functions.
type TransportOptions struct {
	Transport         TransportType
	Endpoint          string
	ObservabilityMode bool
	DeveloperMode     bool
}

// OverStdio configures MCP to use stdio transport.
//
// The HTTP/SSE counterparts that used to live here were unused (no caller
// outside this file); HTTP is the default when no Over* config is supplied,
// and SSE shares the HTTP endpoint via Accept-header routing. Configure the
// endpoint path with hyperserve.WithMCPEndpoint instead.
func OverStdio() TransportConfig {
	return func(o *TransportOptions) {
		o.Transport = StdioTransport
	}
}

// WithObservabilityMode marks the transport options as opting into the
// observability preset. The preset itself is implemented by callers (e.g.
// mcp/builtin) which read this flag.
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
