package server

import (
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

// WithMCPBuiltinTools toggles the built-in MCP tools (Calculator, FileRead,
// HTTPRequest, ListDirectory). Default off. Requires
// `_ "github.com/osauer/hyperserve/pkg/mcp/builtin"` to be blank-imported by
// the consumer; otherwise NewServer logs a warning and registers nothing.
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
