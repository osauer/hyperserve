package server

import (
	"encoding/json"
	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
	"github.com/osauer/hyperserve/pkg/mcp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTool is a simple test tool for namespace testing
type TestTool struct {
	name        string
	description string
}

func (t *TestTool) Name() string {
	return t.name
}

func (t *TestTool) Description() string {
	return t.description
}

func (t *TestTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type": "string",
			},
		},
		"required": []string{"input"},
	}
}

func (t *TestTool) Execute(params map[string]any) (any, error) {
	input, _ := params["input"].(string)
	return map[string]any{
		"result": "Executed " + t.name + " with input: " + input,
	}, nil
}

// TestResource is a simple test resource for namespace testing
type TestResource struct {
	uri         string
	name        string
	description string
}

func (r *TestResource) URI() string {
	return r.uri
}

func (r *TestResource) Name() string {
	return r.name
}

func (r *TestResource) Description() string {
	return r.description
}

func (r *TestResource) MimeType() string {
	return "text/plain"
}

func (r *TestResource) Read() (any, error) {
	return "Content from " + r.name, nil
}

func (r *TestResource) List() ([]string, error) {
	return []string{r.uri}, nil
}

func TestMCPNamespaceIntegration(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{"ServerWithNamespaceOption", testServerWithNamespaceOption},
		{"DirectNamespaceRegistration", testDirectNamespaceRegistration},
		{"MixedRegistrationMethods", testMixedRegistrationMethods},
		{"NamespaceToolExecution", testNamespaceToolExecution},
		{"NamespaceResourceRead", testNamespaceResourceRead},
		{"DefaultNamespaceBehavior", testDefaultNamespaceBehavior},
		{"EmptyNamespaceHandling", testEmptyNamespaceHandling},
		{"ComplexNamespaceScenario", testComplexNamespaceScenario},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func testServerWithNamespaceOption(t *testing.T) {
	// Create tools for different namespaces
	dawPlayTool := &TestTool{name: "play", description: "Play audio"}
	dawStopTool := &TestTool{name: "stop", description: "Stop audio"}

	dbQueryTool := &TestTool{name: "query", description: "Execute query"}
	dbBackupTool := &TestTool{name: "backup", description: "Backup database"}

	// Create server with MCP support
	srv, err := NewServer(
		WithMCPSupport("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register namespaces after server creation
	err = srv.RegisterMCPNamespace("daw",
		mcp.WithNamespaceTools(dawPlayTool, dawStopTool),
	)
	if err != nil {
		t.Fatalf("Failed to register daw namespace: %v", err)
	}

	err = srv.RegisterMCPNamespace("db",
		mcp.WithNamespaceTools(dbQueryTool, dbBackupTool),
	)
	if err != nil {
		t.Fatalf("Failed to register db namespace: %v", err)
	}

	// List tools and verify namespace prefixes
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{
		"jsonrpc": "2.0",
		"method": "tools/list",
		"id": 1
	}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}

	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify tools have correct namespace prefixes
	expectedTools := map[string]string{
		"mcp__daw__play":  "Play audio",
		"mcp__daw__stop":  "Stop audio",
		"mcp__db__query":  "Execute query",
		"mcp__db__backup": "Backup database",
	}

	if len(response.Result.Tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(response.Result.Tools))
	}

	for _, tool := range response.Result.Tools {
		expectedDesc, exists := expectedTools[tool.Name]
		if !exists {
			t.Errorf("Unexpected tool name: %s", tool.Name)
		} else if tool.Description != expectedDesc {
			t.Errorf("Tool %s: expected description '%s', got '%s'",
				tool.Name, expectedDesc, tool.Description)
		}
	}
}

func testDirectNamespaceRegistration(t *testing.T) {
	handler := mcp.NewHandler(mcp.ServerInfo{Name: "test-server", Version: "1.0"})

	// Register individual tools in namespaces
	tool1 := &TestTool{name: "tool1", description: "First tool"}
	tool2 := &TestTool{name: "tool2", description: "Second tool"}

	handler.RegisterToolInNamespace(tool1, "namespace1")
	handler.RegisterToolInNamespace(tool2, "namespace2")

	// Register entire namespace
	tool3 := &TestTool{name: "tool3", description: "Third tool"}
	tool4 := &TestTool{name: "tool4", description: "Fourth tool"}

	err := handler.RegisterNamespace("namespace3",
		mcp.WithNamespaceTools(tool3, tool4),
	)
	if err != nil {
		t.Fatalf("Failed to register namespace: %v", err)
	}

	// Verify all tools are registered with correct names
	expectedTools := []string{
		"mcp__namespace1__tool1",
		"mcp__namespace2__tool2",
		"mcp__namespace3__tool3",
		"mcp__namespace3__tool4",
	}

	for _, expectedName := range expectedTools {
		if !handler.HasTool(expectedName) {
			t.Errorf("Expected tool %s not found", expectedName)
		}
	}
}

func testMixedRegistrationMethods(t *testing.T) {
	handler := mcp.NewHandler(mcp.ServerInfo{Name: "test-server", Version: "1.0"})

	// Register tools using different methods
	backwardCompatTool := &TestTool{name: "legacy", description: "Legacy tool"}
	handler.RegisterTool(backwardCompatTool) // No namespace prefix

	namespacedTool := &TestTool{name: "modern", description: "Modern tool"}
	handler.RegisterToolInNamespace(namespacedTool, "api")

	// Resources
	backwardCompatResource := &TestResource{uri: "legacy://resource", name: "Legacy Resource"}
	handler.RegisterResource(backwardCompatResource) // No namespace prefix

	namespacedResource := &TestResource{uri: "resource://modern", name: "Modern Resource"}
	handler.RegisterResourceInNamespace(namespacedResource, "api")

	// Verify registration
	if !handler.HasTool("legacy") {
		t.Error("Legacy tool should exist without prefix")
	}

	if !handler.HasTool("mcp__api__modern") {
		t.Error("Modern tool should exist with namespace prefix")
	}

	if !handler.HasResource("legacy://resource") {
		t.Error("Legacy resource should exist without prefix")
	}

	if !handler.HasResource("mcp__api__resource://modern") {
		t.Error("Modern resource should exist with namespace prefix")
	}
}

func testNamespaceToolExecution(t *testing.T) {
	handler := mcp.NewHandler(mcp.ServerInfo{Name: "test-server", Version: "1.0"})

	// Register tools in different namespaces
	mathTool := &TestTool{name: "calculate", description: "Math calculations"}
	handler.RegisterToolInNamespace(mathTool, "math")

	webTool := &TestTool{name: "fetch", description: "Web fetch"}
	handler.RegisterToolInNamespace(webTool, "web")

	// Execute math tool
	mathRequest := &jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]any{
			"name": "mcp__math__calculate",
			"arguments": map[string]any{
				"input": "2+2",
			},
		},
		ID: 1,
	}

	response := handler.RPCEngine().ProcessRequestDirect(mathRequest)
	if response.Error != nil {
		t.Fatalf("Math tool execution failed: %v", response.Error)
	}

	// Verify response
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatal("Expected map result")
	}

	content, ok := result["content"].([]map[string]any)
	if !ok || len(content) == 0 {
		t.Fatal("Expected content array in result")
	}

	text, _ := content[0]["text"].(string)
	if !strings.Contains(text, "calculate") || !strings.Contains(text, "2+2") {
		t.Errorf("Unexpected result text: %s", text)
	}

	// Execute web tool
	webRequest := &jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]any{
			"name": "mcp__web__fetch",
			"arguments": map[string]any{
				"input": "https://example.com",
			},
		},
		ID: 2,
	}

	response = handler.RPCEngine().ProcessRequestDirect(webRequest)
	if response.Error != nil {
		t.Fatalf("Web tool execution failed: %v", response.Error)
	}
}

