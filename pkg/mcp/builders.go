package mcp

import "fmt"

// simpleTool is the function-field Tool implementation that ToolBuilder
// assembles in Build(). It is intentionally unexported: every consumer in
// this repo, in examples, and in the tests reaches Tool through the builder
// path or through NewTypedTool — nobody constructs the struct directly.
// Exposing it widens the public surface without buying anything.
type simpleTool struct {
	NameFunc        func() string
	DescriptionFunc func() string
	SchemaFunc      func() map[string]any
	ExecuteFunc     func(map[string]any) (any, error)
}

func (t *simpleTool) Name() string {
	if t.NameFunc != nil {
		return t.NameFunc()
	}
	return "unnamed_tool"
}

func (t *simpleTool) Description() string {
	if t.DescriptionFunc != nil {
		return t.DescriptionFunc()
	}
	return "No description provided"
}

func (t *simpleTool) Schema() map[string]any {
	if t.SchemaFunc != nil {
		return t.SchemaFunc()
	}
	return map[string]any{"type": "object"}
}

func (t *simpleTool) Execute(params map[string]any) (any, error) {
	if t.ExecuteFunc != nil {
		return t.ExecuteFunc(params)
	}
	return nil, fmt.Errorf("execute function not implemented")
}

// ToolBuilder provides a fluent API for building Tools with a hand-tuned
// schema. Prefer NewTypedTool[In, Out] when the input shape is a struct;
// reach for this builder when you need a schema the type system can't
// describe (oneOf, polymorphic shapes, schema-from-JSON-file, etc.).
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
	return &simpleTool{
		NameFunc:        func() string { return b.name },
		DescriptionFunc: func() string { return b.description },
		SchemaFunc:      func() map[string]any { return b.schema },
		ExecuteFunc:     b.executeFunc,
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

// ResourceTemplateExtension is implemented by extensions that also provide
// parameterized resource templates. It is optional so existing Extension
// implementations remain source-compatible.
type ResourceTemplateExtension interface {
	ResourceTemplates() []ResourceTemplate
}

// ExtensionBuilder provides a fluent API for building Extensions.
type ExtensionBuilder struct {
	name              string
	description       string
	tools             []Tool
	resources         []Resource
	resourceTemplates []ResourceTemplate
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

func (b *ExtensionBuilder) WithResourceTemplate(template ResourceTemplate) *ExtensionBuilder {
	b.resourceTemplates = append(b.resourceTemplates, template)
	return b
}

func (b *ExtensionBuilder) Build() Extension {
	return &builtExtension{
		name:              b.name,
		description:       b.description,
		tools:             b.tools,
		resources:         b.resources,
		resourceTemplates: b.resourceTemplates,
	}
}

type builtExtension struct {
	name              string
	description       string
	tools             []Tool
	resources         []Resource
	resourceTemplates []ResourceTemplate
}

func (e *builtExtension) Name() string                          { return e.name }
func (e *builtExtension) Description() string                   { return e.description }
func (e *builtExtension) Tools() []Tool                         { return e.tools }
func (e *builtExtension) Resources() []Resource                 { return e.resources }
func (e *builtExtension) ResourceTemplates() []ResourceTemplate { return e.resourceTemplates }
func (e *builtExtension) Configure(h *Handler) error            { return nil }

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
	templateCount := 0
	if templateExt, ok := ext.(ResourceTemplateExtension); ok {
		templates := templateExt.ResourceTemplates()
		templateCount = len(templates)
		for _, template := range templates {
			h.RegisterResourceTemplate(template)
		}
	}
	h.logger.Info("MCP extension registered",
		"name", ext.Name(),
		"tools", len(ext.Tools()),
		"resources", len(ext.Resources()),
		"resourceTemplates", templateCount,
	)
	return nil
}
