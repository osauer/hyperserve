package mcp

import "fmt"

// SimpleTool is a Tool implementation backed by function fields. Use it when
// you want a one-off tool without defining a new struct.
type SimpleTool struct {
	NameFunc        func() string
	DescriptionFunc func() string
	SchemaFunc      func() map[string]any
	ExecuteFunc     func(map[string]any) (any, error)
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

func (t *SimpleTool) Schema() map[string]any {
	if t.SchemaFunc != nil {
		return t.SchemaFunc()
	}
	return map[string]any{"type": "object"}
}

func (t *SimpleTool) Execute(params map[string]any) (any, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(params)
	}
	return nil, fmt.Errorf("execute function not implemented")
}

// SimpleResource is a Resource implementation backed by function fields.
type SimpleResource struct {
	URIFunc         func() string
	NameFunc        func() string
	DescriptionFunc func() string
	MimeTypeFunc    func() string
	ReadFunc        func() (any, error)
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

func (r *SimpleResource) Read() (any, error) {
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

// ToolBuilder provides a fluent API for building Tools.
type ToolBuilder struct {
	name        string
	description string
	schema      map[string]any
	executeFunc func(map[string]any) (any, error)
}

// NewTool creates a new tool builder.
func NewTool(name string) *ToolBuilder {
	return &ToolBuilder{
		name:   name,
		schema: map[string]any{"type": "object"},
	}
}

func (b *ToolBuilder) WithDescription(desc string) *ToolBuilder {
	b.description = desc
	return b
}

func (b *ToolBuilder) WithParameter(name, paramType, description string, required bool) *ToolBuilder {
	if b.schema["properties"] == nil {
		b.schema["properties"] = map[string]any{}
	}
	props := b.schema["properties"].(map[string]any)
	props[name] = map[string]any{
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

func (b *ToolBuilder) WithExecute(fn func(map[string]any) (any, error)) *ToolBuilder {
	b.executeFunc = fn
	return b
}

func (b *ToolBuilder) Build() Tool {
	return &SimpleTool{
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		SchemaFunc:      func() map[string]any { return b.schema },
		ExecuteFunc:     b.executeFunc,
	}
}

// ResourceBuilder provides a fluent API for building Resources.
type ResourceBuilder struct {
	uri         string
	name        string
	description string
	mimeType    string
	readFunc    func() (any, error)
}

// NewResource creates a new resource builder.
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

func (b *ResourceBuilder) WithRead(fn func() (any, error)) *ResourceBuilder {
	b.readFunc = fn
	return b
}

func (b *ResourceBuilder) Build() Resource {
	return &SimpleResource{
		URIFunc:         func() string { return b.uri },
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		MimeTypeFunc:    func() string { return b.mimeType },
		ReadFunc:        b.readFunc,
		ListFunc:        func() ([]string, error) { return []string{b.uri}, nil },
	}
}

// Extension represents a collection of MCP tools and resources that can be
// registered as a group. It's the lightweight way to package related
// functionality together.
type Extension interface {
	// Name returns the extension name (e.g., "e-commerce", "blog").
	Name() string
	// Description returns a human-readable description.
	Description() string
	// Tools returns the tools provided by this extension.
	Tools() []Tool
	// Resources returns the resources provided by this extension.
	Resources() []Resource
	// Configure runs before registration with the supplied handler. Use it to
	// wire the extension up to the handler (e.g. capture a reference, register
	// extra namespaces).
	Configure(h *Handler) error
}

// ExtensionBuilder provides a fluent API for building Extensions.
type ExtensionBuilder struct {
	name        string
	description string
	tools       []Tool
	resources   []Resource
	configFunc  func(*Handler) error
}

// NewExtension creates a new extension builder.
func NewExtension(name string) *ExtensionBuilder {
	return &ExtensionBuilder{name: name}
}

func (b *ExtensionBuilder) WithDescription(desc string) *ExtensionBuilder {
	b.description = desc
	return b
}

func (b *ExtensionBuilder) WithTool(tool Tool) *ExtensionBuilder {
	b.tools = append(b.tools, tool)
	return b
}

func (b *ExtensionBuilder) WithResource(resource Resource) *ExtensionBuilder {
	b.resources = append(b.resources, resource)
	return b
}

func (b *ExtensionBuilder) WithConfiguration(fn func(*Handler) error) *ExtensionBuilder {
	b.configFunc = fn
	return b
}

func (b *ExtensionBuilder) Build() Extension {
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
	tools       []Tool
	resources   []Resource
	configFunc  func(*Handler) error
}

func (e *builtExtension) Name() string          { return e.name }
func (e *builtExtension) Description() string   { return e.description }
func (e *builtExtension) Tools() []Tool         { return e.tools }
func (e *builtExtension) Resources() []Resource { return e.resources }
func (e *builtExtension) Configure(h *Handler) error {
	if e.configFunc != nil {
		return e.configFunc(h)
	}
	return nil
}

// RegisterExtension wires all of an Extension's tools and resources into the
// handler. It calls Configure(h) first so the extension can hook in.
func (h *Handler) RegisterExtension(ext Extension) error {
	if err := ext.Configure(h); err != nil {
		return fmt.Errorf("extension configuration failed: %w", err)
	}
	for _, tool := range ext.Tools() {
		h.RegisterTool(tool)
	}
	for _, resource := range ext.Resources() {
		h.RegisterResource(resource)
	}
	h.logger.Info("MCP extension registered",
		"name", ext.Name(),
		"tools", len(ext.Tools()),
		"resources", len(ext.Resources()),
	)
	return nil
}
