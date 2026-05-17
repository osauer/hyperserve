package mcp

// Namespace represents a named collection of MCP tools and resources.
type Namespace struct {
	Name      string
	Tools     []Tool
	Resources []Resource
}

// NamespaceConfig configures a Namespace.
type NamespaceConfig func(*Namespace)

// WithNamespaceTools adds tools to a namespace.
func WithNamespaceTools(tools ...Tool) NamespaceConfig {
	return func(ns *Namespace) {
		ns.Tools = append(ns.Tools, tools...)
	}
}

// WithNamespaceResources adds resources to a namespace.
func WithNamespaceResources(resources ...Resource) NamespaceConfig {
	return func(ns *Namespace) {
		ns.Resources = append(ns.Resources, resources...)
	}
}
