package hyperserve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPIntegration_ServerCreation(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"), // Use port 0 for testing
		WithMCPSupport(),
		WithMCPEndpoint("/mcp"),
		WithMCPServerInfo("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server with MCP: %v", err)
	}
	
	if !srv.Options.MCPEnabled {
		t.Error("MCP should be enabled")
	}
	
	if srv.Options.MCPEndpoint != "/mcp" {
		t.Errorf("Expected MCP endpoint '/mcp', got %s", srv.Options.MCPEndpoint)
	}
	
	if srv.Options.MCPServerName != "test-server" {
		t.Errorf("Expected server name 'test-server', got %s", srv.Options.MCPServerName)
	}
	
	if srv.mcpHandler == nil {
		t.Error("MCP handler should be initialized")
	}
}

func TestMCPIntegration_FullWorkflow(t *testing.T) {
	// Create temporary directory for file tools testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_mcp_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello from MCP integration test!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Create server with MCP enabled
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPEndpoint("/mcp"),
		WithMCPServerInfo("integration-test-server", "1.0.0"),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	
	// Test 1: Initialize MCP connection
	t.Run("Initialize", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": MCPVersion,
				"capabilities":    map[string]interface{}{},
				"clientInfo": map[string]interface{}{
					"name":    "integration-test-client",
					"version": "1.0.0",
				},
			},
			"id": 1,
		}
		
		response := makeRequest(t, srv, request)
		validateInitializeResponse(t, response)
	})
	
	// Test 2: List available tools
	t.Run("ListTools", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      2,
		}
		
		response := makeRequest(t, srv, request)
		validateToolsListResponse(t, response)
	})
	
	// Test 3: Call calculator tool
	t.Run("CallCalculator", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "calculator",
				"arguments": map[string]interface{}{
					"operation": "multiply",
					"a":         7.0,
					"b":         6.0,
				},
			},
			"id": 3,
		}
		
		response := makeRequest(t, srv, request)
		validateCalculatorResponse(t, response, 42.0)
	})
	
	// Test 4: Call file read tool
	t.Run("CallFileRead", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "read_file",
				"arguments": map[string]interface{}{
					"path": "test.txt",
				},
			},
			"id": 4,
		}
		
		response := makeRequest(t, srv, request)
		validateFileReadResponse(t, response, testContent)
	})
	
	// Test 5: List available resources
	t.Run("ListResources", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "resources/list",
			"id":      5,
		}
		
		response := makeRequest(t, srv, request)
		validateResourcesListResponse(t, response)
	})
	
	// Test 6: Read system resource
	t.Run("ReadSystemResource", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "resources/read",
			"params": map[string]interface{}{
				"uri": "system://runtime/info",
			},
			"id": 6,
		}
		
		response := makeRequest(t, srv, request)
		validateSystemResourceResponse(t, response)
	})
	
	// Test 7: Ping
	t.Run("Ping", func(t *testing.T) {
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "ping",
			"id":      7,
		}
		
		response := makeRequest(t, srv, request)
		validatePingResponse(t, response)
	})
}

func TestMCPIntegration_DisabledFeatures(t *testing.T) {
	// Create server with tools disabled
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPToolsDisabled(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	
	if srv.Options.MCPToolsEnabled {
		t.Error("Tools should be disabled")
	}
	
	// Test that tools list is empty
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}
	
	response := makeRequest(t, srv, request)
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})
	
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools when disabled, got %d", len(tools))
	}
}

func TestMCPIntegration_ErrorHandling(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	
	// Test calling non-existent tool
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "nonexistent_tool",
			"arguments": map[string]interface{}{},
		},
		"id": 1,
	}
	
	response := makeRequest(t, srv, request)
	
	if response.Error == nil {
		t.Error("Expected error for non-existent tool")
	}
	
	// Test reading non-existent resource
	request = map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "resources/read",
		"params": map[string]interface{}{
			"uri": "nonexistent://resource",
		},
		"id": 2,
	}
	
	response = makeRequest(t, srv, request)
	
	if response.Error == nil {
		t.Error("Expected error for non-existent resource")
	}
}

// Helper functions

func makeRequest(t *testing.T, srv *Server, request map[string]interface{}) JSONRPCResponse {
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	srv.mcpHandler.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	
	var response JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	return response
}

