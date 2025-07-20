package hyperserve

import (
	"encoding/json"
	"net/http"
	"strings"
)

// MCPDiscoveryInfo represents the discovery information for MCP endpoints
type MCPDiscoveryInfo struct {
	Version     string                 `json:"version"`
	Transports  []MCPTransportInfo     `json:"transports"`
	Endpoints   map[string]string      `json:"endpoints"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// MCPTransportInfo describes available transport mechanisms
type MCPTransportInfo struct {
	Type        string            `json:"type"`
	Endpoint    string            `json:"endpoint"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// setupDiscoveryEndpoints registers the discovery endpoints for Claude Code
func (srv *Server) setupDiscoveryEndpoints() {
	if !srv.MCPEnabled() {
		return
	}

	// Register /.well-known/mcp.json endpoint
	srv.mux.HandleFunc("/.well-known/mcp.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		discoveryInfo := srv.buildDiscoveryInfo(r)
		
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
		if err := json.NewEncoder(w).Encode(discoveryInfo); err != nil {
			logger.Error("Failed to encode discovery info", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})

	// Register /mcp/discover endpoint
	srv.mux.HandleFunc(srv.Options.MCPEndpoint + "/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		discoveryInfo := srv.buildDiscoveryInfo(r)
		
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(discoveryInfo); err != nil {
			logger.Error("Failed to encode discovery info", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})

	logger.Debug("MCP discovery endpoints registered", 
		"endpoints", []string{"/.well-known/mcp.json", srv.Options.MCPEndpoint + "/discover"})
}

// buildDiscoveryInfo constructs the discovery information based on server configuration
func (srv *Server) buildDiscoveryInfo(r *http.Request) MCPDiscoveryInfo {
	// Determine the base URL
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	
	host := r.Host
	if host == "" {
		host = "localhost" + srv.Options.Addr
	}
	
	baseURL := scheme + "://" + host
	mcpEndpoint := baseURL + srv.Options.MCPEndpoint

	info := MCPDiscoveryInfo{
		Version: MCPVersion,
		Transports: []MCPTransportInfo{
			{
				Type:        "http",
				Endpoint:    mcpEndpoint,
				Description: "Standard HTTP POST requests with JSON-RPC 2.0",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			{
				Type:        "sse",
				Endpoint:    mcpEndpoint,
				Description: "Server-Sent Events for real-time communication",
				Headers: map[string]string{
					"Accept": "text/event-stream",
				},
			},
		},
		Endpoints: map[string]string{
			"mcp":        mcpEndpoint,
			"initialize": mcpEndpoint,
			"tools":      mcpEndpoint,
			"resources":  mcpEndpoint,
		},
	}

	// Add capabilities if available
	if srv.mcpHandler != nil {
		info.Capabilities = map[string]interface{}{
			"tools": map[string]interface{}{
				"supported": true,
			},
			"resources": map[string]interface{}{
				"supported": true,
			},
			"sse": map[string]interface{}{
				"enabled":       true,
				"endpoint":      "same",
				"headerRouting": true,
			},
		}
		
		// Add transport-specific capabilities
		if srv.Options.MCPTransport == StdioTransport {
			info.Capabilities["stdio"] = map[string]interface{}{
				"supported": true,
			}
		}
	}

	return info
}

// getMCPBaseURL returns the base URL for MCP endpoints, handling various host configurations
func getMCPBaseURL(r *http.Request, addr string) string {
	// Check for forwarded headers first
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		scheme := "http"
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		return scheme + "://" + forwardedHost
	}

	// Use the Host header if available
	if r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + r.Host
	}

	// Fallback to configured address
	host := "localhost"
	if addr != "" && !strings.HasPrefix(addr, ":") {
		// Extract host from addr if it's not just a port
		parts := strings.Split(addr, ":")
		if len(parts) > 0 && parts[0] != "" {
			host = parts[0]
		}
	}
	
	// Extract port from addr
	port := "8080"
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		port = addr[idx+1:]
	}
	
	return "http://" + host + ":" + port
}