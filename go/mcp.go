package hyperserve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MCP Protocol constants
const (
	MCPVersion = "2024-11-05"
)

// MCPTransportType represents the type of transport for MCP communication
type MCPTransportType int

const (
	// HTTPTransport represents HTTP-based MCP communication
	HTTPTransport MCPTransportType = iota
	// StdioTransport represents stdio-based MCP communication
	StdioTransport
)

// MCPTransport defines the interface for MCP communication transports
type MCPTransport interface {
	// Send sends a JSON-RPC response message
	Send(response *JSONRPCResponse) error
	// Receive receives a JSON-RPC request message
	Receive() (*JSONRPCRequest, error)
	// Close closes the transport
	Close() error
}

// MCPTransportConfig is a function that configures MCP transport options
type MCPTransportConfig func(*mcpTransportOptions)

// mcpTransportOptions holds internal transport configuration
type mcpTransportOptions struct {
	transport          MCPTransportType
	endpoint           string
	observabilityMode  bool   // If true, only register observability resources
	developerMode      bool   // If true, enable developer tools (NEVER in production!)
}

// MCPTool defines the interface for Model Context Protocol tools.
type MCPTool interface {
	Name() string
	Description() string
	Schema() map[string]interface{}
	Execute(params map[string]interface{}) (interface{}, error)
}

// MCPResource defines the interface for Model Context Protocol resources.
type MCPResource interface {
	URI() string
	Name() string
	Description() string
	MimeType() string
	Read() (interface{}, error)
	List() ([]string, error)
}

