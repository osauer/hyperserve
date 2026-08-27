package hyperserve

import (
	"fmt"
	"net/http"
	"time"

	mcp "github.com/osauer/hyperserve/v2/mcp"
)

// WithMCPSupport enables MCP (Model Context Protocol) support on the server.
// Server name and version identify the server to MCP clients. By default,
// MCP uses HTTP transport on the "/mcp" endpoint; pass mcp.TransportConfig
// values to switch to stdio or to install a preset (DeveloperMode,
// ObservabilityMode).
//
// Example:
//
//	hyperserve.New(hyperserve.WithMCPSupport("MyServer", "1.0.0"))
func WithMCPSupport(name, version string, configs ...mcp.TransportConfig) Option {
	return func(srv *Server) error {
		srv.options.MCPEnabled = true
		srv.options.MCPServerName = name
		srv.options.MCPServerVersion = version

		if len(configs) == 0 {
			srv.options.MCPTransport = mcp.HTTPTransport
			srv.options.mcpTransportOpts.Transport = mcp.HTTPTransport
			srv.options.mcpTransportOpts.Endpoint = srv.options.MCPEndpoint
		} else {
			for _, cfg := range configs {
				cfg(&srv.options.mcpTransportOpts)
			}
			srv.options.MCPTransport = srv.options.mcpTransportOpts.Transport
			if srv.options.mcpTransportOpts.Endpoint != "" {
				srv.options.MCPEndpoint = srv.options.mcpTransportOpts.Endpoint
			}
		}
		// Keep exported options truthful for status pages and consumers. The
		// transport options are internal implementation detail; callers should
		// not need them to learn which preset is active.
		srv.options.MCPDev = srv.options.mcpTransportOpts.DeveloperMode
		srv.options.MCPObservability = srv.options.mcpTransportOpts.ObservabilityMode

		// Presets gate the built-in registration hooks.
		switch {
		case srv.options.mcpTransportOpts.ObservabilityMode:
			srv.options.MCPResourcesEnabled = true
			srv.options.MCPToolsEnabled = false
		case srv.options.mcpTransportOpts.DeveloperMode:
			srv.options.MCPResourcesEnabled = true
			srv.options.MCPToolsEnabled = true
		}

		return nil
	}
}

// WithMCPOriginValidator overrides MCP's default same-origin browser policy.
// The validator receives every MCP request and should allow requests without
// Origin when non-browser clients are expected. Use an explicit allowlist and
// do not trust Origin as authentication. Passing nil restores the default.
func WithMCPOriginValidator(validator func(*http.Request) bool) Option {
	return func(srv *Server) error {
		srv.options.MCPOriginValidator = validator
		return nil
	}
}

// WithMCPEndpoint configures the MCP endpoint path. The path must be a clean,
// unescaped, non-root literal without a trailing slash and must not be the
// reserved /.well-known/mcp.json discovery path. Default is "/mcp".
func WithMCPEndpoint(endpoint string) Option {
	return func(srv *Server) error {
		if err := validateMCPEndpoint(endpoint); err != nil {
			return err
		}
		srv.options.MCPEndpoint = endpoint
		return nil
	}
}

// WithMCPFileToolRoot scopes MCP file tools to rootDir via os.Root, so
// they cannot read or list paths outside it.
func WithMCPFileToolRoot(rootDir string) Option {
	return func(srv *Server) error {
		srv.options.MCPFileToolRoot = rootDir
		return nil
	}
}

// WithMCPToolCallTimeout sets the per-tool execution budget enforced by the
// MCP handler. Tools that exceed the timeout return context.DeadlineExceeded
// to the caller. Go cannot stop an uncooperative function: a tool that ignores
// its context can continue in a background goroutine until it returns. Zero or
// negative values fall back to the package default (30s).
func WithMCPToolCallTimeout(d time.Duration) Option {
	return func(srv *Server) error {
		srv.options.MCPToolCallTimeout = d
		return nil
	}
}

// WithMCPProtocolVersion overrides the MCP protocol version advertised to
// clients. Empty values reset to mcp.DefaultProtocolVersion.
func WithMCPProtocolVersion(version string) Option {
	return func(srv *Server) error {
		if version == mcp.StreamableHTTPProtocolVersion {
			return fmt.Errorf("MCP protocol version %s is selected per Streamable HTTP request and cannot be configured as the initialize-era version", version)
		}
		if version == "" {
			srv.options.MCPProtocolVersion = mcp.DefaultProtocolVersion
			return nil
		}
		srv.options.MCPProtocolVersion = version
		return nil
	}
}

// WithMCPLegacyRoutedSSE enables HyperServe's proprietary X-SSE-* routed
// transport. It is disabled by default and should be used only while clients
// migrate to MCP 2026-07-28 Streamable HTTP subscriptions/listen.
//
// Deprecated: use MCP 2026-07-28 Streamable HTTP.
func WithMCPLegacyRoutedSSE(enabled bool) Option {
	return func(srv *Server) error {
		srv.options.MCPLegacyRoutedSSE = enabled
		return nil
	}
}

// WithMCPBuiltinTools toggles the built-in MCP tools (Calculator plus
// sandboxed FileRead / ListDirectory when WithMCPFileToolRoot is set).
// Default off. Requires `_ "github.com/osauer/hyperserve/v2/mcp/builtin"`
// to be blank-imported by the consumer; otherwise New logs a warning
// and registers nothing.
func WithMCPBuiltinTools(enabled bool) Option {
	return func(srv *Server) error {
		srv.options.MCPToolsEnabled = enabled
		return nil
	}
}

// WithMCPBuiltinResources toggles the standard built-in MCP resources:
// Config, Metrics, System, and ServerLog. ServerHealth belongs to the
// MCPObservability preset. Default off. Same blank-import requirement as
// WithMCPBuiltinTools.
func WithMCPBuiltinResources(enabled bool) Option {
	return func(srv *Server) error {
		srv.options.MCPResourcesEnabled = enabled
		return nil
	}
}
