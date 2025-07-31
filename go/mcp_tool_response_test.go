package hyperserve

import (
	"encoding/json"
	"testing"
)

// Mock tools for testing different response types
type mockStringTool struct{}

func (t *mockStringTool) Name() string        { return "string_tool" }
func (t *mockStringTool) Description() string { return "Returns a string" }
func (t *mockStringTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockStringTool) Execute(params map[string]interface{}) (interface{}, error) {
	return "Hello, World!", nil
}

type mockMapTool struct{}

func (t *mockMapTool) Name() string        { return "map_tool" }
func (t *mockMapTool) Description() string { return "Returns a map" }
func (t *mockMapTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockMapTool) Execute(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"status":  "success",
		"value":   42,
		"message": "Operation completed",
	}, nil
}

type mockMCPFormattedTool struct{}

func (t *mockMCPFormattedTool) Name() string        { return "mcp_formatted_tool" }
func (t *mockMCPFormattedTool) Description() string { return "Returns MCP-formatted response" }
func (t *mockMCPFormattedTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockMCPFormattedTool) Execute(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "Pre-formatted response",
			},
			{
				"type": "text",
				"text": "With multiple items",
			},
		},
	}, nil
}

type mockErrorTool struct{}

func (t *mockErrorTool) Name() string        { return "error_tool" }
func (t *mockErrorTool) Description() string { return "Returns an error response" }
func (t *mockErrorTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockErrorTool) Execute(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"isError": true,
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": "An error occurred",
			},
		},
	}, nil
}

type mockArrayTool struct{}

func (t *mockArrayTool) Name() string        { return "array_tool" }
func (t *mockArrayTool) Description() string { return "Returns an array" }
func (t *mockArrayTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockArrayTool) Execute(params map[string]interface{}) (interface{}, error) {
	return []interface{}{"item1", "item2", "item3"}, nil
}

func TestMCPHandler_ToolResponseFormatting(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	tests := []struct {
		name           string
		tool           MCPTool
		expectedType   string
		validateResult func(t *testing.T, result interface{})
	}{
		{
			name:         "string response",
			tool:         &mockStringTool{},
			expectedType: "text",
			validateResult: func(t *testing.T, result interface{}) {
				response := result.(map[string]interface{})
				content := response["content"].([]map[string]interface{})
				if len(content) == 0 {
					t.Fatal("Expected at least one content item")
				}
				firstItem := content[0]
				if firstItem["type"] != "text" {
					t.Errorf("Expected type 'text', got %v", firstItem["type"])
				}
				if firstItem["text"] != "Hello, World!" {
					t.Errorf("Expected text 'Hello, World!', got %v", firstItem["text"])
				}
			},
		},
		{
			name:         "map response",
			tool:         &mockMapTool{},
			expectedType: "text",
			validateResult: func(t *testing.T, result interface{}) {
				response := result.(map[string]interface{})
				content := response["content"].([]map[string]interface{})
				if len(content) == 0 {
					t.Fatal("Expected at least one content item")
				}
				firstItem := content[0]
				if firstItem["type"] != "text" {
					t.Errorf("Expected type 'text', got %v", firstItem["type"])
				}
				// Verify it's valid JSON
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(firstItem["text"].(string)), &parsed); err != nil {
					t.Errorf("Failed to parse JSON response: %v", err)
				}
				if parsed["status"] != "success" {
					t.Errorf("Expected status 'success', got %v", parsed["status"])
				}
			},
		},
		{
			name:         "MCP formatted response",
			tool:         &mockMCPFormattedTool{},
			expectedType: "text",
			validateResult: func(t *testing.T, result interface{}) {
				response := result.(map[string]interface{})
				content := response["content"].([]map[string]interface{})
				if len(content) != 2 {
					t.Errorf("Expected 2 content items, got %d", len(content))
				}
				firstItem := content[0]
				if firstItem["text"] != "Pre-formatted response" {
					t.Errorf("Unexpected first item text: %v", firstItem["text"])
				}
			},
		},
		{
			name:         "error response",
			tool:         &mockErrorTool{},
			expectedType: "text",
			validateResult: func(t *testing.T, result interface{}) {
				response := result.(map[string]interface{})
				if !response["isError"].(bool) {
					t.Error("Expected isError to be true")
				}
				content := response["content"].([]map[string]interface{})
				if len(content) == 0 {
					t.Fatal("Expected at least one content item")
				}
				firstItem := content[0]
				if firstItem["text"] != "An error occurred" {
					t.Errorf("Unexpected error text: %v", firstItem["text"])
				}
			},
		},
		{
			name:         "array response",
			tool:         &mockArrayTool{},
			expectedType: "text",
			validateResult: func(t *testing.T, result interface{}) {
				response := result.(map[string]interface{})
				content := response["content"].([]map[string]interface{})
				if len(content) == 0 {
					t.Fatal("Expected at least one content item")
				}
				firstItem := content[0]
				if firstItem["type"] != "text" {
					t.Errorf("Expected type 'text', got %v", firstItem["type"])
				}
				// Verify it's valid JSON array
				var parsed []interface{}
				if err := json.Unmarshal([]byte(firstItem["text"].(string)), &parsed); err != nil {
					t.Errorf("Failed to parse JSON array response: %v", err)
				}
				if len(parsed) != 3 {
					t.Errorf("Expected 3 items in array, got %d", len(parsed))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler.RegisterTool(tt.tool)

			// Call the tool through the handler
			params := map[string]interface{}{
				"name":      tt.tool.Name(),
				"arguments": map[string]interface{}{},
			}

			result, err := handler.handleToolsCall(params)
			if err != nil {
				t.Fatalf("Tool call failed: %v", err)
			}

			tt.validateResult(t, result)
		})
	}
}

