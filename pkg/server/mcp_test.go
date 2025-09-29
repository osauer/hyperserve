package server

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

func TestMCPHandler_MultipleNamespaces(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "testserver", Version: "1.0"})

	// Create test tools for different namespaces
	calcTool := NewCalculatorTool()
	httpTool := NewHTTPRequestTool()

	// Register tools in different namespaces
	handler.RegisterToolInNamespace(calcTool, "math")
	handler.RegisterToolInNamespace(httpTool, "web")

	// Register a tool in the default namespace (backward compatibility)
	defaultTool := NewCalculatorTool()
	handler.RegisterTool(defaultTool)

	// Test that tools are registered with appropriate names
	expectedTools := []string{
		"mcp__math__calculator",  // namespace-specific tool
		"mcp__web__http_request", // namespace-specific tool
		"calculator",             // backward compatible tool (no prefix)
	}

	if len(handler.tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(handler.tools))
	}

	for _, expectedTool := range expectedTools {
		if _, exists := handler.tools[expectedTool]; !exists {
			t.Errorf("Expected tool %s not found", expectedTool)
		}
	}

	// Test tools/list returns prefixed names
	listRequest := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      1,
	}

	response := handler.rpcEngine.ProcessRequestDirect(listRequest)
	if response.Error != nil {
		t.Fatalf("tools/list failed: %v", response.Error)
	}

	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}

	tools, ok := result["tools"].([]map[string]interface{})
	if !ok {
		t.Fatal("tools not found or not a slice")
	}

	if len(tools) != 3 {
		t.Errorf("Expected 3 tools in list, got %d", len(tools))
	}

	// Verify all tools have prefixed names
	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool["name"].(string)
	}

	for _, expectedTool := range expectedTools {
		found := false
		for _, toolName := range toolNames {
			if toolName == expectedTool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected tool %s not found in tools/list response", expectedTool)
		}
	}

	// Test that tools can be called with their registered names
	callRequest := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "mcp__math__calculator", // Use the namespace-prefixed tool
			"arguments": map[string]interface{}{
				"operation": "add",
				"a":         5.0,
				"b":         3.0,
			},
		},
		ID: 2,
	}

	response = handler.rpcEngine.ProcessRequestDirect(callRequest)
	if response.Error != nil {
		t.Errorf("tools/call failed: %v", response.Error)
	}

	// The result should contain the calculation result
	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result to be a map, got %T", response.Result)
	}

	if resultMap["content"] == nil {
		t.Error("Expected content field in tool call response")
	}
}

func TestMCPNamespace_RegisterNamespace(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "testserver", Version: "1.0"})

	// Create test tools
	calc1 := NewCalculatorTool()
	calc2 := NewCalculatorTool()

	// Register a namespace with multiple tools
	err := handler.RegisterNamespace("analytics",
		WithNamespaceTools(calc1, calc2),
	)
	if err != nil {
		t.Fatalf("Failed to register namespace: %v", err)
	}

	// Verify namespace was registered
	if _, exists := handler.namespaces["analytics"]; !exists {
		t.Error("Expected namespace 'analytics' to be registered")
	}

	// Verify tools are registered with prefixed names
	// Note: Both calc tools have the same name, so second overwrites first

	// Note: Since both calc tools have the same name, the second one overwrites the first
	// This is expected behavior
	toolCount := 0
	for toolName := range handler.tools {
		if strings.HasPrefix(toolName, "mcp__analytics__") {
			toolCount++
		}
	}

	if toolCount != 1 { // Only one calculator should remain (second overwrites first)
		t.Errorf("Expected 1 analytics tool, got %d", toolCount)
	}
}

func TestMCPNamespace_EmptyNamespace(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "testserver", Version: "1.0"})

	// Try to register namespace with empty name
	err := handler.RegisterNamespace("", WithNamespaceTools())
	if err == nil {
		t.Error("Expected error when registering namespace with empty name")
	}

	if !strings.Contains(err.Error(), "namespace name cannot be empty") {
		t.Errorf("Expected 'namespace name cannot be empty' error, got: %v", err)
	}
}

