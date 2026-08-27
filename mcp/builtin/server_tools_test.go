package builtin

import (
	"net/http"
	"testing"

	"github.com/osauer/hyperserve/v2"
)

// TestRouteInspectorTool tests the RouteInspectorTool functionality
func TestRouteInspectorTool(t *testing.T) {
	// Create server with MCP support
	srv, err := hyperserve.New(hyperserve.WithMCPSupport("test", "1.0.0"))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add some test routes with middleware
	srv.UsePrefix("/api/test", hyperserve.RequestLoggerMiddleware)
	srv.UsePrefix("/api/users", hyperserve.RequestLoggerMiddleware)
	srv.UsePrefix("/admin", hyperserve.RequestLoggerMiddleware)
	srv.UsePrefix("/static", hyperserve.SecureWeb(srv.Options()))
	srv.UsePrefix("/middleware-only", hyperserve.RequestLoggerMiddleware)

	srv.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {})
	srv.GET("/api/users", func(w http.ResponseWriter, r *http.Request) {})
	srv.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {})
	srv.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {})

	tool := &RouteInspectorTool{server: srv}

	t.Run("basic_functionality", func(t *testing.T) {
		// Test basic route listing
		result, err := tool.Execute(map[string]any{})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Should have more than the original 5 hardcoded routes
		if len(routes) < 4 {
			t.Errorf("Expected at least 4 routes, got %d", len(routes))
		}

		// Check that we have the added routes
		foundRoutes := make(map[string]bool)
		for _, route := range routes {
			pattern, ok := route["pattern"].(string)
			if ok {
				foundRoutes[pattern] = true
			}
		}

		expectedRoutes := []string{"/api/test", "/api/users", "/admin", "/static"}
		for _, expected := range expectedRoutes {
			if !foundRoutes[expected] {
				t.Errorf("Expected route %s not found", expected)
			}
		}
		if foundRoutes["/middleware-only"] {
			t.Error("middleware-only path should not be reported as a registered route")
		}
	})

	t.Run("pattern_filtering", func(t *testing.T) {
		// Test route filtering by pattern
		result, err := tool.Execute(map[string]any{
			"pattern": "/api",
		})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Should only have routes containing "/api"
		for _, route := range routes {
			pattern, ok := route["pattern"].(string)
			if !ok {
				t.Errorf("Expected pattern to be string, got %T", route["pattern"])
				continue
			}
			if !contains(pattern, "/api") {
				t.Errorf("Route %s should contain '/api'", pattern)
			}
		}
	})

	t.Run("middleware_information", func(t *testing.T) {
		// Test middleware chain reporting
		result, err := tool.Execute(map[string]any{
			"include_middleware": true,
		})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Each route should have middleware information
		for _, route := range routes {
			middleware, ok := route["middleware"]
			if !ok {
				t.Errorf("Expected middleware information for route %v", route["pattern"])
			}

			// Middleware should be an array
			if _, ok := middleware.([]string); !ok {
				t.Errorf("Expected middleware to be []string, got %T", middleware)
			}
		}
	})

	t.Run("no_middleware_information", func(t *testing.T) {
		// Test when middleware information is disabled
		result, err := tool.Execute(map[string]any{
			"include_middleware": false,
		})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Routes should not have middleware information
		for _, route := range routes {
			if _, ok := route["middleware"]; ok {
				t.Errorf("Did not expect middleware information for route %v", route["pattern"])
			}
		}
	})

	t.Run("health_server_routes", func(t *testing.T) {
		// Create server with health server enabled
		srvWithHealth, err := hyperserve.New(
			hyperserve.WithMCPSupport("test", "1.0.0"),
			hyperserve.WithHealthServer(),
		)
		if err != nil {
			t.Fatalf("Failed to create server with health: %v", err)
		}

		tool := &RouteInspectorTool{server: srvWithHealth}

		result, err := tool.Execute(map[string]any{})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Should have health routes
		foundHealthRoutes := make(map[string]bool)
		for _, route := range routes {
			pattern, ok := route["pattern"].(string)
			if ok {
				foundHealthRoutes[pattern] = true
			}
		}

		expectedHealthRoutes := []string{"/healthz", "/readyz", "/livez"}
		for _, expected := range expectedHealthRoutes {
			if !foundHealthRoutes[expected] {
				t.Errorf("Expected health route %s not found", expected)
			}
		}
	})

	t.Run("mcp_endpoint_route", func(t *testing.T) {
		// Test MCP endpoint is included
		result, err := tool.Execute(map[string]any{})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		routes, ok := response["routes"].([]map[string]any)
		if !ok {
			t.Errorf("Expected routes to be []map[string]any, got %T", response["routes"])
		}

		// Should have MCP route
		foundMCPRoute := false
		for _, route := range routes {
			pattern, ok := route["pattern"].(string)
			if ok && pattern == srv.Options().MCPEndpoint {
				foundMCPRoute = true
				break
			}
		}

		if !foundMCPRoute {
			t.Errorf("Expected MCP route %s not found", srv.Options().MCPEndpoint)
		}
	})

	t.Run("tool_metadata", func(t *testing.T) {
		// Test tool name and description
		if tool.Name() != "route_inspector" {
			t.Errorf("Expected tool name 'route_inspector', got %s", tool.Name())
		}

		description := tool.Description()
		if description == "" {
			t.Error("Expected non-empty description")
		}

		// Test schema
		schema := tool.Schema()
		if schema == nil {
			t.Error("Schema should not be nil")
		}

		// Check that schema has expected structure
		if schema["type"] != "object" {
			t.Errorf("Expected schema type 'object', got %v", schema["type"])
		}

		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Error("Expected properties to be map[string]any")
		}

		// Should have pattern and include_middleware properties
		if _, ok := properties["pattern"]; !ok {
			t.Error("Expected pattern property in schema")
		}

		if _, ok := properties["include_middleware"]; !ok {
			t.Error("Expected include_middleware property in schema")
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && stringContains(s, substr)))
}

