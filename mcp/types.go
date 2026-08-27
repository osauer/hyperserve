// Package mcp implements the Model Context Protocol (MCP) over JSON-RPC 2.0.
//
// The package is transport-agnostic: HTTP, Server-Sent Events (SSE), and stdio
// transports are all provided, but the protocol Handler does not depend on any
// specific transport. The HTTP-server integration lives in the root
// hyperserve package; built-in tools and resources that need a
// *hyperserve.Server live in mcp/builtin.
package mcp

import (
	"context"
	"log/slog"
	"time"
)

const (
	// DefaultProtocolVersion is the initialize-era MCP protocol version
	// advertised by default. It remains the default for compatibility with
	// existing 2025 clients while Streamable HTTP negotiates the current
	// revision independently on each request.
	DefaultProtocolVersion = "2025-11-25"

	// StreamableHTTPProtocolVersion is the current stateless Streamable HTTP
	// protocol revision supported by Handler.ServeHTTP.
	StreamableHTTPProtocolVersion = "2026-07-28"
)

// ProtocolVersion is kept as a compatibility alias for callers that used the
// old package constant. New code should prefer DefaultProtocolVersion or a
// Handler's configured ProtocolVersion.
const ProtocolVersion = DefaultProtocolVersion

// TransportType identifies the kind of transport used for MCP communication.
type TransportType int

const (
	// HTTPTransport selects HTTP-based MCP communication.
	HTTPTransport TransportType = iota
	// StdioTransport selects stdin/stdout MCP communication.
	StdioTransport
)

// Capabilities represents the server's advertised MCP capabilities.
// Add fields here only when the corresponding capability is actually wired
// in Handler.Capabilities() and exercised on the wire; advertising an
// unsupported capability is worse than omitting it.
type Capabilities struct {
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	SSE       *SSECapability       `json:"sse,omitempty"`
}

// ResourcesCapability represents the server's resource management capabilities.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability represents the server's tool execution capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SSECapability represents the server's Server-Sent Events capability.
type SSECapability struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	HeaderRouting bool   `json:"headerRouting"`
}

// ServerInfo identifies an MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientInfo identifies an MCP client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool defines the interface for Model Context Protocol tools.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(params map[string]any) (any, error)
}

// Resource defines the interface for Model Context Protocol resources.
type Resource interface {
	URI() string
	Name() string
	Description() string
	MimeType() string
	Read() (any, error)
	List() ([]string, error)
}

// ResourceTemplate defines a parameterized family of MCP resources.
type ResourceTemplate interface {
	URITemplate() string
	Name() string
	Description() string
	MimeType() string
	Match(uri string) (params map[string]string, ok bool)
	Read(ctx context.Context, uri string, params map[string]string) (any, error)
}

// SubscribableResourceTemplate extends ResourceTemplate with live update
// subscriptions. Subscribe should block until ctx is canceled or the
// subscription ends.
type SubscribableResourceTemplate interface {
	ResourceTemplate
	Subscribe(ctx context.Context, uri string, params map[string]string, emit ResourceEmitter) error
}

// ResourceEmitter emits resource update notifications for an active
// subscription. MCP update notifications are invalidation signals: clients
// should call resources/read to fetch the latest content.
type ResourceEmitter interface {
	Update(uri string) error
}

// CacheableResource is an optional extension for resources whose read result
// is safe to reuse for a bounded time. Resources are uncached by default so
// live observability views (health, metrics, logs, route lists) never return
// stale data unless they explicitly opt in.
type CacheableResource interface {
	Resource
	ResourceCacheTTL() time.Duration
}

// InitializeParams is the parameter struct for the "initialize" method.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    any        `json:"capabilities"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// InitializeResult is the result returned by the "initialize" method.
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

// ResourceReadParams is the parameter struct for "resources/read".
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceTemplateInfo describes a resource template in
// resources/templates/list responses.
type ResourceTemplateInfo struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ToolCallParams is the parameter struct for "tools/call".
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolInfo describes a tool in tools/list responses. OutputSchema is the
// `outputSchema` field added in the MCP spec revision 2025-06-18; tools
// that implement ToolWithOutputSchema populate it via the handler's
// tools/list path, others omit it.
type ToolInfo struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

// ResourceInfo describes a resource in resources/list responses.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// ResourceContent represents the content body of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     any    `json:"text"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// logger is the fallback for directly constructed MCP handlers. A Handler
// receives its own logger at construction and a HyperServe Server injects its
// instance logger, so this fallback is not mutated by server configuration.
var logger = slog.Default()
