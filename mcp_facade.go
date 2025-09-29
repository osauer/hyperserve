package hyperserve

import pkgserver "github.com/osauer/hyperserve/pkg/server"

type (
	MCPTransportType      = pkgserver.MCPTransportType
	MCPTransport          = pkgserver.MCPTransport
	MCPTransportConfig    = pkgserver.MCPTransportConfig
	MCPTool               = pkgserver.MCPTool
	MCPResource           = pkgserver.MCPResource
	MCPToolWithContext    = pkgserver.MCPToolWithContext
	MCPCapabilities       = pkgserver.MCPCapabilities
	LoggingCapability     = pkgserver.LoggingCapability
	PromptsCapability     = pkgserver.PromptsCapability
	ResourcesCapability   = pkgserver.ResourcesCapability
	ToolsCapability       = pkgserver.ToolsCapability
	SamplingCapability    = pkgserver.SamplingCapability
	SSECapability         = pkgserver.SSECapability
	MCPServerInfo         = pkgserver.MCPServerInfo
	MCPClientInfo         = pkgserver.MCPClientInfo
	MCPNamespace          = pkgserver.MCPNamespace
	MCPNamespaceConfig    = pkgserver.MCPNamespaceConfig
	MCPHandler            = pkgserver.MCPHandler
	MCPInitializeParams   = pkgserver.MCPInitializeParams
	MCPInitializeResult   = pkgserver.MCPInitializeResult
	MCPResourceReadParams = pkgserver.MCPResourceReadParams
	MCPToolCallParams     = pkgserver.MCPToolCallParams
	MCPToolInfo           = pkgserver.MCPToolInfo
	MCPResourceInfo       = pkgserver.MCPResourceInfo
	MCPResourceContent    = pkgserver.MCPResourceContent
	MCPToolResult         = pkgserver.MCPToolResult
	MCPMetrics            = pkgserver.MCPMetrics
	MCPExtension          = pkgserver.MCPExtension
	MCPExtensionBuilder   = pkgserver.MCPExtensionBuilder
	SimpleTool            = pkgserver.SimpleTool
	SimpleResource        = pkgserver.SimpleResource
	ToolBuilder           = pkgserver.ToolBuilder
	ResourceBuilder       = pkgserver.ResourceBuilder
	DiscoveryPolicy       = pkgserver.DiscoveryPolicy
	MCPDiscoveryInfo      = pkgserver.MCPDiscoveryInfo
)

const (
	DiscoveryPublic        = pkgserver.DiscoveryPublic
	DiscoveryCount         = pkgserver.DiscoveryCount
	DiscoveryAuthenticated = pkgserver.DiscoveryAuthenticated
	DiscoveryNone          = pkgserver.DiscoveryNone
)

func MCPOverHTTP(endpoint string) MCPTransportConfig { return pkgserver.MCPOverHTTP(endpoint) }

func MCPOverStdio() MCPTransportConfig { return pkgserver.MCPOverStdio() }

func MCPDev() MCPTransportConfig { return pkgserver.MCPDev() }

func MCPObservability() MCPTransportConfig { return pkgserver.MCPObservability() }

func WithNamespaceTools(tools ...MCPTool) MCPNamespaceConfig {
	return pkgserver.WithNamespaceTools(tools...)
}

func WithNamespaceResources(resources ...MCPResource) MCPNamespaceConfig {
	return pkgserver.WithNamespaceResources(resources...)
}

func NewMCPHandler(serverInfo MCPServerInfo) *MCPHandler {
	return pkgserver.NewMCPHandler(serverInfo)
}

func NewMCPExtension(name string) *MCPExtensionBuilder {
	return pkgserver.NewMCPExtension(name)
}

func NewTool(name string) *ToolBuilder { return pkgserver.NewTool(name) }

func NewResource(uri string) *ResourceBuilder { return pkgserver.NewResource(uri) }

func ExampleECommerceExtension() MCPExtension { return pkgserver.ExampleECommerceExtension() }
