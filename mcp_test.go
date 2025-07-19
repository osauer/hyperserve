package hyperserve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPHandler_NewMCPHandler(t *testing.T) {
	serverInfo := MCPServerInfo{
		Name:    "test-server",
		Version: "1.0.0",
	}
	
	handler := NewMCPHandler(serverInfo)
	if handler == nil {
		t.Fatal("NewMCPHandler returned nil")
	}
	
	if handler.serverInfo.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got %s", handler.serverInfo.Name)
	}
	
	if handler.serverInfo.Version != "1.0.0" {
		t.Errorf("Expected server version '1.0.0', got %s", handler.serverInfo.Version)
	}
	
	if handler.tools == nil {
		t.Error("Tools map is nil")
	}
	
	if handler.resources == nil {
		t.Error("Resources map is nil")
	}
	
	if handler.rpcEngine == nil {
		t.Error("RPC engine is nil")
	}
}

func TestMCPHandler_RegisterTool(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	tool := NewCalculatorTool()
	handler.RegisterTool(tool)
	
	if len(handler.tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(handler.tools))
	}
	
	if _, exists := handler.tools[tool.Name()]; !exists {
		t.Error("Tool not found in handler tools map")
	}
}

func TestMCPHandler_RegisterResource(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	resource := NewSystemResource()
	handler.RegisterResource(resource)
	
	if len(handler.resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(handler.resources))
	}
	
	if _, exists := handler.resources[resource.URI()]; !exists {
		t.Error("Resource not found in handler resources map")
	}
}

func TestMCPHandler_Initialize(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test-server", Version: "1.0.0"})
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": MCPVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
		"id": 1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	if result["protocolVersion"] != MCPVersion {
		t.Errorf("Expected protocol version %s, got %v", MCPVersion, result["protocolVersion"])
	}
	
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("serverInfo not found or not a map")
	}
	
	if serverInfo["name"] != "test-server" {
		t.Errorf("Expected server name 'test-server', got %v", serverInfo["name"])
	}
}

func TestMCPHandler_ToolsList(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register a test tool
	calculator := NewCalculatorTool()
	handler.RegisterTool(calculator)
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("tools not found or not a slice")
	}
	
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	
	tool := tools[0].(map[string]interface{})
	if tool["name"] != calculator.Name() {
		t.Errorf("Expected tool name %s, got %v", calculator.Name(), tool["name"])
	}
}

func TestMCPHandler_ToolsCall(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register calculator tool
	calculator := NewCalculatorTool()
	handler.RegisterTool(calculator)
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "calculator",
			"arguments": map[string]interface{}{
				"operation": "add",
				"a":         5.0,
				"b":         3.0,
			},
		},
		"id": 1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	content, ok := result["content"].([]interface{})
	if !ok {
		t.Fatal("content not found or not a slice")
	}
	
	if len(content) == 0 {
		t.Fatal("Expected at least one content item")
	}
}

func TestMCPHandler_ResourcesList(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register a test resource
	systemResource := NewSystemResource()
	handler.RegisterResource(systemResource)
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "resources/list",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	resources, ok := result["resources"].([]interface{})
	if !ok {
		t.Fatal("resources not found or not a slice")
	}
	
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}
	
	resource := resources[0].(map[string]interface{})
	if resource["uri"] != systemResource.URI() {
		t.Errorf("Expected resource URI %s, got %v", systemResource.URI(), resource["uri"])
	}
}

func TestMCPHandler_ResourcesRead(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register a test resource
	systemResource := NewSystemResource()
	handler.RegisterResource(systemResource)
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "resources/read",
		"params": map[string]interface{}{
			"uri": systemResource.URI(),
		},
		"id": 1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("contents not found or not a slice")
	}
	
	if len(contents) == 0 {
		t.Fatal("Expected at least one content item")
	}
	
	content := contents[0].(map[string]interface{})
	if content["uri"] != systemResource.URI() {
		t.Errorf("Expected content URI %s, got %v", systemResource.URI(), content["uri"])
	}
}

