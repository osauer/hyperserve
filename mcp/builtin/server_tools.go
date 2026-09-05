package builtin

import (
	"fmt"
	"slices"
	"strings"

	"github.com/osauer/hyperserve/v2"
)

const serverControlGetStatus = "get_status"

func serverControlActions() []string {
	return []string{serverControlGetStatus}
}

// ServerControlTool provides read-only server status for development.
type ServerControlTool struct {
	server *hyperserve.Server
}

// NewServerControlTool creates a ServerControlTool.
func NewServerControlTool(srv *hyperserve.Server) *ServerControlTool {
	return &ServerControlTool{server: srv}
}

func (t *ServerControlTool) Name() string { return "server_control" }

func (t *ServerControlTool) Description() string {
	return "Inspect the running HyperServe instance. Action: get_status."
}

func (t *ServerControlTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        serverControlActions(),
				"description": "Return current server health and configuration status.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ServerControlTool) Execute(params map[string]any) (any, error) {
	action, ok := params["action"].(string)
	if !ok {
		return nil, fmt.Errorf("action is required")
	}
	switch action {
	case serverControlGetStatus:
		return map[string]any{
			"running":   t.server.IsRunning(),
			"ready":     t.server.IsReady(),
			"uptime":    serverUptime(t.server).String(),
			"log_level": t.server.Options().LogLevel,
			"addr":      t.server.Options().Addr,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// RouteInspectorTool provides route introspection for development.
type RouteInspectorTool struct {
	server *hyperserve.Server
}

// NewRouteInspectorTool creates a RouteInspectorTool.
func NewRouteInspectorTool(srv *hyperserve.Server) *RouteInspectorTool {
	return &RouteInspectorTool{server: srv}
}

func (t *RouteInspectorTool) Name() string { return "route_inspector" }

func (t *RouteInspectorTool) Description() string {
	return "List registered HTTP routes and middleware registration scopes. The pattern parameter filters routes (e.g., '/api' shows API routes)."
}

func (t *RouteInspectorTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Optional pattern to filter routes (e.g., '/api' to show only API routes, '/health' for health check endpoints)",
			},
			"include_middleware": map[string]any{
				"type":        "boolean",
				"description": "Include the main server's middleware registration scopes and counts (default: true). Registrations are not a per-request execution trace.",
				"default":     true,
			},
		},
	}
}

func (t *RouteInspectorTool) Execute(params map[string]any) (any, error) {
	pattern, _ := params["pattern"].(string)
	includeMiddleware, _ := params["include_middleware"].(bool)
	if !includeMiddleware && params["include_middleware"] == nil {
		includeMiddleware = true
	}

	routes := []map[string]any{}
	for _, route := range t.server.RegisteredRoutes() {
		routePattern, methods := splitServeMuxPattern(route)
		if pattern != "" && !strings.Contains(routePattern, pattern) {
			continue
		}
		routes = append(routes, makeRouteInfo(routePattern, "main", methods))
	}

	// Health routes belong to a separate listener and have no main middleware.
	if t.server.Options().RunHealthServer {
		for _, route := range []string{"/healthz", "/readyz", "/livez"} {
			routes = ensureSyntheticRoute(routes, pattern, route, "health", []string{"GET"})
		}
	}

	if t.server.Options().MCPEnabled {
		options := t.server.Options()
		methods := []string{"POST"}
		if options.MCPLegacyRoutedSSE {
			methods = []string{"GET", "POST"}
		}
		routes = ensureSyntheticRoute(routes, pattern, options.MCPEndpoint, "main", methods)
	}

	result := map[string]any{
		"routes": routes,
		"total":  len(routes),
		"note":   "Routes discovered from registered handlers and known server endpoints",
	}
	if includeMiddleware {
		stacks := t.server.MiddlewareRoutes()
		prefixes := make([]string, 0, len(stacks))
		for prefix := range stacks {
			prefixes = append(prefixes, prefix)
		}
		slices.Sort(prefixes)
		registrations := make([]map[string]any, 0, len(prefixes))
		for _, prefix := range prefixes {
			registrations = append(registrations, map[string]any{
				"prefix": prefix,
				"count":  len(stacks[prefix]),
			})
		}
		result["middleware_registrations"] = registrations
		result["middleware_note"] = "Main server registrations; * is global, the empty prefix covers all paths, and other prefixes match at slash boundaries. Route filters do not filter these registrations."
	}
	return result, nil
}

func splitServeMuxPattern(route string) (string, []string) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || path == "" {
		return route, []string{"ANY"}
	}
	return path, []string{method}
}

