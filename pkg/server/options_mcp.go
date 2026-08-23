package server

import (
	"net/http"
	"time"

	mcp "github.com/osauer/hyperserve/pkg/mcp"
)

// WithMCPSupport enables MCP (Model Context Protocol) support on the server.
// Server name and version identify the server to MCP clients. By default,
// MCP uses HTTP transport on the "/mcp" endpoint; pass mcp.TransportConfig
// values to switch to stdio or to install a preset (DeveloperMode,
// ObservabilityMode).
//
// Example:
//
//	server.NewServer(server.WithMCPSupport("MyServer", "1.0.0"))
func WithMCPSupport(name, version string, configs ...mcp.TransportConfig) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPEnabled = true
		srv.Options.MCPServerName = name
		srv.Options.MCPServerVersion = version

		if len(configs) == 0 {
			srv.Options.MCPTransport = mcp.HTTPTransport
			srv.Options.mcpTransportOpts.Transport = mcp.HTTPTransport
			srv.Options.mcpTransportOpts.Endpoint = srv.Options.MCPEndpoint
		} else {
			for _, cfg := range configs {
				cfg(&srv.Options.mcpTransportOpts)
			}
			srv.Options.MCPTransport = srv.Options.mcpTransportOpts.Transport
			if srv.Options.mcpTransportOpts.Endpoint != "" {
				srv.Options.MCPEndpoint = srv.Options.mcpTransportOpts.Endpoint
			}
		}

		// Presets gate the built-in registration hooks.
		switch {
		case srv.Options.mcpTransportOpts.ObservabilityMode:
			srv.Options.MCPResourcesEnabled = true
			srv.Options.MCPToolsEnabled = false
		case srv.Options.mcpTransportOpts.DeveloperMode:
			srv.Options.MCPResourcesEnabled = true
			srv.Options.MCPToolsEnabled = true
		}

		transportName := "HTTP"
		if srv.Options.MCPTransport == mcp.StdioTransport {
			transportName = "stdio"
		}
		logger.Debug("MCP (Model Context Protocol) support enabled",
			"name", name,
			"version", version,
			"transport", transportName,
			"endpoint", srv.Options.MCPEndpoint,
			"observabilityMode", srv.Options.mcpTransportOpts.ObservabilityMode,
			"developerMode", srv.Options.mcpTransportOpts.DeveloperMode,
		)
		return nil
	}
}

// WithMCPOriginValidator overrides MCP's default same-origin browser policy.
// The validator receives every MCP request and should allow requests without
// Origin when non-browser clients are expected. Use an explicit allowlist and
// do not trust Origin as authentication. Passing nil restores the default.
func WithMCPOriginValidator(validator func(*http.Request) bool) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPOriginValidator = validator
		if srv.mcpHandler != nil {
			srv.mcpHandler.SetOriginValidator(validator)
		}
		return nil
	}
}

// WithMCPEndpoint configures the MCP endpoint path. Default is "/mcp".
func WithMCPEndpoint(endpoint string) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPEndpoint = endpoint
		logger.Debug("MCP endpoint configured", "endpoint", endpoint)
		return nil
	}
}

// WithMCPFileToolRoot scopes MCP file tools to rootDir via os.Root, so
// they cannot read or list paths outside it.
func WithMCPFileToolRoot(rootDir string) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPFileToolRoot = rootDir
		logger.Debug("MCP file tool root configured", "root", rootDir)
		return nil
	}
}

// WithMCPToolCallTimeout sets the per-tool execution budget enforced by the
// MCP handler. Tools that exceed the timeout return context.DeadlineExceeded
// to the caller; see the caveat in contextToolWrapper for what happens to
// the underlying goroutine. Zero or negative values fall back to the
// package default (30s).
func WithMCPToolCallTimeout(d time.Duration) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPToolCallTimeout = d
		return nil
	}
}

// WithMCPProtocolVersion overrides the MCP protocol version advertised to
// clients. Empty values reset to mcp.DefaultProtocolVersion.
func WithMCPProtocolVersion(version string) ServerOptionFunc {
	return func(srv *Server) error {
		if version == "" {
			srv.Options.MCPProtocolVersion = mcp.DefaultProtocolVersion
			return nil
		}
		srv.Options.MCPProtocolVersion = version
		if srv.mcpHandler != nil {
			srv.mcpHandler.SetProtocolVersion(version)
		}
		return nil
	}
}

// WithMCPBuiltinTools toggles the built-in MCP tools (Calculator plus
// sandboxed FileRead / ListDirectory when WithMCPFileToolRoot is set).
// Default off. Requires `_ "github.com/osauer/hyperserve/pkg/mcp/builtin"`
// to be blank-imported by the consumer; otherwise NewServer logs a warning
// and registers nothing.
func WithMCPBuiltinTools(enabled bool) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPToolsEnabled = enabled
		return nil
	}
}

// WithMCPBuiltinResources toggles the built-in MCP resources (Config,
// Metrics, System, ServerLog, ServerHealth). Default off. Same
// blank-import requirement as WithMCPBuiltinTools.
func WithMCPBuiltinResources(enabled bool) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPResourcesEnabled = enabled
		return nil
	}
}