func TestMCPHandler_ResourcesRead_Validation(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register a test resource
	systemResource := NewSystemResource()
	handler.RegisterResource(systemResource)
	
	testCases := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty params",
			params:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "uri parameter is required",
		},
		{
			name:        "empty uri",
			params:      map[string]interface{}{"uri": ""},
			expectError: true,
			errorMsg:    "uri parameter is required",
		},
		{
			name:        "arguments param instead of uri",
			params:      map[string]interface{}{"arguments": map[string]interface{}{}},
			expectError: true,
			errorMsg:    "expects 'uri' parameter, not 'arguments'",
		},
		{
			name:        "valid uri",
			params:      map[string]interface{}{"uri": systemResource.URI()},
			expectError: false,
			errorMsg:    "",
		},
		{
			name:        "nonexistent resource",
			params:      map[string]interface{}{"uri": "nonexistent://resource"},
			expectError: true,
			errorMsg:    "resource not found",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params":  tc.params,
				"id":      1,
			}
			
			requestData, _ := json.Marshal(request)
			responseData := handler.ProcessRequest(requestData)
			
			var response JSONRPCResponse
			if err := json.Unmarshal(responseData, &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}
			
			if tc.expectError {
				if response.Error == nil {
					t.Errorf("Expected error but got none")
				} else {
					// Check both the message and data fields for the error text
					errorText := response.Error.Message
					if response.Error.Data != nil {
						if dataStr, ok := response.Error.Data.(string); ok {
							errorText = dataStr
						}
					}
					if !strings.Contains(errorText, tc.errorMsg) {
						t.Errorf("Expected error to contain '%s', got '%s'", tc.errorMsg, errorText)
					}
				}
			} else {
				if response.Error != nil {
					t.Errorf("Expected no error, got %+v", response.Error)
				}
			}
		})
	}
}

func TestMCPHandler_ResourcesRead_InvalidParams(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	testCases := []struct {
		name        string
		params      interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "string params",
			params:      "invalid",
			expectError: true,
			errorMsg:    "failed to unmarshal",
		},
		{
			name:        "number params",
			params:      123,
			expectError: true,
			errorMsg:    "failed to unmarshal",
		},
		{
			name:        "nil params",
			params:      nil,
			expectError: true,
			errorMsg:    "uri parameter is required",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params":  tc.params,
				"id":      1,
			}
			
			requestData, _ := json.Marshal(request)
			responseData := handler.ProcessRequest(requestData)
			
			var response JSONRPCResponse
			if err := json.Unmarshal(responseData, &response); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}
			
			if tc.expectError {
				if response.Error == nil {
					t.Errorf("Expected error but got none")
				} else {
					// Check both the message and data fields for the error text
					errorText := response.Error.Message
					if response.Error.Data != nil {
						if dataStr, ok := response.Error.Data.(string); ok {
							errorText = dataStr
						}
					}
					if !strings.Contains(errorText, tc.errorMsg) {
						t.Errorf("Expected error to contain '%s', got '%s'", tc.errorMsg, errorText)
					}
				}
			} else {
				if response.Error != nil {
					t.Errorf("Expected no error, got %+v", response.Error)
				}
			}
		})
	}
}

func TestMCPHandler_Ping(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	responseData := handler.ProcessRequest(requestData)
	
	var response JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}
	
	if result["message"] != "pong" {
		t.Errorf("Expected message 'pong', got %v", result["message"])
	}
}

func TestMCPHandler_ServeHTTP(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Test valid POST request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      1,
	}
	
	requestData, _ := json.Marshal(request)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	var response JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
}

func TestMCPHandler_ServeHTTP_MethodNotAllowed(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	
	handler.ServeHTTP(w, req)
	
	// GET requests now return helpful HTML documentation
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for GET request, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type text/html; charset=utf-8, got %s", contentType)
	}
}