func TestMCPHandler_ServeHTTP_AcceptJSON(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	tests := []struct {
		name   string
		accept string
	}{
		{"application/json", "application/json"},
		{"wildcard", "*/*"},
		{"with quality", "application/json;q=0.8"},
		{"multiple types", "text/html,application/json"},
		{"wildcard with quality", "*/*;q=0.8"},
		{"application wildcard", "application/*"},
		{"json with charset", "application/json; charset=utf-8"},
		{"case insensitive", "Application/JSON"},
		{"with spaces", " application/json "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			req.Header.Set("Accept", tt.accept)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Verify JSON structure
			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Errorf("Failed to unmarshal JSON response: %v", err)
			}

			// Verify required fields
			if status := response["status"]; status != "ready" {
				t.Errorf("Expected status 'ready', got %v", status)
			}

			if _, ok := response["capabilities"]; !ok {
				t.Error("Expected capabilities in response")
			}

			if _, ok := response["server"]; !ok {
				t.Error("Expected server in response")
			}

			if endpoint := response["endpoint"]; endpoint != "/mcp" {
				t.Errorf("Expected endpoint '/mcp', got %v", endpoint)
			}

			if transport := response["transport"]; transport != "http" {
				t.Errorf("Expected transport 'http', got %v", transport)
			}
		})
	}
}

func TestMCPHandler_ServeHTTP_AcceptHTML(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	tests := []struct {
		name   string
		accept string
	}{
		{"text/html", "text/html"},
		{"empty accept", ""},
		{"text plain", "text/plain"},
		{"no json preference", "text/html,text/plain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "text/html; charset=utf-8" {
				t.Errorf("Expected Content-Type text/html; charset=utf-8, got %s", contentType)
			}

			// Verify HTML content
			body := w.Body.String()
			if !strings.Contains(body, "<!DOCTYPE html>") {
				t.Error("Expected HTML DOCTYPE in response")
			}

			if !strings.Contains(body, "Model Context Protocol") {
				t.Error("Expected MCP title in HTML response")
			}
		})
	}
}

func TestMCPHandler_GetCapabilities_Consistency(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	// Get capabilities from GET request
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var getResponse map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &getResponse)
	getCaps := getResponse["capabilities"]

	// Get capabilities from initialize
	initResult, _ := handler.handleInitialize(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
	})
	initResponse := initResult.(map[string]interface{})
	initCaps := initResponse["capabilities"]

	// Both should be the same when marshaled to JSON
	// This handles the fact that the types might be different (struct vs map)
	// but the JSON representation should be identical
	getCapsJSON, err1 := json.Marshal(getCaps)
	initCapsJSON, err2 := json.Marshal(initCaps)

	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to marshal capabilities: GET error=%v, INIT error=%v", err1, err2)
	}

	// Parse both JSON back to maps for comparison (to normalize field order)
	var getCapsParsed, initCapsParsed map[string]interface{}
	json.Unmarshal(getCapsJSON, &getCapsParsed)
	json.Unmarshal(initCapsJSON, &initCapsParsed)

	// Now marshal again to get consistent field order
	getCapsNormalized, _ := json.Marshal(getCapsParsed)
	initCapsNormalized, _ := json.Marshal(initCapsParsed)

	if string(getCapsNormalized) != string(initCapsNormalized) {
		t.Errorf("Capabilities should be identical between GET and initialize responses")
		t.Errorf("GET capabilities: %s", string(getCapsNormalized))
		t.Errorf("INIT capabilities: %s", string(initCapsNormalized))
	}
}

func TestMCPHandler_AcceptHeader_EdgeCases(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	tests := []struct {
		name        string
		accept      string
		expectJSON  bool
		description string
	}{
		{
			"json with complex parameters",
			"application/json; charset=utf-8; boundary=something",
			true,
			"Should handle JSON with multiple parameters",
		},
		{
			"wildcard with parameters",
			"*/*; q=0.8, text/html; q=0.9",
			true,
			"Should handle wildcard with quality values",
		},
		{
			"json in complex accept header",
			"text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.8,*/*;q=0.7",
			true,
			"Should find JSON in complex Accept header",
		},
		{
			"no json types",
			"text/html,text/plain,image/png",
			false,
			"Should default to HTML when no JSON types present",
		},
		{
			"application wildcard",
			"application/*,text/html;q=0.9",
			true,
			"Should match application/* for JSON",
		},
		{
			"case variations",
			"APPLICATION/JSON",
			true,
			"Should handle case variations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			req.Header.Set("Accept", tt.accept)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}

			contentType := w.Header().Get("Content-Type")

			if tt.expectJSON {
				if contentType != "application/json" {
					t.Errorf("%s: Expected JSON response, got %s", tt.description, contentType)
				}
			} else {
				if contentType != "text/html; charset=utf-8" {
					t.Errorf("%s: Expected HTML response, got %s", tt.description, contentType)
				}
			}
		})
	}
}
