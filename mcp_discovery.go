package hyperserve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// MCPDiscoveryService provides Claude Code discovery endpoints
type MCPDiscoveryService struct {
	serverInfo MCPServerInfo
	logger     *slog.Logger
	baseURL    string // Will be determined from requests
}

// MCPDiscoveryConfig represents the standard .well-known/mcp.json format
type MCPDiscoveryConfig struct {
	Version string                    `json:"version"`
	Servers map[string]MCPServerEntry `json:"servers"`
}

// MCPServerEntry represents a server entry in the discovery configuration
type MCPServerEntry struct {
	Transport string                 `json:"transport"`
	Endpoint  string                 `json:"endpoint"`
	Name      string                 `json:"name,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// MCPDiscoveryInfo provides detailed information about the MCP server
type MCPDiscoveryInfo struct {
	Server      MCPServerInfo          `json:"server"`
	Transport   MCPTransportInfo       `json:"transport"`
	Endpoints   MCPEndpointInfo        `json:"endpoints"`
	Capabilities MCPCapabilities       `json:"capabilities"`
	Claude      MCPClaudeCodeInfo      `json:"claude"`
	Generated   time.Time              `json:"generated"`
}

// MCPTransportInfo describes supported transport methods
type MCPTransportInfo struct {
	HTTP MCPTransportDetails `json:"http"`
	SSE  MCPTransportDetails `json:"sse"`
}

// MCPTransportDetails provides transport-specific information
type MCPTransportDetails struct {
	Supported bool   `json:"supported"`
	Endpoint  string `json:"endpoint"`
	Method    string `json:"method,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// MCPEndpointInfo lists all available endpoints
type MCPEndpointInfo struct {
	MCP       string `json:"mcp"`
	Discovery string `json:"discovery"`
	WellKnown string `json:"wellKnown"`
	Servers   string `json:"servers"`
}

// MCPClaudeCodeInfo provides Claude Code specific integration details
type MCPClaudeCodeInfo struct {
	Setup          MCPClaudeSetup `json:"setup"`
	Configuration  string         `json:"configuration"`
	ExampleCommand string         `json:"exampleCommand"`
}

// MCPClaudeSetup provides setup instructions for Claude Code
type MCPClaudeSetup struct {
	Steps       []string `json:"steps"`
	ConfigPath  string   `json:"configPath"`
	ConfigFile  string   `json:"configFile"`
}

// NewMCPDiscoveryService creates a new discovery service
func NewMCPDiscoveryService(serverInfo MCPServerInfo) *MCPDiscoveryService {
	return &MCPDiscoveryService{
		serverInfo: serverInfo,
		logger:     logger,
	}
}

// HandleDiscoveryRequest handles discovery-related requests
func (d *MCPDiscoveryService) HandleDiscoveryRequest(w http.ResponseWriter, r *http.Request) bool {
	// Update base URL from request
	d.updateBaseURL(r)
	
	switch r.URL.Path {
	case "/.well-known/mcp.json":
		d.handleWellKnownMCP(w, r)
		return true
	case "/.well-known/mcp-servers":
		d.handleWellKnownServers(w, r)
		return true
	default:
		// Check if it's a discover endpoint under the MCP path
		if strings.HasSuffix(r.URL.Path, "/discover") {
			d.handleDiscoverEndpoint(w, r)
			return true
		}
	}
	
	return false // Not a discovery request
}

// handleWellKnownMCP handles /.well-known/mcp.json requests
func (d *MCPDiscoveryService) handleWellKnownMCP(w http.ResponseWriter, r *http.Request) {
	config := d.generateMCPConfig(r)
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
	
	if err := json.NewEncoder(w).Encode(config); err != nil {
		d.logger.Error("Failed to encode MCP config", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	d.logger.Debug("Served MCP discovery config", "client", r.RemoteAddr, "user_agent", r.Header.Get("User-Agent"))
}

// handleWellKnownServers handles /.well-known/mcp-servers requests  
func (d *MCPDiscoveryService) handleWellKnownServers(w http.ResponseWriter, r *http.Request) {
	servers := []MCPServerEntry{
		d.generateServerEntry(r),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"servers": servers,
		"generated": time.Now().UTC(),
	}); err != nil {
		d.logger.Error("Failed to encode MCP servers", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	d.logger.Debug("Served MCP servers list", "client", r.RemoteAddr)
}

// handleDiscoverEndpoint handles /mcp/discover requests
func (d *MCPDiscoveryService) handleDiscoverEndpoint(w http.ResponseWriter, r *http.Request) {
	info := d.generateDiscoveryInfo(r)
	
	// Support both JSON and HTML responses
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/html") {
		d.handleDiscoverHTML(w, r, info)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60") // Short cache for detailed info
	
	if err := json.NewEncoder(w).Encode(info); err != nil {
		d.logger.Error("Failed to encode discovery info", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	d.logger.Debug("Served MCP discovery info", "client", r.RemoteAddr, "format", "json")
}

// handleDiscoverHTML provides an HTML interface for the discovery endpoint
func (d *MCPDiscoveryService) handleDiscoverHTML(w http.ResponseWriter, r *http.Request, info *MCPDiscoveryInfo) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	configJSON, _ := json.MarshalIndent(d.generateMCPConfig(r), "", "  ")
	
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>MCP Discovery - %s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
               max-width: 1000px; margin: 20px auto; padding: 20px; line-height: 1.6; }
        h1, h2, h3 { color: #333; }
        .info-box { background: #f8f9fa; padding: 20px; border-radius: 8px; margin: 20px 0; border: 1px solid #e9ecef; }
        .success-box { background: #d4edda; padding: 15px; border-radius: 5px; border-left: 4px solid #28a745; margin: 20px 0; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; font-size: 14px; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }
        .step { margin: 15px 0; padding: 10px; background: #fff; border-left: 3px solid #007bff; }
        .endpoint { margin: 10px 0; padding: 8px; background: #e9f7ff; border-radius: 4px; }
        table { width: 100%%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f2f2f2; font-weight: 600; }
        .transport-supported { color: #28a745; font-weight: bold; }
        .transport-method { font-family: monospace; background: #f8f9fa; padding: 2px 4px; border-radius: 3px; }
    </style>
</head>
<body>
    <h1>🤖 MCP Discovery - %s</h1>
    
    <div class="success-box">
        <strong>✅ Claude Code Ready!</strong> This server supports automatic discovery and can be used with Claude Code without manual configuration.
    </div>
    
    <div class="info-box">
        <h3>Server Information</h3>
        <table>
            <tr><td><strong>Name:</strong></td><td>%s</td></tr>
            <tr><td><strong>Version:</strong></td><td>%s</td></tr>
            <tr><td><strong>Protocol Version:</strong></td><td>%s</td></tr>
            <tr><td><strong>Generated:</strong></td><td>%s</td></tr>
        </table>
    </div>
    
    <div class="info-box">
        <h3>Supported Transports</h3>
        <table>
            <tr><th>Transport</th><th>Supported</th><th>Endpoint</th><th>Method</th></tr>
            <tr>
                <td>HTTP</td>
                <td class="transport-supported">✅ Yes</td>
                <td><code>%s</code></td>
                <td><span class="transport-method">POST</span></td>
            </tr>
            <tr>
                <td>SSE</td>
                <td class="transport-supported">✅ Yes</td>
                <td><code>%s</code></td>
                <td><span class="transport-method">GET</span> with <code>Accept: text/event-stream</code></td>
            </tr>
        </table>
    </div>
    
    <div class="info-box">
        <h3>Discovery Endpoints</h3>
        <div class="endpoint">📍 <strong>Standard MCP Config:</strong> <a href="/.well-known/mcp.json">/.well-known/mcp.json</a></div>
        <div class="endpoint">📍 <strong>Server List:</strong> <a href="/.well-known/mcp-servers">/.well-known/mcp-servers</a></div>
        <div class="endpoint">📍 <strong>This Discovery Page:</strong> <a href="%s">%s</a></div>
        <div class="endpoint">📍 <strong>Discovery JSON:</strong> <a href="%s?format=json">%s (JSON)</a></div>
    </div>
    
    <div class="info-box">
        <h3>Claude Code Setup</h3>
        <p>To use this server with Claude Code, you have two options:</p>
        
        <div class="step">
            <strong>Option 1: Automatic Discovery (Recommended)</strong><br>
            Claude Code will automatically discover this server when running on the same machine. No configuration needed!
        </div>
        
        <div class="step">
            <strong>Option 2: Manual Configuration</strong><br>
            Add this to your Claude Code configuration:
            <pre>%s</pre>
        </div>
    </div>
    
    <div class="info-box">
        <h3>Testing the Connection</h3>
        <p>Test the HTTP transport:</p>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize", 
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "test-client", "version": "1.0.0"}
    },
    "id": 1
  }'</pre>
    
        <p>Test the SSE transport:</p>
        <pre>curl -N %s \
  -H "Accept: text/event-stream"</pre>
    </div>
    
    <div class="info-box">
        <h3>Raw Configuration</h3>
        <p>Complete MCP configuration for manual setup:</p>
        <pre>%s</pre>
    </div>
    
    <p><small>Powered by <a href="https://github.com/osauer/hyperserve">HyperServe</a> - MCP SSE Transport Compliant</small></p>
</body>
</html>`, 
		info.Server.Name,
		info.Server.Name,
		info.Server.Name, info.Server.Version, MCPVersion, info.Generated.Format(time.RFC3339),
		info.Transport.HTTP.Endpoint, info.Transport.SSE.Endpoint,
		r.URL.Path, r.URL.Path, r.URL.Path, r.URL.Path,
		configJSON,
		info.Transport.HTTP.Endpoint, info.Transport.SSE.Endpoint,
		configJSON,
	)
	
	d.logger.Debug("Served MCP discovery info", "client", r.RemoteAddr, "format", "html")
}

// generateMCPConfig creates the standard MCP configuration
func (d *MCPDiscoveryService) generateMCPConfig(r *http.Request) *MCPDiscoveryConfig {
	serverKey := strings.ToLower(strings.ReplaceAll(d.serverInfo.Name, " ", "-"))
	
	return &MCPDiscoveryConfig{
		Version: "1.0",
		Servers: map[string]MCPServerEntry{
			serverKey: d.generateServerEntry(r),
		},
	}
}

// generateServerEntry creates a server entry for discovery
func (d *MCPDiscoveryService) generateServerEntry(r *http.Request) MCPServerEntry {
	baseURL := d.getBaseURL(r)
	endpoint := baseURL + "/mcp"
	
	return MCPServerEntry{
		Transport: "sse", // Prefer SSE transport as it's more capable
		Endpoint:  endpoint,
		Name:      d.serverInfo.Name,
		Version:   d.serverInfo.Version,
		Metadata: map[string]interface{}{
			"supportsHTTP":    true,
			"supportsSSE":     true,
			"unified":         true,
			"discoverable":    true,
			"protocolVersion": MCPVersion,
		},
	}
}

// generateDiscoveryInfo creates detailed discovery information
func (d *MCPDiscoveryService) generateDiscoveryInfo(r *http.Request) *MCPDiscoveryInfo {
	baseURL := d.getBaseURL(r)
	mcpEndpoint := baseURL + "/mcp"
	
	return &MCPDiscoveryInfo{
		Server: d.serverInfo,
		Transport: MCPTransportInfo{
			HTTP: MCPTransportDetails{
				Supported: true,
				Endpoint:  mcpEndpoint,
				Method:    "POST",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			SSE: MCPTransportDetails{
				Supported: true,
				Endpoint:  mcpEndpoint,
				Method:    "GET",
				Headers: map[string]string{
					"Accept": "text/event-stream",
				},
			},
		},
		Endpoints: MCPEndpointInfo{
			MCP:       mcpEndpoint,
			Discovery: mcpEndpoint + "/discover",
			WellKnown: baseURL + "/.well-known/mcp.json",
			Servers:   baseURL + "/.well-known/mcp-servers",
		},
		Capabilities: MCPCapabilities{
			Resources: &ResourcesCapability{
				Subscribe:   false,
				ListChanged: false,
			},
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		Claude: MCPClaudeCodeInfo{
			Setup: MCPClaudeSetup{
				Steps: []string{
					"Claude Code will auto-discover this server when running locally",
					"Alternatively, add the configuration below to your Claude Code settings",
					"Restart Claude Code to pick up the new server",
					"The server will appear in Claude Code's MCP server list",
				},
				ConfigPath: "~/.config/claude-code/mcp.json",
				ConfigFile: "mcp.json",
			},
			Configuration:  d.generateClaudeConfig(r),
			ExampleCommand: fmt.Sprintf("curl -X POST %s -H 'Content-Type: application/json' -d '{\"jsonrpc\":\"2.0\",\"method\":\"tools/list\",\"id\":1}'", mcpEndpoint),
		},
		Generated: time.Now().UTC(),
	}
}

// generateClaudeConfig generates Claude Code specific configuration
func (d *MCPDiscoveryService) generateClaudeConfig(r *http.Request) string {
	config := d.generateMCPConfig(r)
	configBytes, _ := json.MarshalIndent(config, "", "  ")
	return string(configBytes)
}

// updateBaseURL updates the base URL from the request
func (d *MCPDiscoveryService) updateBaseURL(r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	
	// Handle X-Forwarded-Proto for reverse proxies
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	
	host := r.Host
	if host == "" {
		host = "localhost:8080" // Fallback
	}
	
	d.baseURL = fmt.Sprintf("%s://%s", scheme, host)
}

// getBaseURL returns the base URL for the request
func (d *MCPDiscoveryService) getBaseURL(r *http.Request) string {
	d.updateBaseURL(r)
	return d.baseURL
}