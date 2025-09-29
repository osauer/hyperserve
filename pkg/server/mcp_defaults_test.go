package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMCPBuiltinDefaults verifies that built-in tools and resources are disabled by default
func TestMCPBuiltinDefaults(t *testing.T) {
	t.Run("BuiltinToolsDisabledByDefault", func(t *testing.T) {
		// Create server with MCP support but no explicit tool enabling
		srv, err := NewServer(
			WithMCPSupport("test-server", "1.0.0"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// List tools
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      1,
		}

		response := makeMCPRequestToServer(t, srv, request)

		// Check response
		if response.Error != nil {
			t.Fatalf("Unexpected error: %v", response.Error)
		}

		// Parse result
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}

		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Fatal("Expected tools to be an array")
		}

		// Should have no tools
		if len(tools) != 0 {
			t.Errorf("Expected 0 tools (disabled by default), got %d", len(tools))
		}
	})

	t.Run("BuiltinResourcesDisabledByDefault", func(t *testing.T) {
		// Create server with MCP support but no explicit resource enabling
		srv, err := NewServer(
			WithMCPSupport("test-server", "1.0.0"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// List resources
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "resources/list",
			"id":      1,
		}

		response := makeMCPRequestToServer(t, srv, request)

		// Check response
		if response.Error != nil {
			t.Fatalf("Unexpected error: %v", response.Error)
		}

		// Parse result
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}

		resources, ok := result["resources"].([]interface{})
		if !ok {
			t.Fatal("Expected resources to be an array")
		}

		// Should have no resources
		if len(resources) != 0 {
			t.Errorf("Expected 0 resources (disabled by default), got %d", len(resources))
		}
	})

	t.Run("BuiltinToolsEnabledExplicitly", func(t *testing.T) {
		// Create server with MCP support and explicitly enable tools
		srv, err := NewServer(
			WithMCPSupport("test-server", "1.0.0"),
			WithMCPBuiltinTools(true),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// List tools
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      1,
		}

		response := makeMCPRequestToServer(t, srv, request)

		// Check response
		if response.Error != nil {
			t.Fatalf("Unexpected error: %v", response.Error)
		}

		// Parse result
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}

		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Fatal("Expected tools to be an array")
		}

		// Should have built-in tools
		if len(tools) < 2 {
			t.Errorf("Expected at least 2 built-in tools, got %d", len(tools))
		}

		// Check for specific tools
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolMap, ok := tool.(map[string]interface{})
			if ok {
				if name, ok := toolMap["name"].(string); ok {
					toolNames[name] = true
				}
			}
		}

		// Verify calculator and http_request are present
		if !toolNames["mcp__hyperserve__calculator"] {
			t.Error("Expected mcp__hyperserve__calculator tool to be present")
		}
		if !toolNames["mcp__hyperserve__http_request"] {
			t.Error("Expected mcp__hyperserve__http_request tool to be present")
		}
	})

	t.Run("BuiltinResourcesEnabledExplicitly", func(t *testing.T) {
		// Create server with MCP support and explicitly enable resources
		srv, err := NewServer(
			WithMCPSupport("test-server", "1.0.0"),
			WithMCPBuiltinResources(true),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// List resources
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "resources/list",
			"id":      1,
		}

		response := makeMCPRequestToServer(t, srv, request)

		// Check response
		if response.Error != nil {
			t.Fatalf("Unexpected error: %v", response.Error)
		}

		// Parse result
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Fatal("Expected result to be a map")
		}

		resources, ok := result["resources"].([]interface{})
		if !ok {
			t.Fatal("Expected resources to be an array")
		}

		// Should have built-in resources
		if len(resources) < 2 {
			t.Errorf("Expected at least 2 built-in resources, got %d", len(resources))
		}

		// Check for specific resources
		resourceURIs := make(map[string]bool)
		for _, resource := range resources {
			resourceMap, ok := resource.(map[string]interface{})
			if ok {
				if uri, ok := resourceMap["uri"].(string); ok {
					resourceURIs[uri] = true
				}
			}
		}

		// Verify some resources are present
		if !resourceURIs["config://server/options"] {
			t.Error("Expected config://server/options resource to be present")
		}
		if !resourceURIs["system://runtime/info"] {
			t.Error("Expected system://runtime/info resource to be present")
		}
	})
}

// TestMCPGetRequest verifies that GET requests return helpful documentation
func TestMCPGetRequest(t *testing.T) {
	srv, err := NewServer(
		WithMCPSupport("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()

	srv.mcpHandler.ServeHTTP(w, req)

	// Should return 200 OK with HTML content
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected Content-Type text/html; charset=utf-8, got %s", contentType)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("Model Context Protocol")) {
		t.Error("Expected response to contain 'Model Context Protocol'")
	}
	if !bytes.Contains([]byte(body), []byte("JSON-RPC 2.0")) {
		t.Error("Expected response to contain 'JSON-RPC 2.0'")
	}
}

// Helper function to make MCP request to a server
func makeMCPRequestToServer(t *testing.T, srv *Server, request map[string]interface{}) JSONRPCResponse {
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.mcpHandler.ServeHTTP(w, req)

	// Should return 200 OK
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var response JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	return response
}