// MCPToolWithContext is an enhanced interface that supports context for cancellation and timeouts
type MCPToolWithContext interface {
	MCPTool
	ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// MCPCapabilities represents the server's MCP capabilities
type MCPCapabilities struct {
	Experimental   map[string]interface{} `json:"experimental,omitempty"`
	Logging        *LoggingCapability     `json:"logging,omitempty"`
	Prompts        *PromptsCapability     `json:"prompts,omitempty"`
	Resources      *ResourcesCapability   `json:"resources,omitempty"`
	Tools          *ToolsCapability       `json:"tools,omitempty"`
	Sampling       *SamplingCapability    `json:"sampling,omitempty"`
	SSE            *SSECapability         `json:"sse,omitempty"`
}

// LoggingCapability represents the server's logging capability.
type LoggingCapability struct{}

// PromptsCapability represents the server's prompt handling capability.
type PromptsCapability struct{}

// ResourcesCapability represents the server's resource management capabilities.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability represents the server's tool execution capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability represents the server's sampling capability.
type SamplingCapability struct{}

// SSECapability represents the server's Server-Sent Events capability.
type SSECapability struct {
	Enabled        bool   `json:"enabled"`
	Endpoint       string `json:"endpoint"`
	HeaderRouting  bool   `json:"headerRouting"`
}

// MCPServerInfo represents MCP server information
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPClientInfo represents MCP client information
type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPOverHTTP configures MCP to use HTTP transport with the specified endpoint
func MCPOverHTTP(endpoint string) MCPTransportConfig {
	return func(o *mcpTransportOptions) {
		o.transport = HTTPTransport
		o.endpoint = endpoint
	}
}

// MCPOverStdio configures MCP to use stdio transport
func MCPOverStdio() MCPTransportConfig {
	return func(o *mcpTransportOptions) {
		o.transport = StdioTransport
	}
}

// MCPNamespace represents a named collection of MCP tools and resources
type MCPNamespace struct {
	Name      string
	Tools     []MCPTool
	Resources []MCPResource
}

// MCPNamespaceConfig is a function that configures namespace options
type MCPNamespaceConfig func(*MCPNamespace)

// WithNamespaceTools adds tools to a namespace
func WithNamespaceTools(tools ...MCPTool) MCPNamespaceConfig {
	return func(ns *MCPNamespace) {
		ns.Tools = append(ns.Tools, tools...)
	}
}

// WithNamespaceResources adds resources to a namespace
func WithNamespaceResources(resources ...MCPResource) MCPNamespaceConfig {
	return func(ns *MCPNamespace) {
		ns.Resources = append(ns.Resources, resources...)
	}
}

// MCPHandler manages MCP protocol communication with multiple namespace support
type MCPHandler struct {
	tools       map[string]MCPTool    // Flat map with prefixed keys: mcp__namespace__toolname
	resources   map[string]MCPResource // Flat map with prefixed keys: mcp__namespace__resourcename
	namespaces  map[string]*MCPNamespace // Track registered namespaces
	rpcEngine   *JSONRPCEngine
	serverInfo  MCPServerInfo
	logger      *slog.Logger
	transport   MCPTransport
	metrics     *MCPMetrics
	cache       *resourceCache
	sseManager  *SSEManager
	sseRequests map[string]chan *JSONRPCRequest // Maps SSE client IDs to request channels
	sseMutex    sync.RWMutex
}

// httpTransport implements MCPTransport for HTTP-based communication
type httpTransport struct {
	w      http.ResponseWriter
	r      *http.Request
	logger *slog.Logger
}

// newHTTPTransport creates a new HTTP transport
func newHTTPTransport(w http.ResponseWriter, r *http.Request) *httpTransport {
	return &httpTransport{
		w:      w,
		r:      r,
		logger: logger,
	}
}

// Send sends a JSON-RPC response over HTTP
func (t *httpTransport) Send(response *JSONRPCResponse) error {
	t.w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(t.w).Encode(response)
}

// Receive receives a JSON-RPC request from HTTP
func (t *httpTransport) Receive() (*JSONRPCRequest, error) {
	if t.r.Method != http.MethodPost {
		return nil, fmt.Errorf("method not allowed: %s", t.r.Method)
	}
	
	if !strings.Contains(t.r.Header.Get("Content-Type"), "application/json") {
		return nil, fmt.Errorf("Content-Type must be application/json")
	}
	
	var request JSONRPCRequest
	if err := json.NewDecoder(t.r.Body).Decode(&request); err != nil {
		return nil, fmt.Errorf("failed to decode request: %w", err)
	}
	
	return &request, nil
}

// Close closes the HTTP transport (no-op for HTTP)
func (t *httpTransport) Close() error {
	return nil
}

// NewMCPHandler creates a new MCP handler instance
func NewMCPHandler(serverInfo MCPServerInfo) *MCPHandler {
	handler := &MCPHandler{
		tools:       make(map[string]MCPTool),
		resources:   make(map[string]MCPResource),
		namespaces:  make(map[string]*MCPNamespace),
		rpcEngine:   NewJSONRPCEngine(),
		serverInfo:  serverInfo,
		logger:      logger,
		metrics:     newMCPMetrics(),
		cache:       newResourceCache(100), // Default cache size of 100 items
		sseManager:  NewSSEManager(),
		sseRequests: make(map[string]chan *JSONRPCRequest),
	}
	
	// Register MCP protocol methods
	handler.registerMCPMethods()
	
	return handler
}

// formatToolName creates a namespaced tool name in the format mcp__namespace__toolname
func (h *MCPHandler) formatToolName(namespace, toolName string) string {
	return fmt.Sprintf("mcp__%s__%s", namespace, toolName)
}

// formatResourceName creates a namespaced resource name in the format mcp__namespace__resourcename  
func (h *MCPHandler) formatResourceName(namespace, resourceName string) string {
	return fmt.Sprintf("mcp__%s__%s", namespace, resourceName)
}


// RegisterTool registers an MCP tool without namespace prefixing (for simplicity)
func (h *MCPHandler) RegisterTool(tool MCPTool) {
	h.tools[tool.Name()] = tool
	h.logger.Debug("MCP tool registered", "tool", tool.Name())
}

// RegisterToolInNamespace registers an MCP tool in the specified namespace
// This always applies namespace prefixing
func (h *MCPHandler) RegisterToolInNamespace(tool MCPTool, namespace string) {
	// Use server name as default namespace if empty
	if namespace == "" {
		namespace = h.serverInfo.Name
	}
	
	prefixedName := h.formatToolName(namespace, tool.Name())
	h.tools[prefixedName] = tool
	h.logger.Debug("MCP tool registered in namespace", "tool", tool.Name(), "namespace", namespace, "prefixedName", prefixedName)
}


// RegisterResource registers an MCP resource without namespace prefixing (for simplicity)
func (h *MCPHandler) RegisterResource(resource MCPResource) {
	h.resources[resource.URI()] = resource
	h.logger.Debug("MCP resource registered", "resource", resource.Name(), "uri", resource.URI())
}

// RegisterResourceInNamespace registers an MCP resource in the specified namespace
// This always applies namespace prefixing
func (h *MCPHandler) RegisterResourceInNamespace(resource MCPResource, namespace string) {
	if namespace == "" {
		h.logger.Error("Cannot register resource without namespace", "resource", resource.Name())
		return
	}
	
	prefixedURI := h.formatResourceName(namespace, resource.URI())
	h.resources[prefixedURI] = resource
	h.logger.Debug("MCP resource registered in namespace", "resource", resource.Name(), "namespace", namespace, "uri", resource.URI(), "prefixedURI", prefixedURI)
}

// RegisterNamespace registers an entire namespace with its tools and resources
func (h *MCPHandler) RegisterNamespace(name string, configs ...MCPNamespaceConfig) error {
	if name == "" {
		return fmt.Errorf("namespace name cannot be empty")
	}
	
	// Create namespace
	ns := &MCPNamespace{
		Name:      name,
		Tools:     make([]MCPTool, 0),
		Resources: make([]MCPResource, 0),
	}
	
	// Apply configurations
	for _, config := range configs {
		config(ns)
	}
	
	// Register tools
	for _, tool := range ns.Tools {
		h.RegisterToolInNamespace(tool, name)
	}
	
	// Register resources
	for _, resource := range ns.Resources {
		h.RegisterResourceInNamespace(resource, name)
	}
	
	// Store namespace
	h.namespaces[name] = ns
	
	h.logger.Debug("MCP namespace registered", "namespace", name, "tools", len(ns.Tools), "resources", len(ns.Resources))
	return nil
}

// GetMetrics returns the current MCP metrics summary
func (h *MCPHandler) GetMetrics() map[string]interface{} {
	if h.metrics == nil {
		return nil
	}
	return h.metrics.GetMetricsSummary()
}

// GetRegisteredTools returns a list of all registered tool names
func (h *MCPHandler) GetRegisteredTools() []string {
	tools := make([]string, 0, len(h.tools))
	for name := range h.tools {
		tools = append(tools, name)
	}
	return tools
}

// GetRegisteredResources returns a list of all registered resource URIs
func (h *MCPHandler) GetRegisteredResources() []string {
	resources := make([]string, 0, len(h.resources))
	for uri := range h.resources {
		resources = append(resources, uri)
	}
	return resources
}

// GetToolByName returns a tool by its name (for discovery filtering)
func (h *MCPHandler) GetToolByName(name string) (MCPTool, bool) {
	tool, exists := h.tools[name]
	return tool, exists
}

// getCapabilities returns the server's MCP capabilities
func (h *MCPHandler) getCapabilities() MCPCapabilities {
	return MCPCapabilities{
		Resources: &ResourcesCapability{
			Subscribe:   false,
			ListChanged: false,
		},
		Tools: &ToolsCapability{
			ListChanged: false,
		},
		SSE: &SSECapability{
			Enabled:       true,
			Endpoint:      "same",
			HeaderRouting: true,
		},
	}
}

// ProcessRequest processes an MCP request
func (h *MCPHandler) ProcessRequest(requestData []byte) []byte {
	return h.rpcEngine.ProcessRequest(requestData)
}

// isJSONAccepted checks if the Accept header indicates JSON is acceptable
func isJSONAccepted(accept string) bool {
	if accept == "" {
		return false
	}
	
	// Convert to lowercase for case-insensitive matching
	accept = strings.ToLower(accept)
	
	// Handle */* (accept anything)
	if accept == "*/*" {
		return true
	}
	
	// Parse Accept header by splitting on commas
	for _, part := range strings.Split(accept, ",") {
		// Extract media type (before semicolon for quality factors)
		mediaType := strings.TrimSpace(strings.Split(part, ";")[0])
		
		// Check for JSON-compatible media types
		if mediaType == "application/json" || 
		   mediaType == "*/*" || 
		   mediaType == "application/*" {
			return true
		}
	}
	
	return false
}

// ServeHTTP implements the http.Handler interface for MCP
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Debug logging for tests
	if h.logger.Enabled(context.Background(), slog.LevelDebug) {
		h.logger.Debug("MCP ServeHTTP called", "path", r.URL.Path, "method", r.Method)
	}
	
	// Unified handler: Route based on Accept header
	if r.Header.Get("Accept") == "text/event-stream" {
		h.sseManager.HandleSSE(w, r, h)
		return
	}
	
	// Handle GET requests with helpful information
	if r.Method == http.MethodGet {
		accept := r.Header.Get("Accept")
		if isJSONAccepted(accept) {
			// Return JSON status for tools like Claude Code
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			status := map[string]interface{}{
				"status": "ready",
				"server": h.serverInfo,
				"capabilities": h.getCapabilities(),
				"endpoint": r.URL.Path,
				"transport": "http",
			}
			if err := json.NewEncoder(w).Encode(status); err != nil {
				h.logger.Error("Failed to encode JSON status", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>MCP Endpoint - HyperServe</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
               max-width: 800px; margin: 50px auto; padding: 20px; line-height: 1.6; }
        h1 { color: #333; }
        code { background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }
        pre { background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }
        .example { margin: 20px 0; }
        .note { background: #e8f4f8; padding: 15px; border-left: 4px solid #0084c7; margin: 20px 0; }
    </style>
</head>
<body>
    <h1>Model Context Protocol (MCP) Endpoint</h1>
    
    <p>This endpoint implements the <a href="https://modelcontextprotocol.io">Model Context Protocol</a> 
    for AI assistant integration.</p>
    
    <div class="note">
        <strong>Note:</strong> MCP uses JSON-RPC 2.0 over HTTP POST. GET requests are not supported.
    </div>
    
    <h2>How to Use</h2>
    
    <div class="example">
        <h3>Initialize Connection</h3>
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
    </div>
    
    <div class="example">
        <h3>List Available Tools</h3>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "tools/list", "id": 2}'</pre>
    </div>
    
    <div class="example">
        <h3>List Available Resources</h3>
        <pre>curl -X POST %s \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "resources/list", "id": 3}'</pre>
    </div>
    
    <h2>Available Methods</h2>
    <ul>
        <li><code>initialize</code> - Initialize MCP session</li>
        <li><code>ping</code> - Test connectivity</li>
        <li><code>tools/list</code> - List available tools</li>
        <li><code>tools/call</code> - Execute a tool</li>
        <li><code>resources/list</code> - List available resources</li>
        <li><code>resources/read</code> - Read a resource</li>
    </ul>
    
    <h2>Server-Sent Events (SSE) Support</h2>
    <p>This server also supports SSE for real-time communication:</p>
    <ul>
        <li>SSE endpoint: <code>%s/sse</code></li>
        <li>Send requests to <code>%s</code> with header <code>X-SSE-Client-ID: {your-client-id}</code></li>
        <li>Responses will be delivered via the SSE connection</li>
    </ul>
    
    <h2>More Information</h2>
    <p>For detailed documentation, see the <a href="https://github.com/osauer/hyperserve">HyperServe GitHub repository</a>.</p>
</body>
</html>`, r.URL.Path, r.URL.Path, r.URL.Path, r.URL.Path, r.URL.Path)
		return
	}
	
	// Check if this is a request that should route responses through SSE
	clientID := r.Header.Get("X-SSE-Client-ID")
	if clientID != "" {
		// This request wants responses via SSE
		h.handleSSERoutedRequest(w, r, clientID)
		return
	}
	
	// Create HTTP transport for this request
	transport := newHTTPTransport(w, r)
	defer transport.Close()
	
	// Process the request using the transport
	if err := h.ProcessRequestWithTransport(transport); err != nil {
		h.logger.Error("Failed to process MCP request", "error", err)
		if strings.Contains(err.Error(), "method not allowed") {
			http.Error(w, "Method not allowed. MCP requires POST requests.", http.StatusMethodNotAllowed)
		} else if strings.Contains(err.Error(), "Content-Type") {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// ProcessRequestWithTransport processes an MCP request using the provided transport
func (h *MCPHandler) ProcessRequestWithTransport(transport MCPTransport) error {
	start := time.Now()
	
	// Receive request
	request, err := transport.Receive()
	if err != nil {
		return fmt.Errorf("failed to receive request: %w", err)
	}
	
	// Process with JSON-RPC engine directly (avoiding double marshaling)
	response := h.rpcEngine.ProcessRequestDirect(request)
	
	// Record metrics
	var responseErr error
	if response.Error != nil {
		responseErr = fmt.Errorf("error: %s", response.Error.Message)
	}
	h.metrics.recordRequest(request.Method, time.Since(start), responseErr)
	
	// Send response
	if err := transport.Send(response); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	
	return nil
}

// registerMCPMethods registers all MCP protocol methods with the JSON-RPC engine
func (h *MCPHandler) registerMCPMethods() {
	// Initialize methods
	h.rpcEngine.RegisterMethod("initialize", h.handleInitialize)
	h.rpcEngine.RegisterMethod("initialized", h.handleInitialized)
	
	// Resource methods
	h.rpcEngine.RegisterMethod("resources/list", h.handleResourcesList)
	h.rpcEngine.RegisterMethod("resources/read", h.handleResourcesRead)
	
	// Tool methods
	h.rpcEngine.RegisterMethod("tools/list", h.handleToolsList)
	h.rpcEngine.RegisterMethod("tools/call", h.handleToolsCall)
	
	// Utility methods
	h.rpcEngine.RegisterMethod("ping", h.handlePing)
}

// MCPInitializeParams represents the parameters for the initialize method
type MCPInitializeParams struct {
	ProtocolVersion string        `json:"protocolVersion"`
	Capabilities    interface{}   `json:"capabilities"`
	ClientInfo      MCPClientInfo `json:"clientInfo"`
}

// MCPInitializeResult represents the result of the initialize method
type MCPInitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo   `json:"serverInfo"`
}

// MCPResourceReadParams represents the parameters for reading a resource
type MCPResourceReadParams struct {
	URI string `json:"uri"`
}

// MCPToolCallParams represents the parameters for calling a tool
type MCPToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCPToolInfo represents information about a tool
type MCPToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPResourceInfo represents information about a resource
type MCPResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// MCPResourceContent represents the content of a resource
type MCPResourceContent struct {
	URI      string      `json:"uri"`
	MimeType string      `json:"mimeType"`
	Text     interface{} `json:"text"`
}

// MCPToolResult represents the result of a tool execution
type MCPToolResult struct {
	Content []map[string]interface{} `json:"content"`
}

// MCP method handlers

func (h *MCPHandler) handleInitialize(params interface{}) (interface{}, error) {
	var initParams MCPInitializeParams
	
	// Parse parameters
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		
		if err := json.Unmarshal(paramBytes, &initParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal init params: %w", err)
		}
	}
	
	h.logger.Debug("MCP client initialized", "client", initParams.ClientInfo.Name, "version", initParams.ClientInfo.Version)
	
	// Return server capabilities
	return map[string]interface{}{
		"protocolVersion": MCPVersion,
		"capabilities": h.getCapabilities(),
		"serverInfo": h.serverInfo,
		"instructions": "Follow the initialization protocol: after receiving this response, send an 'initialized' notification, then the server will send a 'ready' notification. For SSE support, connect to the SAME endpoint with 'Accept: text/event-stream' header.",
	}, nil
}

func (h *MCPHandler) handleInitialized(params interface{}) (interface{}, error) {
	// The initialized notification doesn't require a response
	h.logger.Debug("MCP client confirmed initialization")
	return nil, nil
}

func (h *MCPHandler) handleResourcesList(params interface{}) (interface{}, error) {
	resources := make([]map[string]interface{}, 0, len(h.resources))
	
	for prefixedURI, resource := range h.resources {
		resources = append(resources, map[string]interface{}{
			"uri":         prefixedURI, // Use the prefixed URI that clients will request
			"name":        resource.Name(),
			"description": resource.Description(),
			"mimeType":    resource.MimeType(),
		})
	}
	
	return map[string]interface{}{
		"resources": resources,
	}, nil
}

func (h *MCPHandler) handleResourcesRead(params interface{}) (interface{}, error) {
	start := time.Now()
	var readParams MCPResourceReadParams
	
	// Parse parameters
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		
		// Debug logging to understand what parameters are being passed
		// Only perform expensive marshaling if debug logging is enabled
		if h.logger.Enabled(context.Background(), slog.LevelDebug) {
			h.logger.Debug("MCP resources/read parameters received", "params", string(paramBytes))
		}
		
		if err := json.Unmarshal(paramBytes, &readParams); err != nil {
			// Check if the client is mistakenly sending tool call parameters
			if paramsMap, ok := params.(map[string]interface{}); ok {
				if _, hasArguments := paramsMap["arguments"]; hasArguments {
					return nil, fmt.Errorf("invalid parameters: resources/read expects 'uri' parameter, not 'arguments'. Use tools/call for tool execution")
				}
			}
			return nil, fmt.Errorf("failed to unmarshal read params: %w", err)
		}
		
		// Check if the client is mistakenly sending tool call parameters even if unmarshaling succeeded
		if paramsMap, ok := params.(map[string]interface{}); ok {
			if _, hasArguments := paramsMap["arguments"]; hasArguments {
				return nil, fmt.Errorf("invalid parameters: resources/read expects 'uri' parameter, not 'arguments'. Use tools/call for tool execution")
			}
		}
	}
	
	// Validate that URI parameter is provided
	if readParams.URI == "" {
		return nil, fmt.Errorf("uri parameter is required for resources/read method")
	}
	
	resource, exists := h.resources[readParams.URI]
	if !exists {
		return nil, fmt.Errorf("resource not found: %s", readParams.URI)
	}
	
	// Check cache first
	cacheKey := readParams.URI
	cacheHit := false
	if cachedContent, hit := h.cache.get(cacheKey); hit {
		cacheHit = true
		h.metrics.recordResourceRead(readParams.URI, time.Since(start), nil, true)
		
		// Return cached content
		return map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      resource.URI(),
					"mimeType": resource.MimeType(),
					"text":     cachedContent,
				},
			},
		}, nil
	}
	
	// Read from resource
	content, err := resource.Read()
	
	// Record metrics
	h.metrics.recordResourceRead(readParams.URI, time.Since(start), err, cacheHit)
	
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}
	
	// Convert content to string if it's not already
	var textContent string
	switch v := content.(type) {
	case string:
		textContent = v
	case []byte:
		textContent = string(v)
	default:
		// For any other type (maps, structs, etc.), marshal to JSON
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal resource content to JSON: %w", err)
		}
		textContent = string(jsonBytes)
	}
	
	// Cache the string result (with 5 minute TTL for now)
	h.cache.set(cacheKey, textContent, 5*time.Minute)
	
	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      resource.URI(),
				"mimeType": resource.MimeType(),
				"text":     textContent,
			},
		},
	}, nil
}

func (h *MCPHandler) handleToolsList(params interface{}) (interface{}, error) {
	tools := make([]map[string]interface{}, 0, len(h.tools))
	
	for prefixedName, tool := range h.tools {
		tools = append(tools, map[string]interface{}{
			"name":        prefixedName, // Use the prefixed name that clients will call
			"description": tool.Description(),
			"inputSchema": tool.Schema(),
		})
	}
	
	return map[string]interface{}{
		"tools": tools,
	}, nil
}

func (h *MCPHandler) handleToolsCall(params interface{}) (interface{}, error) {
	start := time.Now()
	var callParams MCPToolCallParams
	
	// Parse parameters
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		
		if err := json.Unmarshal(paramBytes, &callParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal call params: %w", err)
		}
	}
	
	tool, exists := h.tools[callParams.Name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", callParams.Name)
	}
	
	// Wrap tool to support context if needed
	ctxTool := wrapToolWithContext(tool)
	
	// Create context with timeout (default 30 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Execute tool with context
	result, err := ctxTool.ExecuteWithContext(ctx, callParams.Arguments)
	
	// Record metrics
	h.metrics.recordToolExecution(callParams.Name, time.Since(start), err)
	
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}
	
	// Handle different response types
	var content []map[string]interface{}
	
	switch v := result.(type) {
	case string:
		// Simple string response
		content = []map[string]interface{}{
			{
				"type": "text",
				"text": v,
			},
		}
	case map[string]interface{}:
		// Check if it's already an MCP-formatted response with content array
		if existingContent, ok := v["content"].([]map[string]interface{}); ok {
			content = existingContent
		} else if existingContent, ok := v["content"].([]interface{}); ok {
			// Convert []interface{} to []map[string]interface{}
			content = make([]map[string]interface{}, len(existingContent))
			for i, item := range existingContent {
				if m, ok := item.(map[string]interface{}); ok {
					content[i] = m
				} else {
					// Fallback: convert to JSON string
					jsonBytes, _ := json.Marshal(v)
					content = []map[string]interface{}{
						{
							"type": "text",
							"text": string(jsonBytes),
						},
					}
					break
				}
			}
		} else {
			// Regular map response - convert to JSON string
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool response: %w", err)
			}
			content = []map[string]interface{}{
				{
					"type": "text",
					"text": string(jsonBytes),
				},
			}
		}
	case []interface{}:
		// Array response - convert to JSON string
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool response: %w", err)
		}
		content = []map[string]interface{}{
			{
				"type": "text",
				"text": string(jsonBytes),
			},
		}
	default:
		// For any other type (structs, etc.), marshal to JSON
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool response: %w", err)
		}
		content = []map[string]interface{}{
			{
				"type": "text",
				"text": string(jsonBytes),
			},
		}
	}
	
	response := map[string]interface{}{
		"content": content,
	}
	
	// Check if the tool response included an error flag
	if resultMap, ok := result.(map[string]interface{}); ok {
		if isError, ok := resultMap["isError"].(bool); ok && isError {
			response["isError"] = true
		}
	}
	
	return response, nil
}

func (h *MCPHandler) handlePing(params interface{}) (interface{}, error) {
	return map[string]interface{}{
		"message": "pong",
	}, nil
}

// handleSSERoutedRequest handles HTTP requests that route responses through SSE
func (h *MCPHandler) handleSSERoutedRequest(w http.ResponseWriter, r *http.Request, clientID string) {
	// Validate the SSE client exists
	h.sseMutex.RLock()
	requestChan, exists := h.sseRequests[clientID]
	h.sseMutex.RUnlock()
	
	if !exists {
		http.Error(w, "Invalid SSE client ID", http.StatusBadRequest)
		return
	}
	
	// Parse the request
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}
	
	var request JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	
	// Send request to SSE handler
	select {
	case requestChan <- &request:
		// Request queued successfully
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "accepted",
			"message": "Request queued for processing",
		})
	default:
		// Channel full
		http.Error(w, "Request queue full", http.StatusServiceUnavailable)
	}
}

// RegisterSSEClient registers a new SSE client for request routing
func (h *MCPHandler) RegisterSSEClient(clientID string) chan *JSONRPCRequest {
	h.sseMutex.Lock()
	defer h.sseMutex.Unlock()
	
	// Create a buffered channel for requests
	requestChan := make(chan *JSONRPCRequest, 10)
	h.sseRequests[clientID] = requestChan
	
	return requestChan
}

// UnregisterSSEClient removes an SSE client
func (h *MCPHandler) UnregisterSSEClient(clientID string) {
	h.sseMutex.Lock()
	defer h.sseMutex.Unlock()
	
	if ch, exists := h.sseRequests[clientID]; exists {
		close(ch)
		delete(h.sseRequests, clientID)
	}
}

// SendSSENotification sends a notification to a specific SSE client
func (h *MCPHandler) SendSSENotification(clientID string, method string, params interface{}) error {
	// Create a notification structure - this is not a standard JSONRPCResponse
	// but a custom structure for SSE notifications
	notification := map[string]interface{}{
		"jsonrpc": JSONRPCVersion,
		"method":  method,
		"params":  params,
	}
	
	// Wrap it in a JSONRPCResponse for the SSE transport
	response := &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Result:  notification,
		ID:      nil, // No ID for SSE messages
	}
	
	return h.sseManager.SendToClient(clientID, response)
}

// resourceCache provides thread-safe caching for MCP resources
type resourceCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	maxSize int
}

type cacheEntry struct {
	value     interface{}
	timestamp time.Time
	ttl       time.Duration
}

// newResourceCache creates a new resource cache
func newResourceCache(maxSize int) *resourceCache {
	return &resourceCache{
		data:    make(map[string]*cacheEntry),
		maxSize: maxSize,
	}
}

// get retrieves a value from the cache if it exists and hasn't expired
func (c *resourceCache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.data[key]
	if !exists {
		return nil, false
	}
	
	// Check if entry has expired
	if time.Since(entry.timestamp) > entry.ttl {
		return nil, false
	}
	
	return entry.value, true
}

// set stores a value in the cache with the given TTL
func (c *resourceCache) set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Implement simple LRU eviction if cache is full
	if len(c.data) >= c.maxSize && c.maxSize > 0 {
		// Find oldest entry
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.data {
			if oldestKey == "" || v.timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.timestamp
			}
		}
		delete(c.data, oldestKey)
	}
	
	c.data[key] = &cacheEntry{
		value:     value,
		timestamp: time.Now(),
		ttl:       ttl,
	}
}

// MCPMetrics tracks performance metrics for MCP operations
type MCPMetrics struct {
	mu               sync.RWMutex
	totalRequests    int64
	totalErrors      int64
	methodDurations  map[string]*durationStats
	toolExecutions   map[string]*executionStats
	resourceReads    map[string]*executionStats
	cacheHits        int64
	cacheMisses      int64
}

type durationStats struct {
	count    int64
	totalMs  int64
	minMs    int64
	maxMs    int64
}

type executionStats struct {
	count    int64
	errors   int64
	totalMs  int64
}

// newMCPMetrics creates a new metrics tracker
func newMCPMetrics() *MCPMetrics {
	return &MCPMetrics{
		methodDurations: make(map[string]*durationStats),
		toolExecutions:  make(map[string]*executionStats),
		resourceReads:   make(map[string]*executionStats),
	}
}

// recordRequest records a request metric
func (m *MCPMetrics) recordRequest(method string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.totalRequests++
	if err != nil {
		m.totalErrors++
	}
	
	durationMs := duration.Milliseconds()
	
	stats, exists := m.methodDurations[method]
	if !exists {
		stats = &durationStats{
			minMs: durationMs,
			maxMs: durationMs,
		}
		m.methodDurations[method] = stats
	}
	
	stats.count++
	stats.totalMs += durationMs
	if durationMs < stats.minMs {
		stats.minMs = durationMs
	}
	if durationMs > stats.maxMs {
		stats.maxMs = durationMs
	}
}

// recordToolExecution records a tool execution metric
func (m *MCPMetrics) recordToolExecution(toolName string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stats, exists := m.toolExecutions[toolName]
	if !exists {
		stats = &executionStats{}
		m.toolExecutions[toolName] = stats
	}
	
	stats.count++
	stats.totalMs += duration.Milliseconds()
	if err != nil {
		stats.errors++
	}
}

// recordResourceRead records a resource read metric
func (m *MCPMetrics) recordResourceRead(uri string, duration time.Duration, err error, cacheHit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if cacheHit {
		m.cacheHits++
		return
	}
	
	m.cacheMisses++
	
	stats, exists := m.resourceReads[uri]
	if !exists {
		stats = &executionStats{}
		m.resourceReads[uri] = stats
	}
	
	stats.count++
	stats.totalMs += duration.Milliseconds()
	if err != nil {
		stats.errors++
	}
}

// GetMetricsSummary returns a summary of collected metrics
func (m *MCPMetrics) GetMetricsSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Calculate method stats
	methodStats := make(map[string]interface{})
	for method, stats := range m.methodDurations {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		methodStats[method] = map[string]interface{}{
			"count":   stats.count,
			"avg_ms":  avgMs,
			"min_ms":  stats.minMs,
			"max_ms":  stats.maxMs,
		}
	}
	
	// Calculate tool stats
	toolStats := make(map[string]interface{})
	for tool, stats := range m.toolExecutions {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		toolStats[tool] = map[string]interface{}{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}
	
	// Calculate resource stats
	resourceStats := make(map[string]interface{})
	for uri, stats := range m.resourceReads {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		resourceStats[uri] = map[string]interface{}{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}
	
	// Calculate cache hit rate
	totalCacheRequests := m.cacheHits + m.cacheMisses
	cacheHitRate := float64(0)
	if totalCacheRequests > 0 {
		cacheHitRate = float64(m.cacheHits) / float64(totalCacheRequests)
	}
	
	return map[string]interface{}{
		"total_requests": m.totalRequests,
		"total_errors":   m.totalErrors,
		"error_rate":     float64(m.totalErrors) / float64(m.totalRequests),
		"methods":        methodStats,
		"tools":          toolStats,
		"resources":      resourceStats,
		"cache": map[string]interface{}{
			"hits":     m.cacheHits,
			"misses":   m.cacheMisses,
			"hit_rate": cacheHitRate,
		},
	}
}

// wrapToolWithContext wraps a regular MCPTool to support context
func wrapToolWithContext(tool MCPTool) MCPToolWithContext {
	// If it already supports context, return as-is
	if ctxTool, ok := tool.(MCPToolWithContext); ok {
		return ctxTool
	}
	
	// Otherwise, wrap it
	return &contextToolWrapper{tool: tool}
}

type contextToolWrapper struct {
	tool MCPTool
}

func (w *contextToolWrapper) Name() string {
	return w.tool.Name()
}

func (w *contextToolWrapper) Description() string {
	return w.tool.Description()
}

func (w *contextToolWrapper) Schema() map[string]interface{} {
	return w.tool.Schema()
}

func (w *contextToolWrapper) Execute(params map[string]interface{}) (interface{}, error) {
	return w.tool.Execute(params)
}

func (w *contextToolWrapper) ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Create a channel to receive the result
	type result struct {
		value interface{}
		err   error
	}
	
	resultChan := make(chan result, 1)
	
	// Run the tool in a goroutine
	go func() {
		value, err := w.tool.Execute(params)
		resultChan <- result{value: value, err: err}
	}()
	
	// Wait for either the result or context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.value, res.err
	}
}

// =============================================================================
// MCP Extensions - Helper types for building custom MCP functionality
// =============================================================================

// MCPExtension represents a collection of MCP tools and resources
// that can be registered as a group. This makes it easy for applications
// to package related functionality together.
type MCPExtension interface {
	// Name returns the extension name (e.g., "e-commerce", "blog", "analytics")
	Name() string
	
	// Description returns a human-readable description
	Description() string
	
	// Tools returns the tools provided by this extension
	Tools() []MCPTool
	
	// Resources returns the resources provided by this extension  
	Resources() []MCPResource
	
	// Configure is called with the server instance before registration
	// This allows the extension to store server reference if needed
	Configure(srv *Server) error
}

// MCPExtensionBuilder provides a fluent API for building MCP extensions
type MCPExtensionBuilder struct {
	name        string
	description string
	tools       []MCPTool
	resources   []MCPResource
	configFunc  func(*Server) error
}

// NewMCPExtension creates a new extension builder
func NewMCPExtension(name string) *MCPExtensionBuilder {
	return &MCPExtensionBuilder{
		name:      name,
		tools:     []MCPTool{},
		resources: []MCPResource{},
	}
}

func (b *MCPExtensionBuilder) WithDescription(desc string) *MCPExtensionBuilder {
	b.description = desc
	return b
}

func (b *MCPExtensionBuilder) WithTool(tool MCPTool) *MCPExtensionBuilder {
	b.tools = append(b.tools, tool)
	return b
}

func (b *MCPExtensionBuilder) WithResource(resource MCPResource) *MCPExtensionBuilder {
	b.resources = append(b.resources, resource)
	return b
}

func (b *MCPExtensionBuilder) WithConfiguration(fn func(*Server) error) *MCPExtensionBuilder {
	b.configFunc = fn
	return b
}

func (b *MCPExtensionBuilder) Build() MCPExtension {
	return &builtExtension{
		name:        b.name,
		description: b.description,
		tools:       b.tools,
		resources:   b.resources,
		configFunc:  b.configFunc,
	}
}

type builtExtension struct {
	name        string
	description string
	tools       []MCPTool
	resources   []MCPResource
	configFunc  func(*Server) error
}

func (e *builtExtension) Name() string        { return e.name }
func (e *builtExtension) Description() string { return e.description }
func (e *builtExtension) Tools() []MCPTool    { return e.tools }
func (e *builtExtension) Resources() []MCPResource { return e.resources }
func (e *builtExtension) Configure(srv *Server) error {
	if e.configFunc != nil {
		return e.configFunc(srv)
	}
	return nil
}

// RegisterMCPExtension registers all tools and resources from an extension
func (srv *Server) RegisterMCPExtension(ext MCPExtension) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}

	// Configure the extension with server reference
	if err := ext.Configure(srv); err != nil {
		return fmt.Errorf("extension configuration failed: %w", err)
	}

	// Register all tools
	for _, tool := range ext.Tools() {
		srv.mcpHandler.RegisterTool(tool)
	}

	// Register all resources  
	for _, resource := range ext.Resources() {
		srv.mcpHandler.RegisterResource(resource)
	}

	logger.Info("MCP extension registered",
		"name", ext.Name(),
		"tools", len(ext.Tools()),
		"resources", len(ext.Resources()),
	)

	return nil
}

// SimpleTool provides a simple way to create MCP tools without implementing the full interface
type SimpleTool struct {
	NameFunc        func() string
	DescriptionFunc func() string
	SchemaFunc      func() map[string]interface{}
	ExecuteFunc     func(map[string]interface{}) (interface{}, error)
}

func (t *SimpleTool) Name() string {
	if t.NameFunc != nil {
		return t.NameFunc()
	}
	return "unnamed_tool"
}

func (t *SimpleTool) Description() string {
	if t.DescriptionFunc != nil {
		return t.DescriptionFunc()
	}
	return "No description provided"
}

func (t *SimpleTool) Schema() map[string]interface{} {
	if t.SchemaFunc != nil {
		return t.SchemaFunc()
	}
	return map[string]interface{}{"type": "object"}
}

func (t *SimpleTool) Execute(params map[string]interface{}) (interface{}, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(params)
	}
	return nil, fmt.Errorf("execute function not implemented")
}

// SimpleResource provides a simple way to create MCP resources
type SimpleResource struct {
	URIFunc         func() string
	NameFunc        func() string
	DescriptionFunc func() string
	MimeTypeFunc    func() string
	ReadFunc        func() (interface{}, error)
	ListFunc        func() ([]string, error)
}

func (r *SimpleResource) URI() string {
	if r.URIFunc != nil {
		return r.URIFunc()
	}
	return "resource://unknown"
}

func (r *SimpleResource) Name() string {
	if r.NameFunc != nil {
		return r.NameFunc()
	}
	return "Unnamed Resource"
}

func (r *SimpleResource) Description() string {
	if r.DescriptionFunc != nil {
		return r.DescriptionFunc()
	}
	return "No description provided"
}

func (r *SimpleResource) MimeType() string {
	if r.MimeTypeFunc != nil {
		return r.MimeTypeFunc()
	}
	return "application/json"
}

func (r *SimpleResource) Read() (interface{}, error) {
	if r.ReadFunc != nil {
		return r.ReadFunc()
	}
	return nil, fmt.Errorf("read function not implemented")
}

func (r *SimpleResource) List() ([]string, error) {
	if r.ListFunc != nil {
		return r.ListFunc()
	}
	return []string{r.URI()}, nil
}

// ToolBuilder provides a fluent API for building tools
type ToolBuilder struct {
	name        string
	description string
	schema      map[string]interface{}
	executeFunc func(map[string]interface{}) (interface{}, error)
}

// NewTool creates a new tool builder
func NewTool(name string) *ToolBuilder {
	return &ToolBuilder{
		name:   name,
		schema: map[string]interface{}{"type": "object"},
	}
}

func (b *ToolBuilder) WithDescription(desc string) *ToolBuilder {
	b.description = desc
	return b
}

func (b *ToolBuilder) WithParameter(name, paramType, description string, required bool) *ToolBuilder {
	if b.schema["properties"] == nil {
		b.schema["properties"] = map[string]interface{}{}
	}
	props := b.schema["properties"].(map[string]interface{})
	props[name] = map[string]interface{}{
		"type":        paramType,
		"description": description,
	}
	
	if required {
		if b.schema["required"] == nil {
			b.schema["required"] = []string{}
		}
		b.schema["required"] = append(b.schema["required"].([]string), name)
	}
	
	return b
}

func (b *ToolBuilder) WithExecute(fn func(map[string]interface{}) (interface{}, error)) *ToolBuilder {
	b.executeFunc = fn
	return b
}

func (b *ToolBuilder) Build() MCPTool {
	return &SimpleTool{
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		SchemaFunc:      func() map[string]interface{} { return b.schema },
		ExecuteFunc:     b.executeFunc,
	}
}

// ResourceBuilder provides a fluent API for building resources
type ResourceBuilder struct {
	uri         string
	name        string
	description string
	mimeType    string
	readFunc    func() (interface{}, error)
}

// NewResource creates a new resource builder
func NewResource(uri string) *ResourceBuilder {
	return &ResourceBuilder{
		uri:      uri,
		mimeType: "application/json",
	}
}

func (b *ResourceBuilder) WithName(name string) *ResourceBuilder {
	b.name = name
	return b
}

func (b *ResourceBuilder) WithDescription(desc string) *ResourceBuilder {
	b.description = desc
	return b
}

func (b *ResourceBuilder) WithMimeType(mimeType string) *ResourceBuilder {
	b.mimeType = mimeType
	return b
}

func (b *ResourceBuilder) WithRead(fn func() (interface{}, error)) *ResourceBuilder {
	b.readFunc = fn
	return b
}

func (b *ResourceBuilder) Build() MCPResource {
	return &SimpleResource{
		URIFunc:         func() string { return b.uri },
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		MimeTypeFunc:    func() string { return b.mimeType },
		ReadFunc:        b.readFunc,
		ListFunc:        func() ([]string, error) { return []string{b.uri}, nil },
	}
}

// Example: Creating a custom e-commerce extension
func ExampleECommerceExtension() MCPExtension {
	return NewMCPExtension("ecommerce").
		WithDescription("E-commerce management tools and resources").
		WithTool(
			NewTool("manage_products").
				WithDescription("Add, update, or remove products").
				WithParameter("action", "string", "Action to perform", true).
				WithParameter("product_id", "string", "Product ID", false).
				WithParameter("data", "object", "Product data", false).
				WithExecute(func(params map[string]interface{}) (interface{}, error) {
					action := params["action"].(string)
					switch action {
					case "list":
						// Return product list
						return map[string]interface{}{"products": []interface{}{}}, nil
					case "add":
						// Add product logic
						return map[string]interface{}{"status": "product_added"}, nil
					default:
						return nil, fmt.Errorf("unknown action: %s", action)
					}
				}).
				Build(),
		).
		WithResource(
			NewResource("products://catalog/all").
				WithName("Product Catalog").
				WithDescription("All products in the catalog").
				WithRead(func() (interface{}, error) {
					// Fetch from database
					return map[string]interface{}{
						"products": []interface{}{
							// Product data
						},
					}, nil
				}).
				Build(),
		).
		WithConfiguration(func(s *Server) error {
			// Store server reference, initialize database, etc.
			return nil
		}).
		Build()
}