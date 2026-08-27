package hyperserve

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCustomTool implements mcp.Tool for testing
type TestCustomTool struct {
	name string
}

func (t *TestCustomTool) Name() string        { return t.name }
func (t *TestCustomTool) Description() string { return "Test tool" }
func (t *TestCustomTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type": "string",
			},
		},
	}
}
func (t *TestCustomTool) Execute(params map[string]any) (any, error) {
	return map[string]any{"result": "ok"}, nil
}

// TestCustomResource implements mcp.Resource for testing
type TestCustomResource struct {
	uri string
}

func (r *TestCustomResource) URI() string         { return r.uri }
func (r *TestCustomResource) Name() string        { return "Test resource" }
func (r *TestCustomResource) Description() string { return "Test resource" }
func (r *TestCustomResource) MimeType() string    { return "application/json" }
func (r *TestCustomResource) Read() (any, error) {
	return map[string]any{"data": "test"}, nil
}
func (r *TestCustomResource) List() ([]string, error) {
	return []string{r.uri}, nil
}

type TestCustomResourceTemplate struct{}

func (t *TestCustomResourceTemplate) URITemplate() string { return "test://{id}" }
func (t *TestCustomResourceTemplate) Name() string        { return "Test resource template" }
func (t *TestCustomResourceTemplate) Description() string { return "Test resource template" }
func (t *TestCustomResourceTemplate) MimeType() string    { return "application/json" }
func (t *TestCustomResourceTemplate) Match(uri string) (map[string]string, bool) {
	id, ok := strings.CutPrefix(uri, "test://")
	if !ok || id == "" {
		return nil, false
	}
	return map[string]string{"id": id}, true
}
func (t *TestCustomResourceTemplate) Read(context.Context, string, map[string]string) (any, error) {
	return map[string]any{"data": "test"}, nil
}

func TestMCPCustomRegistration(t *testing.T) {
	// Run tests in isolation to avoid state pollution
	t.Run("RegisterTool", func(t *testing.T) {
		t.Parallel()
		// Create server with MCP enabled
		srv, err := New(
			WithMCPSupport("hyperserve", "1.0.0"),
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
		if !srv.MCPHandler().HasTool(tool.Name()) {
			t.Fatal("Tool was not registered")
		}
	})

	t.Run("RegisterResource", func(t *testing.T) {
		t.Parallel()
		// Create server with MCP enabled
		srv, err := New(
			WithMCPSupport("hyperserve", "1.0.0"),
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
		if !srv.MCPHandler().HasResource(resource.URI()) {
			t.Fatal("Resource was not registered")
		}
	})

	t.Run("RegisterResourceTemplate", func(t *testing.T) {
		t.Parallel()
		srv, err := New(
			WithMCPSupport("hyperserve", "1.0.0"),
		)
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}

		template := &TestCustomResourceTemplate{}
		err = srv.RegisterMCPResourceTemplate(template)
		if err != nil {
			t.Fatalf("Failed to register resource template: %v", err)
		}
		if !srv.MCPHandler().HasResourceTemplate(template.URITemplate()) {
			t.Fatal("Resource template was not registered")
		}
	})

	t.Run("RegisterWithoutMCP", func(t *testing.T) {
		t.Parallel()
		// Ensure MCP env var is not set
		os.Unsetenv("HS_MCP_ENABLED")

		// Create server without MCP
		srv, err := New()
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

		template := &TestCustomResourceTemplate{}
		err = srv.RegisterMCPResourceTemplate(template)
		if err == nil {
			t.Fatal("Expected error when registering resource template without MCP enabled")
		}
	})
}
