package mcp

import (
	"fmt"
	"maps"
	"slices"
)

type simpleTool struct {
	name        string
	description string
	schema      map[string]any
	execute     func(map[string]any) (any, error)
}

func (t *simpleTool) Name() string           { return t.name }
func (t *simpleTool) Description() string    { return t.description }
func (t *simpleTool) Schema() map[string]any { return t.schema }

func (t *simpleTool) Execute(params map[string]any) (any, error) {
	if t.execute != nil {
		return t.execute(params)
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

// Build snapshots the tool's metadata, parameter schema, and execution function.
// Later changes to the builder do not alter the returned tool.
func (b *ToolBuilder) Build() Tool {
	schema := maps.Clone(b.schema)
	if properties, ok := b.schema["properties"].(map[string]any); ok {
		copied := make(map[string]any, len(properties))
		for name, property := range properties {
			copied[name] = maps.Clone(property.(map[string]any))
		}
		schema["properties"] = copied
	}
	if required, ok := b.schema["required"].([]string); ok {
		schema["required"] = slices.Clone(required)
	}
	return &simpleTool{
		name:        b.name,
		description: b.description,
		schema:      schema,
		execute:     b.executeFunc,
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
