package server

import "net/http"

// WithMCPDiscoveryPolicy sets the discovery policy for MCP tools and resources
//
// Example:
//
//	srv, _ := hyperserve.NewServer(
//	    hyperserve.WithMCPDiscoveryPolicy(hyperserve.DiscoveryCount),
//	)
func WithMCPDiscoveryPolicy(policy DiscoveryPolicy) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPDiscoveryPolicy = policy
		return nil
	}
}

// WithMCPDiscoveryFilter sets a custom filter function for MCP discovery
//
// The filter function receives the tool name and HTTP request, allowing
// for context-aware filtering based on auth tokens, IP addresses, etc.
//
// Example - Hide admin tools from external requests:
//
//	srv, _ := hyperserve.NewServer(
//	    hyperserve.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
//	        if strings.Contains(toolName, "admin") {
//	            // Only show admin tools to internal IPs
//	            return strings.HasPrefix(r.RemoteAddr, "10.") ||
//	                   strings.HasPrefix(r.RemoteAddr, "192.168.")
//	        }
//	        return true
//	    }),
//	)
//
// Example - RBAC with Bearer token:
//
//	srv, _ := hyperserve.NewServer(
//	    hyperserve.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
//	        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
//	        if token == "" {
//	            return !strings.Contains(toolName, "sensitive")
//	        }
//
//	        // Decode JWT and check claims
//	        claims := decodeJWT(token)
//	        if claims.Role == "admin" {
//	            return true // Admins see everything
//	        }
//
//	        // Check tool permissions in claims
//	        return claims.HasPermission(toolName)
//	    }),
//	)
func WithMCPDiscoveryFilter(filter func(toolName string, r *http.Request) bool) ServerOptionFunc {
	return func(srv *Server) error {
		srv.Options.MCPDiscoveryFilter = filter
		return nil
	}
}
