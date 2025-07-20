package hyperserve

// MCPAutoDiscovery configures MCP with Claude Code auto-discovery features
// This enables:
// - Unified endpoint compliance with MCP SSE transport specification
// - Automatic discovery endpoints (/.well-known/mcp.json, /mcp/discover)
// - Session management with proper state machine
// - Claude Code integration out of the box
//
// Example usage:
//   srv, err := hyperserve.NewServer(
//       hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPAutoDiscovery()),
//   )
func MCPAutoDiscovery() MCPTransportConfig {
	return func(opts *mcpTransportOptions) {
		// Enable auto-discovery mode
		opts.discoveryMode = true
	}
}

// Add discoveryMode to mcpTransportOptions
// Note: This extends the existing mcpTransportOptions struct
// which is defined in mcp.go