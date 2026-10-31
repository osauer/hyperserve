package hyperserve

// MCP (Model Context Protocol) glue.
//
// The MCP protocol itself lives in github.com/osauer/hyperserve/v2/mcp. This
// file wires *Server up to *mcp.Handler: server options that flip MCP modes,
// the discovery endpoint registration, and the thin Register* helpers that
// delegate into the handler.

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/osauer/hyperserve/v2/mcp"
)

// =============================================================================
// Preset registration hooks
// =============================================================================
//
// The root package cannot import mcp/builtin (that would cycle, since builtin
// imports the root package). Instead, builtin's init installs the hooks below
// via SetBuiltinPresetHooks. When a hook is non-nil, New invokes it after
// the MCP handler is created.
//
// Applications that enable builtins must explicitly blank-import mcp/builtin;
// MCPDev and MCPObservability do not import it for them.

var (
	builtinToolsHook             func(*Server)
	builtinObservabilityHook     func(*Server)
	builtinDeveloperHook         func(*Server)
	builtinStandardResourcesHook func(*Server)
)

// SetBuiltinPresetHooks lets mcp/builtin (and only it, in practice) wire
// itself into the auto-registration flow used by New when MCP is
// enabled. Pass nil for any preset you don't implement.
func SetBuiltinPresetHooks(tools, standardResources, observability, developer func(*Server)) {
	builtinToolsHook = tools
	builtinStandardResourcesHook = standardResources
	builtinObservabilityHook = observability
	builtinDeveloperHook = developer
}

// MCPDev configures MCP with developer tools for local development.
//
// SECURITY WARNING: Only use in development environments. It exposes runtime
// status, registered routes, middleware layout, and development logs.
//
// Tools provided:
//   - mcp__hyperserve__server_control
//   - mcp__hyperserve__route_inspector
//   - mcp__hyperserve__dev_guide
//
// Resources provided:
//   - logs://server/stream, routes://server/all
func MCPDev() mcp.TransportConfig { return mcp.WithDeveloperMode() }

// MCPObservability configures MCP with read-only observability resources.
// It does not authenticate or authorize requests; applications must protect the
// MCP endpoint or keep it on a private listener. The preset provides:
//   - config://server/current (sanitized server config, no secrets)
//   - health://server/status (uptime and health metrics)
//   - logs://server/recent (circular buffer of recent log entries)
func MCPObservability() mcp.TransportConfig { return mcp.WithObservabilityMode() }

// MCPHandler returns the MCP handler attached to this server, or nil if MCP
// is not enabled.
func (srv *Server) MCPHandler() *mcp.Handler { return srv.mcpHandler }

// MCPEnabled reports whether MCP support has been initialized for this server.
func (srv *Server) MCPEnabled() bool {
	return srv.options.MCPEnabled && srv.mcpHandler != nil
}

// RegisterMCPTool registers a custom MCP tool. Must be called after server
// creation but before Run().
func (srv *Server) RegisterMCPTool(tool mcp.Tool) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	srv.mcpHandler.RegisterTool(tool)
	return nil
}

// RegisterMCPResource registers a custom MCP resource.
func (srv *Server) RegisterMCPResource(resource mcp.Resource) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	srv.mcpHandler.RegisterResource(resource)
	return nil
}

// RegisterMCPResourceTemplate registers a custom MCP resource template.
func (srv *Server) RegisterMCPResourceTemplate(template mcp.ResourceTemplate) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	srv.mcpHandler.RegisterResourceTemplate(template)
	return nil
}

// RegisterMCPNamespace registers an entire MCP namespace with its tools and
// resources. Per-tool/per-resource namespace registration goes through this
// path — callers that need a single tool in a namespace pass it inside a
// NamespaceConfig rather than reaching for two separate helpers.
func (srv *Server) RegisterMCPNamespace(name string, configs ...mcp.NamespaceConfig) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	return srv.mcpHandler.RegisterNamespace(name, configs...)
}

// RegisterMCPExtension registers all tools and resources from an extension.
func (srv *Server) RegisterMCPExtension(ext mcp.Extension) error {
	if !srv.MCPEnabled() {
		return fmt.Errorf("MCP is not enabled on this server")
	}
	return srv.mcpHandler.RegisterExtension(ext)
}

// =============================================================================
// Discovery endpoints
// =============================================================================

// setupDiscoveryEndpoints registers the discovery endpoints for AI clients.
// Both /.well-known/mcp.json and <MCPEndpoint>/discover return the same
// DiscoveryInfo payload, computed per-request from the handler state.
func (srv *Server) setupDiscoveryEndpoints() {
	if !srv.MCPEnabled() {
		return
	}

	cfg := func() mcp.DiscoveryConfig {
		return mcp.DiscoveryConfig{
			MCPEndpoint: srv.options.MCPEndpoint,
			DefaultAddr: srv.options.Addr,
			Transport:   srv.options.MCPTransport,
			Policy:      srv.options.MCPDiscoveryPolicy,
			Filter:      srv.options.MCPDiscoveryFilter,
		}
	}

	// setDiscoveryCacheHeaders writes cache-control headers that respect the
	// discovery policy. Under DiscoveryAuthenticated the response body varies
	// on the Authorization header, so the response must not be stored by
	// shared caches (CDN/reverse proxy) keyed on URL alone — otherwise an
	// authenticated response replays to anonymous clients. Vary: Authorization
	// is set unconditionally as defense in depth for caches that honor it.
	setDiscoveryCacheHeaders := func(w http.ResponseWriter) {
		w.Header().Set("Vary", "Authorization")
		if srv.options.MCPDiscoveryPolicy == mcp.DiscoveryAuthenticated {
			w.Header().Set("Cache-Control", "private, max-age=60")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
	}

	writeDiscovery := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		info := srv.mcpHandler.BuildDiscoveryInfo(r, cfg())
		w.Header().Set("Content-Type", "application/json")
		setDiscoveryCacheHeaders(w)
		if err := json.NewEncoder(w).Encode(info); err != nil {
			srv.logger.Error("Failed to encode discovery info", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}

	srv.registerRoute(mcpDiscoveryEndpoint)
	srv.mux.HandleFunc(mcpDiscoveryEndpoint, writeDiscovery)

	srv.registerRoute(srv.options.MCPEndpoint + "/discover")
	srv.mux.HandleFunc(srv.options.MCPEndpoint+"/discover", writeDiscovery)

	srv.logger.Debug("MCP discovery endpoints registered",
		"endpoints", []string{mcpDiscoveryEndpoint, srv.options.MCPEndpoint + "/discover"})
}