func testNamespaceResourceRead(t *testing.T) {
	handler := mcp.NewHandler(mcp.ServerInfo{Name: "test-server", Version: "1.0"})

	// Register resources in different namespaces
	configResource := &TestResource{
		uri:         "config://app",
		name:        "App Config",
		description: "Application configuration",
	}
	handler.RegisterResourceInNamespace(configResource, "system")

	dataResource := &TestResource{
		uri:         "data://users",
		name:        "User Data",
		description: "User information",
	}
	handler.RegisterResourceInNamespace(dataResource, "db")

	// List resources
	listRequest := &jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "resources/list",
		ID:      1,
	}

	response := handler.RPCEngine().ProcessRequestDirect(listRequest)
	if response.Error != nil {
		t.Fatalf("Resource list failed: %v", response.Error)
	}

	result, _ := response.Result.(map[string]any)
	resources, _ := result["resources"].([]map[string]any)

	// Verify resources have namespace prefixes
	expectedURIs := map[string]bool{
		"mcp__system__config://app": false,
		"mcp__db__data://users":     false,
	}

	for _, resource := range resources {
		uri, _ := resource["uri"].(string)
		if _, expected := expectedURIs[uri]; expected {
			expectedURIs[uri] = true
		}
	}

	for uri, found := range expectedURIs {
		if !found {
			t.Errorf("Expected resource URI not found: %s", uri)
		}
	}

	// Read namespaced resource
	readRequest := &jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "resources/read",
		Params: map[string]any{
			"uri": "mcp__system__config://app",
		},
		ID: 2,
	}

	response = handler.RPCEngine().ProcessRequestDirect(readRequest)
	if response.Error != nil {
		t.Fatalf("Resource read failed: %v", response.Error)
	}

	result, _ = response.Result.(map[string]any)
	contents, _ := result["contents"].([]map[string]any)
	if len(contents) == 0 {
		t.Fatal("Expected resource contents")
	}

	text, _ := contents[0]["text"].(string)
	if text != "Content from App Config" {
		t.Errorf("Unexpected resource content: %s", text)
	}
}