// Simple string contains implementation
func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestServerControlTool tests the ServerControlTool functionality
func TestServerControlTool(t *testing.T) {
	srv, err := hyperserve.New(hyperserve.WithMCPSupport("test", "1.0.0"))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	tool := &ServerControlTool{server: srv}

	t.Run("tool_metadata", func(t *testing.T) {
		if tool.Name() != "server_control" {
			t.Errorf("Expected tool name 'server_control', got %s", tool.Name())
		}

		description := tool.Description()
		if description == "" {
			t.Error("Expected non-empty description")
		}

		schema := tool.Schema()
		if schema == nil {
			t.Error("Schema should not be nil")
		}
	})

	t.Run("get_status", func(t *testing.T) {
		result, err := tool.Execute(map[string]any{
			"action": "get_status",
		})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		// Check expected fields
		expectedFields := []string{"running", "ready", "uptime", "log_level", "addr"}
		for _, field := range expectedFields {
			if _, ok := response[field]; !ok {
				t.Errorf("Expected field %s not found in response", field)
			}
		}
	})

	t.Run("invalid_action", func(t *testing.T) {
		_, err := tool.Execute(map[string]any{
			"action": "invalid_action",
		})
		if err == nil {
			t.Error("Expected error for invalid action")
		}
	})

	t.Run("missing_action", func(t *testing.T) {
		_, err := tool.Execute(map[string]any{})
		if err == nil {
			t.Error("Expected error for missing action")
		}
	})
}

// TestDevGuideTool tests the DevGuideTool functionality
func TestDevGuideTool(t *testing.T) {
	srv, err := hyperserve.New(hyperserve.WithMCPSupport("test", "1.0.0"))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	tool := &DevGuideTool{server: srv}

	t.Run("tool_metadata", func(t *testing.T) {
		if tool.Name() != "dev_guide" {
			t.Errorf("Expected tool name 'dev_guide', got %s", tool.Name())
		}

		description := tool.Description()
		if description == "" {
			t.Error("Expected non-empty description")
		}

		schema := tool.Schema()
		if schema == nil {
			t.Error("Schema should not be nil")
		}
	})

	t.Run("overview", func(t *testing.T) {
		result, err := tool.Execute(map[string]any{
			"topic": "overview",
		})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		// Check expected fields
		expectedFields := []string{"description", "tools", "resources", "tip"}
		for _, field := range expectedFields {
			if _, ok := response[field]; !ok {
				t.Errorf("Expected field %s not found in response", field)
			}
		}
	})

	t.Run("default_topic", func(t *testing.T) {
		// Test that default topic is overview
		result, err := tool.Execute(map[string]any{})
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}

		response, ok := result.(map[string]any)
		if !ok {
			t.Errorf("Expected map[string]any, got %T", result)
		}

		// Should have overview fields
		if _, ok := response["description"]; !ok {
			t.Error("Expected description field for default topic")
		}
	})

	t.Run("invalid_topic", func(t *testing.T) {
		_, err := tool.Execute(map[string]any{
			"topic": "invalid_topic",
		})
		if err == nil {
			t.Error("Expected error for invalid topic")
		}
	})
}
