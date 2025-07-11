package hyperserve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
	transport MCPTransportType
	endpoint  string
}

// MCP Tool interface defines the contract for MCP tools
type MCPTool interface {
	Name() string
	Description() string
	Schema() map[string]interface{}
	Execute(params map[string]interface{}) (interface{}, error)
}

// MCP Resource interface defines the contract for MCP resources
type MCPResource interface {
	URI() string
	Name() string
	Description() string
	MimeType() string
	Read() (interface{}, error)
	List() ([]string, error)
}

// MCPCapabilities represents the server's MCP capabilities
type MCPCapabilities struct {
	Experimental   map[string]interface{} `json:"experimental,omitempty"`
	Logging        *LoggingCapability     `json:"logging,omitempty"`
	Prompts        *PromptsCapability     `json:"prompts,omitempty"`
	Resources      *ResourcesCapability   `json:"resources,omitempty"`
	Tools          *ToolsCapability       `json:"tools,omitempty"`
	Sampling       *SamplingCapability    `json:"sampling,omitempty"`
}

// Individual capability structs
type LoggingCapability struct{}
type PromptsCapability struct{}
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}
type SamplingCapability struct{}

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

// MCPHandler manages MCP protocol communication
type MCPHandler struct {
	tools      map[string]MCPTool
	resources  map[string]MCPResource
	rpcEngine  *JSONRPCEngine
	serverInfo MCPServerInfo
	logger     *slog.Logger
	transport  MCPTransport
	metrics    *MCPMetrics
	cache      *resourceCache
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
		tools:      make(map[string]MCPTool),
		resources:  make(map[string]MCPResource),
		rpcEngine:  NewJSONRPCEngine(),
		serverInfo: serverInfo,
		logger:     logger,
		metrics:    newMCPMetrics(),
		cache:      newResourceCache(100), // Default cache size of 100 items
	}
	
	// Register MCP protocol methods
	handler.registerMCPMethods()
	
	return handler
}

// RegisterTool registers an MCP tool
func (h *MCPHandler) RegisterTool(tool MCPTool) {
	h.tools[tool.Name()] = tool
	h.logger.Info("MCP tool registered", "tool", tool.Name())
}

// RegisterResource registers an MCP resource
func (h *MCPHandler) RegisterResource(resource MCPResource) {
	h.resources[resource.URI()] = resource
	h.logger.Info("MCP resource registered", "resource", resource.Name(), "uri", resource.URI())
}

// GetMetrics returns the current MCP metrics summary
func (h *MCPHandler) GetMetrics() map[string]interface{} {
	if h.metrics == nil {
		return nil
	}
	return h.metrics.GetMetricsSummary()
}

// ProcessRequest processes an MCP request
func (h *MCPHandler) ProcessRequest(requestData []byte) []byte {
	return h.rpcEngine.ProcessRequest(requestData)
}

// ServeHTTP implements the http.Handler interface for MCP
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create HTTP transport for this request
	transport := newHTTPTransport(w, r)
	defer transport.Close()
	
	// Process the request using the transport
	if err := h.ProcessRequestWithTransport(transport); err != nil {
		h.logger.Error("Failed to process MCP request", "error", err)
		if err.Error() == "method not allowed: "+r.Method {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
	
	h.logger.Info("MCP client initialized", "client", initParams.ClientInfo.Name, "version", initParams.ClientInfo.Version)
	
	// Return server capabilities
	return map[string]interface{}{
		"protocolVersion": MCPVersion,
		"capabilities": MCPCapabilities{
			Resources: &ResourcesCapability{
				Subscribe:   false,
				ListChanged: false,
			},
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		"serverInfo": h.serverInfo,
	}, nil
}

func (h *MCPHandler) handleInitialized(params interface{}) (interface{}, error) {
	// The initialized notification doesn't require a response
	h.logger.Info("MCP client confirmed initialization")
	return nil, nil
}

func (h *MCPHandler) handleResourcesList(params interface{}) (interface{}, error) {
	resources := make([]map[string]interface{}, 0, len(h.resources))
	
	for _, resource := range h.resources {
		resources = append(resources, map[string]interface{}{
			"uri":         resource.URI(),
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
		
		if err := json.Unmarshal(paramBytes, &readParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal read params: %w", err)
		}
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
	
	// Cache the result (with 5 minute TTL for now)
	h.cache.set(cacheKey, content, 5*time.Minute)
	
	return map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"uri":      resource.URI(),
				"mimeType": resource.MimeType(),
				"text":     content,
			},
		},
	}, nil
}

func (h *MCPHandler) handleToolsList(params interface{}) (interface{}, error) {
	tools := make([]map[string]interface{}, 0, len(h.tools))
	
	for _, tool := range h.tools {
		tools = append(tools, map[string]interface{}{
			"name":        tool.Name(),
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
	
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": result,
			},
		},
	}, nil
}

func (h *MCPHandler) handlePing(params interface{}) (interface{}, error) {
	return map[string]interface{}{
		"message": "pong",
	}, nil
}