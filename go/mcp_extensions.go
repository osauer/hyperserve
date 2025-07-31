// Package hyperserve provides MCP extension capabilities for custom applications.
//
// This file contains helpers and patterns for applications built on hyperserve
// to easily add their own MCP tools, resources, and (future) prompts.
package hyperserve

import (
	"fmt"
)

// MCPExtension represents a collection of MCP tools and resources
// that can be registered as a group. This makes it easy for applications
// to package related functionality together.
type MCPExtension interface {
	// Name returns the extension name (e.g., "e-commerce", "blog", "analytics")
	Name() string
	
	// Description returns a human-readable description
	Description() string
	
	// Tools returns the tools provided by this extension
	Tools() []MCPTool
	
	// Resources returns the resources provided by this extension  
	Resources() []MCPResource
	
	// Configure is called with the server instance before registration
	// This allows the extension to store server reference if needed
	Configure(srv *Server) error
}

// MCPExtensionBuilder provides a fluent API for building MCP extensions
type MCPExtensionBuilder struct {
	name        string
	description string
	tools       []MCPTool
	resources   []MCPResource
	configFunc  func(*Server) error
}

// NewMCPExtension creates a new extension builder
func NewMCPExtension(name string) *MCPExtensionBuilder {
	return &MCPExtensionBuilder{
		name:      name,
		tools:     []MCPTool{},
		resources: []MCPResource{},
	}
}

func (b *MCPExtensionBuilder) WithDescription(desc string) *MCPExtensionBuilder {
	b.description = desc
	return b
}

func (b *MCPExtensionBuilder) WithTool(tool MCPTool) *MCPExtensionBuilder {
	b.tools = append(b.tools, tool)
	return b
}

func (b *MCPExtensionBuilder) WithResource(resource MCPResource) *MCPExtensionBuilder {
	b.resources = append(b.resources, resource)
	return b
}

func (b *MCPExtensionBuilder) WithConfiguration(fn func(*Server) error) *MCPExtensionBuilder {
	b.configFunc = fn
	return b
}

func (b *MCPExtensionBuilder) Build() MCPExtension {
	return &builtExtension{
		name:        b.name,
		description: b.description,
		tools:       b.tools,
		resources:   b.resources,
		configFunc:  b.configFunc,
	}
}

type builtExtension struct {
	name        string
	description string
	tools       []MCPTool
	resources   []MCPResource
	configFunc  func(*Server) error
}

func (e *builtExtension) Name() string        { return e.name }
func (e *builtExtension) Description() string { return e.description }
func (e *builtExtension) Tools() []MCPTool    { return e.tools }
func (e *builtExtension) Resources() []MCPResource { return e.resources }
func (e *builtExtension) Configure(srv *Server) error {
	if e.configFunc != nil {
		return e.configFunc(srv)
	}
	return nil
}

// RegisterMCPExtension registers all tools and resources from an extension
func (srv *Server) RegisterMCPExtension(ext MCPExtension) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}

	// Configure the extension with server reference
	if err := ext.Configure(srv); err != nil {
		return fmt.Errorf("extension configuration failed: %w", err)
	}

	// Register all tools
	for _, tool := range ext.Tools() {
		srv.mcpHandler.RegisterTool(tool)
	}

	// Register all resources  
	for _, resource := range ext.Resources() {
		srv.mcpHandler.RegisterResource(resource)
	}

	logger.Info("MCP extension registered",
		"name", ext.Name(),
		"tools", len(ext.Tools()),
		"resources", len(ext.Resources()),
	)

	return nil
}

// SimpleTool provides a simple way to create MCP tools without implementing the full interface
type SimpleTool struct {
	NameFunc        func() string
	DescriptionFunc func() string
	SchemaFunc      func() map[string]interface{}
	ExecuteFunc     func(map[string]interface{}) (interface{}, error)
}

func (t *SimpleTool) Name() string {
	if t.NameFunc != nil {
		return t.NameFunc()
	}
	return "unnamed_tool"
}

func (t *SimpleTool) Description() string {
	if t.DescriptionFunc != nil {
		return t.DescriptionFunc()
	}
	return "No description provided"
}

func (t *SimpleTool) Schema() map[string]interface{} {
	if t.SchemaFunc != nil {
		return t.SchemaFunc()
	}
	return map[string]interface{}{"type": "object"}
}

func (t *SimpleTool) Execute(params map[string]interface{}) (interface{}, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(params)
	}
	return nil, fmt.Errorf("execute function not implemented")
}

// SimpleResource provides a simple way to create MCP resources
type SimpleResource struct {
	URIFunc         func() string
	NameFunc        func() string
	DescriptionFunc func() string
	MimeTypeFunc    func() string
	ReadFunc        func() (interface{}, error)
	ListFunc        func() ([]string, error)
}

func (r *SimpleResource) URI() string {
	if r.URIFunc != nil {
		return r.URIFunc()
	}
	return "resource://unknown"
}