func validateInitializeResponse(t *testing.T, response JSONRPCResponse) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	
	if result["protocolVersion"] != MCPVersion {
		t.Errorf("Expected protocol version %s, got %v", MCPVersion, result["protocolVersion"])
	}
	
	serverInfo := result["serverInfo"].(map[string]interface{})
	if serverInfo["name"] != "integration-test-server" {
		t.Errorf("Expected server name 'integration-test-server', got %v", serverInfo["name"])
	}
}

func validateToolsListResponse(t *testing.T, response JSONRPCResponse) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	tools := result["tools"].([]interface{})
	
	if len(tools) < 4 {
		t.Errorf("Expected at least 4 tools, got %d", len(tools))
	}
	
	// Check for expected tools
	foundCalculator := false
	foundFileRead := false
	foundListDir := false
	foundHTTP := false
	
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		name := toolMap["name"].(string)
		
		switch name {
		case "calculator":
			foundCalculator = true
		case "read_file":
			foundFileRead = true
		case "list_directory":
			foundListDir = true
		case "http_request":
			foundHTTP = true
		}
	}
	
	if !foundCalculator || !foundFileRead || !foundListDir || !foundHTTP {
		t.Error("Not all expected tools found")
	}
}

func validateCalculatorResponse(t *testing.T, response JSONRPCResponse, expectedResult float64) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	content := result["content"].([]interface{})
	
	if len(content) == 0 {
		t.Fatal("Expected at least one content item")
	}
	
	contentItem := content[0].(map[string]interface{})
	textResult := contentItem["text"].(map[string]interface{})
	calculatorResult := textResult["result"].(float64)
	
	if calculatorResult != expectedResult {
		t.Errorf("Expected calculator result %f, got %f", expectedResult, calculatorResult)
	}
}

func validateFileReadResponse(t *testing.T, response JSONRPCResponse, expectedContent string) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	content := result["content"].([]interface{})
	
	if len(content) == 0 {
		t.Fatal("Expected at least one content item")
	}
	
	contentItem := content[0].(map[string]interface{})
	textResult := contentItem["text"].(string)
	
	if textResult != expectedContent {
		t.Errorf("Expected file content '%s', got '%s'", expectedContent, textResult)
	}
}

func validateResourcesListResponse(t *testing.T, response JSONRPCResponse) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	resources := result["resources"].([]interface{})
	
	if len(resources) < 4 {
		t.Errorf("Expected at least 4 resources, got %d", len(resources))
	}
	
	// Check for expected resources
	foundConfig := false
	foundMetrics := false
	foundSystem := false
	foundLogs := false
	
	for _, resource := range resources {
		resourceMap := resource.(map[string]interface{})
		uri := resourceMap["uri"].(string)
		
		switch uri {
		case "config://server/options":
			foundConfig = true
		case "metrics://server/stats":
			foundMetrics = true
		case "system://runtime/info":
			foundSystem = true
		case "logs://server/recent":
			foundLogs = true
		}
	}
	
	if !foundConfig || !foundMetrics || !foundSystem || !foundLogs {
		t.Error("Not all expected resources found")
	}
}

func validateSystemResourceResponse(t *testing.T, response JSONRPCResponse) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	contents := result["contents"].([]interface{})
	
	if len(contents) == 0 {
		t.Fatal("Expected at least one content item")
	}
	
	contentItem := contents[0].(map[string]interface{})
	text := contentItem["text"].(string)
	
	// Verify it's valid JSON with expected fields
	var systemInfo map[string]interface{}
	if err := json.Unmarshal([]byte(text), &systemInfo); err != nil {
		t.Fatalf("System resource content is not valid JSON: %v", err)
	}
	
	if _, exists := systemInfo["go"]; !exists {
		t.Error("Expected 'go' field in system info")
	}
	
	if _, exists := systemInfo["memory"]; !exists {
		t.Error("Expected 'memory' field in system info")
	}
}

func validatePingResponse(t *testing.T, response JSONRPCResponse) {
	if response.Error != nil {
		t.Errorf("Expected no error, got %+v", response.Error)
	}
	
	result := response.Result.(map[string]interface{})
	
	if result["message"] != "pong" {
		t.Errorf("Expected message 'pong', got %v", result["message"])
	}
}