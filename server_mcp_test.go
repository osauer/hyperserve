package hyperserve

import (
	"os"
	"testing"
)

// TestCustomTool implements MCPTool for testing
type TestCustomTool struct {
	name string
}

func (t *TestCustomTool) Name() string        { return t.name }
func (t *TestCustomTool) Description() string { return "Test tool" }
func (t *TestCustomTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type": "string",
			},
		},
	}
}
func (t *TestCustomTool) Execute(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"result": "ok"}, nil
}

// TestCustomResource implements MCPResource for testing
type TestCustomResource struct {
	uri string
}

func (r *TestCustomResource) URI() string         { return r.uri }
func (r *TestCustomResource) Name() string        { return "Test resource" }
func (r *TestCustomResource) Description() string { return "Test resource" }
func (r *TestCustomResource) MimeType() string    { return "application/json" }
func (r *TestCustomResource) Read() (interface{}, error) {
	return map[string]interface{}{"data": "test"}, nil
}
func (r *TestCustomResource) List() ([]string, error) {
	return []string{r.uri}, nil
}

func TestMCPCustomRegistration(t *testing.T) {
	// Run tests in isolation to avoid state pollution
	t.Run("RegisterTool", func(t *testing.T) {
		t.Parallel()
		// Create server with MCP enabled
		srv, err := NewServer(
			WithMCPSupport(),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// Check MCP is enabled
		if !srv.MCPEnabled() {
			t.Fatal("MCP should be enabled")
		}

		// Register custom tool
		tool := &TestCustomTool{name: "test_tool"}
		err = srv.RegisterMCPTool(tool)
		if err != nil {
			t.Fatalf("Failed to register tool: %v", err)
		}

		// Verify tool was registered by checking handler's tools map
		if srv.mcpHandler.tools[tool.Name()] == nil {
			t.Fatal("Tool was not registered")
		}
	})

	t.Run("RegisterResource", func(t *testing.T) {
		t.Parallel()
		// Create server with MCP enabled
		srv, err := NewServer(
			WithMCPSupport(),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// Register custom resource
		resource := &TestCustomResource{uri: "test://resource"}
		err = srv.RegisterMCPResource(resource)
		if err != nil {
			t.Fatalf("Failed to register resource: %v", err)
		}

		// Verify resource was registered
		if srv.mcpHandler.resources[resource.URI()] == nil {
			t.Fatal("Resource was not registered")
		}
	})

	t.Run("RegisterWithoutMCP", func(t *testing.T) {
		t.Parallel()
		// Ensure MCP env var is not set
		os.Unsetenv("HS_MCP_ENABLED")
		
		// Create server without MCP
		srv, err := NewServer()
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		// Check MCP is disabled
		if srv.MCPEnabled() {
			t.Fatal("MCP should be disabled")
		}

		// Try to register tool - should fail
		tool := &TestCustomTool{name: "test_tool"}
		err = srv.RegisterMCPTool(tool)
		if err == nil {
			t.Fatal("Expected error when registering tool without MCP enabled")
		}

		// Try to register resource - should fail
		resource := &TestCustomResource{uri: "test://resource"}
		err = srv.RegisterMCPResource(resource)
		if err == nil {
			t.Fatal("Expected error when registering resource without MCP enabled")
		}
	})
}