func (r *SimpleResource) Name() string {
	if r.NameFunc != nil {
		return r.NameFunc()
	}
	return "Unnamed Resource"
}

func (r *SimpleResource) Description() string {
	if r.DescriptionFunc != nil {
		return r.DescriptionFunc()
	}
	return "No description provided"
}

func (r *SimpleResource) MimeType() string {
	if r.MimeTypeFunc != nil {
		return r.MimeTypeFunc()
	}
	return "application/json"
}

func (r *SimpleResource) Read() (interface{}, error) {
	if r.ReadFunc != nil {
		return r.ReadFunc()
	}
	return nil, fmt.Errorf("read function not implemented")
}

func (r *SimpleResource) List() ([]string, error) {
	if r.ListFunc != nil {
		return r.ListFunc()
	}
	return []string{r.URI()}, nil
}

// ToolBuilder provides a fluent API for building tools
type ToolBuilder struct {
	name        string
	description string
	schema      map[string]interface{}
	executeFunc func(map[string]interface{}) (interface{}, error)
}

// NewTool creates a new tool builder
func NewTool(name string) *ToolBuilder {
	return &ToolBuilder{
		name:   name,
		schema: map[string]interface{}{"type": "object"},
	}
}

func (b *ToolBuilder) WithDescription(desc string) *ToolBuilder {
	b.description = desc
	return b
}

func (b *ToolBuilder) WithParameter(name, paramType, description string, required bool) *ToolBuilder {
	if b.schema["properties"] == nil {
		b.schema["properties"] = map[string]interface{}{}
	}
	props := b.schema["properties"].(map[string]interface{})
	props[name] = map[string]interface{}{
		"type":        paramType,
		"description": description,
	}
	
	if required {
		if b.schema["required"] == nil {
			b.schema["required"] = []string{}
		}
		b.schema["required"] = append(b.schema["required"].([]string), name)
	}
	
	return b
}

func (b *ToolBuilder) WithExecute(fn func(map[string]interface{}) (interface{}, error)) *ToolBuilder {
	b.executeFunc = fn
	return b
}

func (b *ToolBuilder) Build() MCPTool {
	return &SimpleTool{
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		SchemaFunc:      func() map[string]interface{} { return b.schema },
		ExecuteFunc:     b.executeFunc,
	}
}

// ResourceBuilder provides a fluent API for building resources
type ResourceBuilder struct {
	uri         string
	name        string
	description string
	mimeType    string
	readFunc    func() (interface{}, error)
}

// NewResource creates a new resource builder
func NewResource(uri string) *ResourceBuilder {
	return &ResourceBuilder{
		uri:      uri,
		mimeType: "application/json",
	}
}

func (b *ResourceBuilder) WithName(name string) *ResourceBuilder {
	b.name = name
	return b
}

func (b *ResourceBuilder) WithDescription(desc string) *ResourceBuilder {
	b.description = desc
	return b
}

func (b *ResourceBuilder) WithMimeType(mimeType string) *ResourceBuilder {
	b.mimeType = mimeType
	return b
}

func (b *ResourceBuilder) WithRead(fn func() (interface{}, error)) *ResourceBuilder {
	b.readFunc = fn
	return b
}

func (b *ResourceBuilder) Build() MCPResource {
	return &SimpleResource{
		URIFunc:         func() string { return b.uri },
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		MimeTypeFunc:    func() string { return b.mimeType },
		ReadFunc:        b.readFunc,
		ListFunc:        func() ([]string, error) { return []string{b.uri}, nil },
	}
}

// Example: Creating a custom e-commerce extension
func ExampleECommerceExtension() MCPExtension {
	return NewMCPExtension("ecommerce").
		WithDescription("E-commerce management tools and resources").
		WithTool(
			NewTool("manage_products").
				WithDescription("Add, update, or remove products").
				WithParameter("action", "string", "Action to perform", true).
				WithParameter("product_id", "string", "Product ID", false).
				WithParameter("data", "object", "Product data", false).
				WithExecute(func(params map[string]interface{}) (interface{}, error) {
					action := params["action"].(string)
					switch action {
					case "list":
						// Return product list
						return map[string]interface{}{"products": []interface{}{}}, nil
					case "add":
						// Add product logic
						return map[string]interface{}{"status": "product_added"}, nil
					default:
						return nil, fmt.Errorf("unknown action: %s", action)
					}
				}).
				Build(),
		).
		WithResource(
			NewResource("products://catalog/all").
				WithName("Product Catalog").
				WithDescription("All products in the catalog").
				WithRead(func() (interface{}, error) {
					// Fetch from database
					return map[string]interface{}{
						"products": []interface{}{
							// Product data
						},
					}, nil
				}).
				Build(),
		).
		WithConfiguration(func(s *Server) error {
			// Store server reference, initialize database, etc.
			return nil
		}).
		Build()
}