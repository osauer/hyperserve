package hyperserve

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// MCP Protocol constants
const (
	MCPVersion = "2024-11-05"
)

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

// MCPHandler manages MCP protocol communication
type MCPHandler struct {
	tools      map[string]MCPTool
	resources  map[string]MCPResource
	rpcEngine  *JSONRPCEngine
	serverInfo MCPServerInfo
	logger     *slog.Logger
}

// NewMCPHandler creates a new MCP handler instance
func NewMCPHandler(serverInfo MCPServerInfo) *MCPHandler {
	handler := &MCPHandler{
		tools:      make(map[string]MCPTool),
		resources:  make(map[string]MCPResource),
		rpcEngine:  NewJSONRPCEngine(),
		serverInfo: serverInfo,
		logger:     logger,
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

// ProcessRequest processes an MCP request
func (h *MCPHandler) ProcessRequest(requestData []byte) []byte {
	return h.rpcEngine.ProcessRequest(requestData)
}

// ServeHTTP implements the http.Handler interface for MCP
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}
	
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	
	// Process MCP request
	response := h.ProcessRequest(body)
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(response); err != nil {
		h.logger.Error("Failed to write response", "error", err)
	}
}

// registerMCPMethods registers all MCP protocol methods with the JSON-RPC engine
func (h *MCPHandler) registerMCPMethods() {
	// Initialize method
	h.rpcEngine.RegisterMethod("initialize", h.handleInitialize)
	
	// Resource methods
	h.rpcEngine.RegisterMethod("resources/list", h.handleResourcesList)
	h.rpcEngine.RegisterMethod("resources/read", h.handleResourcesRead)
	
	// Tool methods
	h.rpcEngine.RegisterMethod("tools/list", h.handleToolsList)
	h.rpcEngine.RegisterMethod("tools/call", h.handleToolsCall)
	
	// Utility methods
	h.rpcEngine.RegisterMethod("ping", h.handlePing)
}

// MCP method handlers

func (h *MCPHandler) handleInitialize(params interface{}) (interface{}, error) {
	var initParams struct {
		ProtocolVersion string        `json:"protocolVersion"`
		Capabilities    interface{}   `json:"capabilities"`
		ClientInfo      MCPClientInfo `json:"clientInfo"`
	}
	
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
	var readParams struct {
		URI string `json:"uri"`
	}
	
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
	
	content, err := resource.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read resource: %w", err)
	}
	
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
	var callParams struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	
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
	
	result, err := tool.Execute(callParams.Arguments)
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