func makeRouteInfo(pattern, server string, methods []string) map[string]any {
	info := map[string]any{
		"pattern": pattern,
		"methods": methods,
	}
	if server != "" {
		info["server"] = server
	}
	return info
}

// ensureSyntheticRoute appends a route entry for a server-managed endpoint or
// corrects the method metadata of an already tracked pattern. Server-managed
// handlers such as /mcp are registered as plain ServeMux patterns, so their
// protocol-specific methods cannot be inferred from the pattern alone.
func ensureSyntheticRoute(routes []map[string]any, filter, route, server string, methods []string) []map[string]any {
	if filter != "" && !strings.Contains(route, filter) {
		return routes
	}
	for _, existing := range routes {
		if existing["pattern"] == route && existing["server"] == server {
			existing["methods"] = methods
			if server != "" {
				existing["server"] = server
			}
			return routes
		}
	}
	return append(routes, makeRouteInfo(route, server, methods))
}

// DevGuideTool surfaces a short reference about the available developer tools.
type DevGuideTool struct {
	server *hyperserve.Server
}

// NewDevGuideTool creates a DevGuideTool.
func NewDevGuideTool(srv *hyperserve.Server) *DevGuideTool {
	return &DevGuideTool{server: srv}
}

func (t *DevGuideTool) Name() string { return "dev_guide" }

func (t *DevGuideTool) Description() string {
	return "Get help and examples for using HyperServe MCP developer tools. Shows available tools, resources, and common workflows."
}

func (t *DevGuideTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic": map[string]any{
				"type":        "string",
				"enum":        []string{"overview", "tools", "resources", "examples", "workflows"},
				"description": "Help topic: overview (all capabilities), tools (available tools), resources (data sources), examples (usage examples), workflows (common tasks)",
			},
		},
	}
}

func (t *DevGuideTool) Execute(params map[string]any) (any, error) {
	topic, _ := params["topic"].(string)
	if topic == "" {
		topic = "overview"
	}
	switch topic {
	case "overview":
		return map[string]any{
			"description": "HyperServe MCP Developer Tools",
			"tools": []map[string]any{
				{"name": "server_control", "purpose": "Inspect server health and configuration status", "actions": serverControlActions()},
				{"name": "route_inspector", "purpose": "View registered HTTP routes", "features": []string{"filter routes by pattern", "list middleware registration scopes"}},
				{"name": "dev_guide", "purpose": "This help tool", "topics": []string{"overview", "tools", "resources", "examples", "workflows"}},
			},
			"resources": []map[string]any{
				{"uri": "logs://server/stream", "purpose": "Recent MCP server log snapshot; re-read to refresh"},
				{"uri": "routes://server/all", "purpose": "Detailed route information"},
			},
			"tip": "Use 'dev_guide' with topic='examples' to see usage examples",
		}, nil

	case "tools":
		return map[string]any{
			"available_tools": []map[string]any{
				{
					"tool": "server_control",
					"actions": map[string]string{
						serverControlGetStatus: "Check if the server is running, ready, and how it is configured",
					},
				},
				{
					"tool": "route_inspector",
					"parameters": map[string]string{
						"pattern":            "Filter routes by pattern (optional)",
						"include_middleware": "Show middleware info (default: true)",
					},
				},
			},
		}, nil

	case "resources":
		return map[string]any{
			"available_resources": []map[string]any{
				{
					"uri":         "logs://server/stream",
					"description": "Bounded snapshot of recent MCP server logs",
					"contents":    "Recent log entries with timestamp, level, message",
					"use_case":    "Re-read while diagnosing server activity during development",
				},
				{
					"uri":         "routes://server/all",
					"description": "Complete list of registered routes",
					"contents":    "Route patterns and declared HTTP methods",
					"use_case":    "Understand registered request routes",
				},
			},
		}, nil

	case "examples":
		return map[string]any{
			"common_examples": []map[string]any{
				{"task": "Check server status", "tool": "server_control", "arguments": map[string]any{"action": serverControlGetStatus}},
				{"task": "Find all API routes", "tool": "route_inspector", "arguments": map[string]any{"pattern": "/api"}},
			},
		}, nil

	case "workflows":
		return map[string]any{
			"common_workflows": []map[string]any{
				{
					"workflow": "Debug 404 errors",
					"steps": []string{
						"1. Use route_inspector to list all routes",
						"2. Check if your path matches any pattern",
						"3. Check which middleware registration prefixes cover the request path",
					},
				},
				{
					"workflow": "Performance debugging",
					"steps": []string{
						"1. Check server status with server_control",
						"2. Re-read logs://server/stream for recent entries",
						"3. Inspect middleware registration scopes with route_inspector",
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown topic: %s. Valid topics: overview, tools, resources, examples, workflows", topic)
	}
}
