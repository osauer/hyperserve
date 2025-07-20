package hyperserve

import (
	"encoding/json"
	"net/http"
	"strings"
)

// DiscoveryPolicy defines how MCP tools and resources are exposed in discovery endpoints
type DiscoveryPolicy int

const (
	// DiscoveryPublic shows all discoverable tools/resources (default)
	DiscoveryPublic DiscoveryPolicy = iota
	// DiscoveryCount only shows counts, not names
	DiscoveryCount
	// DiscoveryAuthenticated shows all if request has valid auth
	DiscoveryAuthenticated
	// DiscoveryNone hides all tool/resource information
	DiscoveryNone
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

	// Add capabilities with dynamic tool/resource information
	if srv.mcpHandler != nil {
		// Get registered tools and resources
		tools := srv.mcpHandler.GetRegisteredTools()
		resources := srv.mcpHandler.GetRegisteredResources()
		
		// Build tool capability info based on policy
		toolCapability := map[string]interface{}{
			"supported": true,
			"count":     len(tools),
		}
		
		// Apply discovery policy for tools
		if srv.shouldIncludeToolList(r) {
			filteredTools := make([]string, 0, len(tools))
			for _, toolName := range tools {
				if srv.shouldExposeToolInDiscovery(toolName, r) {
					filteredTools = append(filteredTools, toolName)
				}
			}
			if len(filteredTools) > 0 {
				toolCapability["available"] = filteredTools
			}
		}
		
		// Build resource capability info
		resourceCapability := map[string]interface{}{
			"supported": true,
			"count":     len(resources),
		}
		
		// Resources follow the same policy as tools
		if srv.shouldIncludeToolList(r) {
			resourceCapability["available"] = resources
		}
		
		info.Capabilities = map[string]interface{}{
			"tools":     toolCapability,
			"resources": resourceCapability,
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

// shouldIncludeToolList determines if tool/resource lists should be included based on policy
func (srv *Server) shouldIncludeToolList(r *http.Request) bool {
	switch srv.Options.MCPDiscoveryPolicy {
	case DiscoveryNone, DiscoveryCount:
		return false
	case DiscoveryAuthenticated:
		// Check for Authorization header
		return r.Header.Get("Authorization") != ""
	case DiscoveryPublic:
		return true
	default:
		return true // Default to public
	}
}

// shouldExposeToolInDiscovery determines if a specific tool should be exposed
func (srv *Server) shouldExposeToolInDiscovery(toolName string, r *http.Request) bool {
	// Use custom filter if provided
	if srv.Options.MCPDiscoveryFilter != nil {
		return srv.Options.MCPDiscoveryFilter(toolName, r)
	}
	
	// Default filtering logic
	switch srv.Options.MCPDiscoveryPolicy {
	case DiscoveryNone:
		return false
		
	case DiscoveryCount:
		return false // Only counts, no names
		
	case DiscoveryAuthenticated:
		// Must have auth to see any tools
		if r.Header.Get("Authorization") == "" {
			return false
		}
		// Fall through to default filtering
		
	case DiscoveryPublic:
		// Apply default filtering rules
	}
	
	// Default rules for all policies except None/Count
	
	// Hide internal tools
	if strings.HasPrefix(toolName, "internal_") || strings.HasPrefix(toolName, "_") {
		return false
	}
	
	// Hide sensitive tools unless in dev mode
	if !srv.Options.MCPDev {
		if strings.Contains(toolName, "debug") || strings.Contains(toolName, "admin") {
			return false
		}
		// Hide dev tools like server_control
		if toolName == "server_control" || toolName == "request_debugger" {
			return false
		}
	}
	
	// Check if tool implements IsDiscoverable
	if tool, exists := srv.mcpHandler.GetToolByName(toolName); exists {
		if discoverable, ok := tool.(interface{ IsDiscoverable() bool }); ok {
			return discoverable.IsDiscoverable()
		}
	}
	
	return true // Default to discoverable
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