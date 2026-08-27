package hyperserve

import (
	"net/http"

	"github.com/osauer/hyperserve/v2/mcp"
)

// WithMCPDiscoveryPolicy sets the discovery policy for MCP tools and resources.
//
// Example:
//
//	app, _ := hyperserve.New(
//	    hyperserve.WithMCPDiscoveryPolicy(mcp.DiscoveryCount),
//	)
func WithMCPDiscoveryPolicy(policy mcp.DiscoveryPolicy) Option {
	return func(srv *Server) error {
		srv.options.MCPDiscoveryPolicy = policy
		return nil
	}
}

// WithMCPDiscoveryFilter sets a custom filter function for MCP discovery.
//
// The filter function receives the tool name and HTTP request, allowing
// for context-aware filtering based on auth tokens, IP addresses, etc.
//
// Example - Hide admin tools from external requests:
//
//	app, _ := hyperserve.New(
//	    hyperserve.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
//	        if strings.Contains(toolName, "admin") {
//	            return strings.HasPrefix(r.RemoteAddr, "10.") ||
//	                   strings.HasPrefix(r.RemoteAddr, "192.168.")
//	        }
//	        return true
//	    }),
//	)
func WithMCPDiscoveryFilter(filter func(toolName string, r *http.Request) bool) Option {
	return func(srv *Server) error {
		srv.options.MCPDiscoveryFilter = filter
		return nil
	}
}