func testDefaultNamespaceBehavior(t *testing.T) {
	serverInfo := mcp.ServerInfo{Name: "myserver", Version: "1.0"}
	handler := mcp.NewHandler(serverInfo)

	// Register tool without specifying namespace - should use server name
	tool := &TestTool{name: "default_tool", description: "Tool in default namespace"}
	handler.RegisterToolInNamespace(tool, "") // Empty namespace should use default

	// Verify tool is registered with server name as namespace
	expectedName := "mcp__myserver__default_tool"
	if !handler.HasTool(expectedName) {
		t.Errorf("Expected tool %s not found", expectedName)
	}

	// Note: Default namespace concept removed for simplified implementation
}

func testEmptyNamespaceHandling(t *testing.T) {
	handler := mcp.NewHandler(mcp.ServerInfo{Name: "test", Version: "1.0"})

	// Test empty namespace registration error
	err := handler.RegisterNamespace("", mcp.WithNamespaceTools())
	if err == nil {
		t.Error("Expected error when registering empty namespace")
	}
	if !strings.Contains(err.Error(), "namespace name cannot be empty") {
		t.Errorf("Expected specific error message, got: %v", err)
	}

	// Test registering with empty namespace should use default
	tool := &TestTool{name: "tool", description: "Test tool"}
	handler.RegisterToolInNamespace(tool, "")

	expectedName := "mcp__test__tool" // Should use server name as default
	if !handler.HasTool(expectedName) {
		t.Errorf("Tool should be registered with default namespace, expected %s", expectedName)
	}
}

func testComplexNamespaceScenario(t *testing.T) {
	// Create a server with MCP support
	srv, err := NewServer(
		WithMCPSupport("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register multiple namespaces
	err = srv.RegisterMCPNamespace("analytics",
		mcp.WithNamespaceTools(
			&TestTool{name: "track", description: "Track events"},
			&TestTool{name: "report", description: "Generate reports"},
		),
		mcp.WithNamespaceResources(
			&TestResource{uri: "metrics://daily", name: "Daily Metrics"},
			&TestResource{uri: "metrics://monthly", name: "Monthly Metrics"},
		),
	)
	if err != nil {
		t.Fatalf("Failed to register analytics namespace: %v", err)
	}

	err = srv.RegisterMCPNamespace("admin",
		mcp.WithNamespaceTools(
			&TestTool{name: "users", description: "Manage users"},
			&TestTool{name: "config", description: "Manage config"},
		),
		mcp.WithNamespaceResources(
			&TestResource{uri: "settings://global", name: "Global Settings"},
		),
	)
	if err != nil {
		t.Fatalf("Failed to register admin namespace: %v", err)
	}

	// Test listing all tools
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{
		"jsonrpc": "2.0",
		"method": "tools/list",
		"id": 1
	}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var toolsResponse struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	json.NewDecoder(w.Body).Decode(&toolsResponse)

	// Verify we have all expected tools
	expectedToolCount := 4 // 2 analytics + 2 admin
	if len(toolsResponse.Result.Tools) != expectedToolCount {
		t.Errorf("Expected %d tools, got %d", expectedToolCount, len(toolsResponse.Result.Tools))
	}

	// Test listing all resources
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{
		"jsonrpc": "2.0",
		"method": "resources/list",
		"id": 2
	}`))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var resourcesResponse struct {
		Result struct {
			Resources []struct {
				URI string `json:"uri"`
			} `json:"resources"`
		} `json:"result"`
	}

	json.NewDecoder(w.Body).Decode(&resourcesResponse)

	// Verify we have all expected resources
	expectedResourceCount := 3 // 2 analytics + 1 admin
	if len(resourcesResponse.Result.Resources) != expectedResourceCount {
		t.Errorf("Expected %d resources, got %d", expectedResourceCount, len(resourcesResponse.Result.Resources))
	}

	// Verify namespace prefixes are applied correctly
	toolNames := make(map[string]bool)
	for _, tool := range toolsResponse.Result.Tools {
		toolNames[tool.Name] = true
	}

	expectedToolNames := []string{
		"mcp__analytics__track",
		"mcp__analytics__report",
		"mcp__admin__users",
		"mcp__admin__config",
	}

	for _, expectedName := range expectedToolNames {
		if !toolNames[expectedName] {
			t.Errorf("Expected tool %s not found", expectedName)
		}
	}

	// Execute a tool from a specific namespace
	req = httptest.NewRequest("POST", "/mcp", strings.NewReader(`{
		"jsonrpc": "2.0",
		"method": "tools/call",
		"params": {
			"name": "mcp__analytics__track",
			"arguments": {
				"input": "user_login"
			}
		},
		"id": 3
	}`))
	req.Header.Set("Content-Type", "application/json")

	w = httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Tool execution failed with status %d", w.Code)
	}

	var execResponse struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	json.NewDecoder(w.Body).Decode(&execResponse)

	if execResponse.Error != nil {
		t.Errorf("Tool execution error: %s", execResponse.Error.Message)
	}

	if len(execResponse.Result.Content) == 0 {
		t.Error("Expected execution result content")
	}
}