// Test that tools can return complex content types
func TestMCPHandler_ComplexContentTypes(t *testing.T) {
	handler := NewMCPHandler(MCPServerInfo{Name: "test", Version: "1.0"})

	// Mock tool that returns different content types
	complexTool := &mockComplexContentTool{}
	handler.RegisterTool(complexTool)

	params := map[string]interface{}{
		"name":      "complex_content_tool",
		"arguments": map[string]interface{}{},
	}

	result, err := handler.handleToolsCall(params)
	if err != nil {
		t.Fatalf("Tool call failed: %v", err)
	}

	response := result.(map[string]interface{})
	content := response["content"].([]map[string]interface{})

	if len(content) != 3 {
		t.Fatalf("Expected 3 content items, got %d", len(content))
	}

	// Check first item is text
	first := content[0]
	if first["type"] != "text" {
		t.Errorf("Expected first item type 'text', got %v", first["type"])
	}

	// Check second item is image
	second := content[1]
	if second["type"] != "image" {
		t.Errorf("Expected second item type 'image', got %v", second["type"])
	}
	if second["mimeType"] != "image/png" {
		t.Errorf("Expected mime type 'image/png', got %v", second["mimeType"])
	}

	// Check third item is resource
	third := content[2]
	if third["type"] != "resource" {
		t.Errorf("Expected third item type 'resource', got %v", third["type"])
	}
}

type mockComplexContentTool struct{}

func (t *mockComplexContentTool) Name() string        { return "complex_content_tool" }
func (t *mockComplexContentTool) Description() string { return "Returns multiple content types" }
func (t *mockComplexContentTool) Schema() map[string]interface{} {
	return map[string]interface{}{"type": "object"}
}
func (t *mockComplexContentTool) Execute(params map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": "Here's some text",
			},
			map[string]interface{}{
				"type":     "image",
				"data":     "base64encodeddata",
				"mimeType": "image/png",
			},
			map[string]interface{}{
				"type": "resource",
				"resource": map[string]interface{}{
					"uri":  "file://example.txt",
					"name": "Example File",
				},
			},
		},
	}, nil
}