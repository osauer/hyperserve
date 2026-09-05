package builtin

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
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
			if !strings.Contains(pattern, "/api") {
				t.Errorf("Route %s should contain '/api'", pattern)
			}
		}
	})

	t.Run("middleware_information", func(t *testing.T) {
		result, err := tool.Execute(map[string]any{
			"include_middleware": true,
			"pattern":            "/api",
		})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		response := result.(map[string]any)
		want := []map[string]any{
			{"prefix": "*", "count": 3},
			{"prefix": "/admin", "count": 1},
			{"prefix": "/api/test", "count": 1},
			{"prefix": "/api/users", "count": 1},
			{"prefix": "/middleware-only", "count": 1},
			{"prefix": "/static", "count": 1},
		}
		if got := response["middleware_registrations"]; !reflect.DeepEqual(got, want) {
			t.Errorf("middleware registrations = %#v, want %#v", got, want)
		}
		for _, route := range response["routes"].([]map[string]any) {
			if _, exists := route["middleware"]; exists {
				t.Errorf("route %s must not claim an inferred middleware chain", route["pattern"])
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
		for _, key := range []string{"middleware_registrations", "middleware_note"} {
			if _, exists := response[key]; exists {
				t.Errorf("include_middleware=false returned %q", key)
			}
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

func TestRouteInspectorMCPMethodsMatchTransportMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		legacy  bool
		methods []string
	}{
		{name: "standards only", methods: []string{"POST"}},
		{name: "legacy routed SSE", legacy: true, methods: []string{"GET", "POST"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := []hyperserve.Option{hyperserve.WithMCPSupport("test", "1.0.0")}
			if test.legacy {
				//lint:ignore SA1019 This truth test covers the explicit legacy compatibility mode.
				options = append(options, hyperserve.WithMCPLegacyRoutedSSE(true))
			}
			app, err := hyperserve.New(options...)
			if err != nil {
				t.Fatalf("create server: %v", err)
			}

			result, err := NewRouteInspectorTool(app).Execute(map[string]any{
				"pattern": app.Options().MCPEndpoint,
			})
			if err != nil {
				t.Fatalf("inspect routes: %v", err)
			}
			routes := result.(map[string]any)["routes"].([]map[string]any)
			for _, route := range routes {
				if route["pattern"] != app.Options().MCPEndpoint {
					continue
				}
				methods, ok := route["methods"].([]string)
				if !ok {
					t.Fatalf("MCP methods have type %T, want []string", route["methods"])
				}
				if !slices.Equal(methods, test.methods) {
					t.Fatalf("MCP methods = %v, want %v", methods, test.methods)
				}
				return
			}
			t.Fatalf("MCP route %q not reported", app.Options().MCPEndpoint)
		})
	}
}

func TestRouteInspectorPreservesSamePathAcrossMainAndHealthServers(t *testing.T) {
	t.Parallel()

	app, err := hyperserve.New(hyperserve.WithHealthServer())
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	app.HandleFunc("/healthz", func(http.ResponseWriter, *http.Request) {})

	result, err := NewRouteInspectorTool(app).Execute(map[string]any{"pattern": "/healthz"})
	if err != nil {
		t.Fatalf("inspect routes: %v", err)
	}
	routes := result.(map[string]any)["routes"].([]map[string]any)
	methodsByServer := make(map[string][]string)
	for _, route := range routes {
		if route["pattern"] != "/healthz" {
			continue
		}
		server, _ := route["server"].(string)
		methods, _ := route["methods"].([]string)
		methodsByServer[server] = methods
	}
	if !slices.Equal(methodsByServer["main"], []string{"ANY"}) {
		t.Errorf("main /healthz methods = %v, want [ANY]", methodsByServer["main"])
	}
	if !slices.Equal(methodsByServer["health"], []string{"GET"}) {
		t.Errorf("health /healthz methods = %v, want [GET]", methodsByServer["health"])
	}
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
		if got := response["uptime"]; got != "0s" {
			t.Errorf("pre-Run uptime = %v, want 0s", got)
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

func TestServerControlActionParity(t *testing.T) {
	srv, err := hyperserve.New(hyperserve.WithMCPSupport("test", "1.0.0"))
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	control := &ServerControlTool{server: srv}
	guide := &DevGuideTool{server: srv}

	properties, ok := control.Schema()["properties"].(map[string]any)
	if !ok {
		t.Fatal("server_control schema properties have unexpected shape")
	}
	actionSchema, ok := properties["action"].(map[string]any)
	if !ok {
		t.Fatal("server_control action schema has unexpected shape")
	}
	schemaActions, ok := actionSchema["enum"].([]string)
	if !ok {
		t.Fatalf("server_control action enum has unexpected type %T", actionSchema["enum"])
	}
	if !slices.Equal(schemaActions, serverControlActions()) {
		t.Fatalf("schema actions %v differ from runtime actions %v", schemaActions, serverControlActions())
	}
	for _, action := range schemaActions {
		if _, err := control.Execute(map[string]any{"action": action}); err != nil {
			t.Errorf("schema advertises action %q that runtime rejects: %v", action, err)
		}
	}
	if _, err := control.Execute(map[string]any{"action": "set_log_level"}); err == nil {
		t.Error("removed set_log_level action must remain rejected")
	}

	overviewResult, err := guide.Execute(map[string]any{"topic": "overview"})
	if err != nil {
		t.Fatalf("execute dev guide overview: %v", err)
	}
	overview := overviewResult.(map[string]any)
	overviewTools := overview["tools"].([]map[string]any)
	var overviewActions []string
	for _, advertised := range overviewTools {
		if advertised["name"] == "server_control" {
			overviewActions, _ = advertised["actions"].([]string)
		}
	}
	if !slices.Equal(overviewActions, schemaActions) {
		t.Errorf("dev guide overview actions %v differ from schema actions %v", overviewActions, schemaActions)
	}

	toolsResult, err := guide.Execute(map[string]any{"topic": "tools"})
	if err != nil {
		t.Fatalf("execute dev guide tools: %v", err)
	}
	availableTools := toolsResult.(map[string]any)["available_tools"].([]map[string]any)
	var documentedActions map[string]string
	for _, advertised := range availableTools {
		if advertised["tool"] == "server_control" {
			documentedActions, _ = advertised["actions"].(map[string]string)
		}
	}
	if len(documentedActions) != len(schemaActions) {
		t.Errorf("dev guide documents %d server_control actions; schema advertises %d", len(documentedActions), len(schemaActions))
	}
	for action := range documentedActions {
		if !slices.Contains(schemaActions, action) {
			t.Errorf("dev guide documents server_control action %q that schema omits", action)
		}
	}

	examplesResult, err := guide.Execute(map[string]any{"topic": "examples"})
	if err != nil {
		t.Fatalf("execute dev guide examples: %v", err)
	}
	examples := examplesResult.(map[string]any)["common_examples"].([]map[string]any)
	for _, example := range examples {
		if example["tool"] != "server_control" {
			continue
		}
		arguments := example["arguments"].(map[string]any)
		action, _ := arguments["action"].(string)
		if !slices.Contains(schemaActions, action) {
			t.Errorf("dev guide example advertises server_control action %q that schema omits", action)
		}
	}

	for _, topic := range []string{"overview", "tools", "resources", "examples", "workflows"} {
		result, err := guide.Execute(map[string]any{"topic": topic})
		if err != nil {
			t.Fatalf("execute dev guide topic %q: %v", topic, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("encode dev guide topic %q: %v", topic, err)
		}
		if strings.Contains(string(encoded), "set_log_level") {
			t.Errorf("dev guide topic %q still advertises removed set_log_level action", topic)
		}
		if strings.Contains(strings.ToLower(string(encoded)), "real-time mcp server log") {
			t.Errorf("dev guide topic %q advertises snapshot logs as real-time", topic)
		}
	}
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