func TestMCPHandler_ServeHTTP_InvalidContentType(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "text/plain")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestMCPNamespace_Creation(t *testing.T) {
	tool := NewCalculatorTool()
	resource := NewSystemResource()
	
	namespace := NewMCPNamespace("test", 
		WithNamespaceTools(tool),
		WithNamespaceResources(resource))
	
	if namespace.Name() != "test" {
		t.Errorf("Expected namespace name 'test', got %s", namespace.Name())
	}
	
	if len(namespace.Tools()) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(namespace.Tools()))
	}
	
	if len(namespace.Resources()) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(namespace.Resources()))
	}
}

func TestMCPHandler_RegisterNamespace(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	tool := NewCalculatorTool()
	resource := NewSystemResource()
	
	namespace := NewMCPNamespace("analytics", 
		WithNamespaceTools(tool),
		WithNamespaceResources(resource))
	
	// Register the namespace
	handler.RegisterNamespace(namespace)
	
	// Check that tools are registered with prefixed names
	prefixedToolName := "mcp__analytics__calculator"
	if _, exists := handler.tools[prefixedToolName]; !exists {
		t.Errorf("Expected tool %s to be registered", prefixedToolName)
	}
	
	// Check that resources are registered with prefixed URIs
	prefixedResourceURI := "mcp__analytics__system://runtime/info"
	if _, exists := handler.resources[prefixedResourceURI]; !exists {
		t.Errorf("Expected resource %s to be registered", prefixedResourceURI)
	}
	
	// Check that namespace is stored
	if _, exists := handler.namespaces["analytics"]; !exists {
		t.Error("Expected namespace 'analytics' to be stored")
	}
}

func TestMCPHandler_RegisterToolInNamespace(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	tool := NewCalculatorTool()
	handler.RegisterToolInNamespace(tool, "math")
	
	// Check that tool is registered with prefixed name
	prefixedToolName := "mcp__math__calculator"
	if _, exists := handler.tools[prefixedToolName]; !exists {
		t.Errorf("Expected tool %s to be registered", prefixedToolName)
	}
}

func TestMCPHandler_RegisterResourceInNamespace(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	resource := NewSystemResource()
	handler.RegisterResourceInNamespace(resource, "system")
	
	// Check that resource is registered with prefixed URI
	prefixedResourceURI := "mcp__system__system://runtime/info"
	if _, exists := handler.resources[prefixedResourceURI]; !exists {
		t.Errorf("Expected resource %s to be registered", prefixedResourceURI)
	}
	
	// Test that the wrapped resource works correctly
	wrappedResource := handler.resources[prefixedResourceURI]
	if wrappedResource.URI() != prefixedResourceURI {
		t.Errorf("Expected wrapped resource URI %s, got %s", prefixedResourceURI, wrappedResource.URI())
	}
	
	// Test that the original functionality is preserved
	data, err := wrappedResource.Read()
	if err != nil {
		t.Errorf("Error reading wrapped resource: %v", err)
	}
	
	// SystemResource returns a struct, not a string - just verify it's not nil
	if data == nil {
		t.Error("Expected wrapped resource data to not be nil")
	}
}

func TestNamespacedTool_BackwardCompatibility(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})
	
	// Register a tool the old way (no namespace)
	tool := NewCalculatorTool()
	handler.RegisterTool(tool)
	
	// Register a tool with namespace
	handler.RegisterToolInNamespace(tool, "math")
	
	// Both should be available
	if _, exists := handler.tools["calculator"]; !exists {
		t.Error("Expected original tool name 'calculator' to be available")
	}
	
	if _, exists := handler.tools["mcp__math__calculator"]; !exists {
		t.Error("Expected namespaced tool name 'mcp__math__calculator' to be available")
	}
	
	// Should have 2 tools total
	if len(handler.tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(handler.tools))
	}
}