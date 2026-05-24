package mcp

import (
	"cmp"
	"net/http"
	"strings"
)

// DiscoveryPolicy controls how MCP tools and resources are exposed via the
// discovery endpoints (/.well-known/mcp.json and /mcp/discover).
type DiscoveryPolicy int

const (
	// DiscoveryPublic shows all discoverable tools/resources (default).
	DiscoveryPublic DiscoveryPolicy = iota
	// DiscoveryCount only exposes counts, not names.
	DiscoveryCount
	// DiscoveryAuthenticated returns the full list only when the request
	// carries an Authorization header.
	DiscoveryAuthenticated
	// DiscoveryNone hides all tool/resource information.
	DiscoveryNone
)

// DiscoveryInfo describes the MCP endpoints surfaced by the discovery API.
type DiscoveryInfo struct {
	Version      string            `json:"version"`
	Transports   []TransportInfo   `json:"transports"`
	Endpoints    map[string]string `json:"endpoints"`
	Capabilities map[string]any    `json:"capabilities,omitempty"`
}

// TransportInfo describes one available transport mechanism.
type TransportInfo struct {
	Type        string            `json:"type"`
	Endpoint    string            `json:"endpoint"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// DiscoveryConfig groups the inputs needed to build a DiscoveryInfo response.
type DiscoveryConfig struct {
	// MCPEndpoint is the URL path the MCP handler is mounted on (e.g. "/mcp").
	MCPEndpoint string
	// DefaultAddr is used to derive the base URL when the request has no Host header.
	DefaultAddr string
	// Transport identifies the transport in use (HTTP / Stdio).
	Transport TransportType
	// Policy controls discovery list visibility.
	Policy DiscoveryPolicy
	// Dev indicates the server is running in MCP developer mode and may
	// therefore expose tools that would otherwise be hidden.
	Dev bool
	// Filter, if non-nil, makes the final decision per tool. It overrides the
	// default rules entirely.
	Filter func(toolName string, r *http.Request) bool
}

// BuildDiscoveryInfo constructs the discovery payload based on the supplied
// configuration and request. The caller is responsible for marshalling and
// writing the response.
func (h *Handler) BuildDiscoveryInfo(r *http.Request, cfg DiscoveryConfig) DiscoveryInfo {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := cmp.Or(r.Host, "localhost"+cfg.DefaultAddr)
	baseURL := scheme + "://" + host
	mcpEndpoint := baseURL + cfg.MCPEndpoint

	info := DiscoveryInfo{
		Version: h.ProtocolVersion(),
		Transports: []TransportInfo{
			{
				Type:        "http",
				Endpoint:    mcpEndpoint,
				Description: "Standard HTTP POST requests with JSON-RPC 2.0",
				Headers:     map[string]string{"Content-Type": "application/json"},
			},
			{
				Type:        "sse",
				Endpoint:    mcpEndpoint,
				Description: "Server-Sent Events for real-time communication",
				Headers:     map[string]string{"Accept": "text/event-stream"},
			},
		},
		Endpoints: map[string]string{
			"mcp":        mcpEndpoint,
			"initialize": mcpEndpoint,
			"tools":      mcpEndpoint,
			"resources":  mcpEndpoint,
		},
	}

	tools := h.RegisteredTools()
	resources := h.RegisteredResources()
	resourceTemplates := h.RegisteredResourceTemplates()

	toolCapability := map[string]any{
		"supported": true,
		"count":     len(tools),
	}
	if shouldIncludeToolList(cfg.Policy, r) {
		filteredTools := make([]string, 0, len(tools))
		for _, toolName := range tools {
			if h.shouldExposeToolInDiscovery(toolName, r, cfg) {
				filteredTools = append(filteredTools, toolName)
			}
		}
		if len(filteredTools) > 0 {
			toolCapability["available"] = filteredTools
		}
	}

	resourceCapability := map[string]any{
		"supported": true,
		"count":     len(resources),
		"subscribe": h.hasSubscribableResourceTemplates(),
	}
	if shouldIncludeToolList(cfg.Policy, r) {
		resourceCapability["available"] = resources
	}
	if len(resourceTemplates) > 0 {
		resourceTemplateCapability := map[string]any{
			"supported": true,
			"count":     len(resourceTemplates),
		}
		if shouldIncludeToolList(cfg.Policy, r) {
			resourceTemplateCapability["available"] = resourceTemplates
		}
		resourceCapability["templates"] = resourceTemplateCapability
	}

	info.Capabilities = map[string]any{
		"tools":     toolCapability,
		"resources": resourceCapability,
		"sse": map[string]any{
			"enabled":       true,
			"endpoint":      "same",
			"headerRouting": true,
		},
	}

	if cfg.Transport == StdioTransport {
		info.Capabilities["stdio"] = map[string]any{
			"supported": true,
		}
	}

	return info
}

// shouldIncludeToolList reports whether the discovery payload should include
// the full list of tool/resource names.
func shouldIncludeToolList(policy DiscoveryPolicy, r *http.Request) bool {
	switch policy {
	case DiscoveryNone, DiscoveryCount:
		return false
	case DiscoveryAuthenticated:
		return r.Header.Get("Authorization") != ""
	case DiscoveryPublic:
		return true
	default:
		return true
	}
}

// shouldExposeToolInDiscovery applies the policy + custom filter to a single
// tool name. By the time it runs, shouldIncludeToolList has already gated
// the policy — DiscoveryNone and DiscoveryCount can never reach this
// function, so this only needs to handle the policies that allow listing.
//
// A tool may opt out of discovery for itself by implementing
// `interface{ IsDiscoverable() bool }`. That is the only mechanism — the
// protocol package does not pattern-match tool names, since "debug"/"admin"
// substring rules would silently hide legitimate user tools (e.g. "tax_admin_lookup")
// and leak builtin-namespace knowledge upward into pkg/mcp. Names beginning
// with `_` or `internal_` remain a convention for explicitly hidden tools.
func (h *Handler) shouldExposeToolInDiscovery(toolName string, r *http.Request, cfg DiscoveryConfig) bool {
	if cfg.Filter != nil {
		return cfg.Filter(toolName, r)
	}

	if cfg.Policy == DiscoveryAuthenticated && r.Header.Get("Authorization") == "" {
		return false
	}

	if strings.HasPrefix(toolName, "internal_") || strings.HasPrefix(toolName, "_") {
		return false
	}

	if tool, exists := h.Tool(toolName); exists {
		if discoverable, ok := tool.(interface{ IsDiscoverable() bool }); ok {
			return discoverable.IsDiscoverable()
		}
	}
	return true
}
