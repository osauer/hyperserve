package builtin

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/osauer/hyperserve/pkg/server"
)

// ServerControlTool provides server lifecycle management for development.
type ServerControlTool struct {
	server *server.Server
	mu     sync.Mutex
}

// NewServerControlTool creates a ServerControlTool.
func NewServerControlTool(srv *server.Server) *ServerControlTool {
	return &ServerControlTool{server: srv}
}

func (t *ServerControlTool) Name() string { return "server_control" }

func (t *ServerControlTool) Description() string {
	return "Inspect and adjust the running HyperServe instance. Actions: get_status (check health), set_log_level (DEBUG/INFO/WARN/ERROR)."
}

func (t *ServerControlTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"set_log_level", "get_status"},
				"description": "Action to perform: get_status (read server health) or set_log_level (change logging verbosity).",
			},
			"log_level": map[string]any{
				"type":        "string",
				"enum":        []string{"DEBUG", "INFO", "WARN", "ERROR"},
				"description": "New log level for set_log_level action. DEBUG shows all logs, INFO shows informational and above, WARN shows warnings and errors, ERROR shows only errors",
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
	t.mu.Lock()
	defer t.mu.Unlock()

	switch action {
	case "set_log_level":
		level, ok := params["log_level"].(string)
		if !ok {
			return nil, fmt.Errorf("log_level is required for set_log_level action")
		}
		switch level {
		case "DEBUG":
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "INFO":
			slog.SetLogLoggerLevel(slog.LevelInfo)
		case "WARN":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "ERROR":
			slog.SetLogLoggerLevel(slog.LevelError)
		default:
			return nil, fmt.Errorf("invalid log level: %s", level)
		}
		t.server.Options.LogLevel = level
		return map[string]any{
			"status":    "log_level_changed",
			"new_level": level,
		}, nil

	case "get_status":
		return map[string]any{
			"running":   t.server.IsRunning(),
			"ready":     t.server.IsReady(),
			"uptime":    time.Since(t.server.ServerStart()).String(),
			"log_level": t.server.Options.LogLevel,
			"addr":      t.server.Options.Addr,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// RouteInspectorTool provides route introspection for development.
type RouteInspectorTool struct {
	server *server.Server
}

// NewRouteInspectorTool creates a RouteInspectorTool.
func NewRouteInspectorTool(srv *server.Server) *RouteInspectorTool {
	return &RouteInspectorTool{server: srv}
}

func (t *RouteInspectorTool) Name() string { return "route_inspector" }

func (t *RouteInspectorTool) Description() string {
	return "List all registered HTTP routes in HyperServe with their patterns and middleware. Use pattern parameter to filter routes (e.g., '/api' shows only API routes)"
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
				"description": "Include middleware chain information for each route (default: true). Shows security headers, rate limiting, auth middleware, etc.",
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
	middlewareRoutes := t.server.MiddlewareRoutes()
	for _, route := range t.server.RegisteredRoutes() {
		routePattern, methods := splitServeMuxPattern(route)
		if pattern != "" && !strings.Contains(routePattern, pattern) {
			continue
		}
		middlewareStack := middlewareRoutes[route]
		if len(middlewareStack) == 0 {
			middlewareStack = middlewareRoutes[routePattern]
		}
		routes = append(routes, makeRouteInfo(routePattern, "main", methods, includeMiddleware, middlewareStackNames(middlewareStack)))
	}

	// Synthesize known health routes when they aren't visible via middleware registry.
	if t.server.Options.RunHealthServer {
		for _, route := range []string{"/healthz", "/readyz", "/livez"} {
			routes = ensureSyntheticRoute(routes, pattern, route, "health", []string{"GET"}, includeMiddleware, []string{"HealthCheckMiddleware"})
		}
	}

	if t.server.Options.MCPEnabled {
		mcpRoute := t.server.Options.MCPEndpoint
		routes = ensureSyntheticRoute(routes, pattern, mcpRoute, "main", []string{"GET", "POST"}, includeMiddleware, []string{"MCPMiddleware"})
	}

	return map[string]any{
		"routes": routes,
		"total":  len(routes),
		"note":   "Routes discovered from registered handlers and known server endpoints",
	}, nil
}

func splitServeMuxPattern(route string) (string, []string) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || path == "" {
		return route, []string{"ANY"}
	}
	return path, []string{method}
}

func makeRouteInfo(pattern, server string, methods []string, includeMiddleware bool, middlewareNames []string) map[string]any {
	info := map[string]any{
		"pattern": pattern,
		"methods": methods,
	}
	if server != "" {
		info["server"] = server
	}
	if includeMiddleware {
		info["middleware"] = middlewareNames
	}
	return info
}

func middlewareStackNames(stack server.MiddlewareStack) []string {
	names := make([]string, 0, len(stack))
	for _, mw := range stack {
		name := fmt.Sprintf("%T", mw)
		if strings.Contains(name, ".") {
			parts := strings.Split(name, ".")
			name = parts[len(parts)-1]
		}
		names = append(names, name)
	}
	return names
}

// ensureSyntheticRoute appends a route entry for a server-managed endpoint
// (e.g. /healthz, /mcp) that isn't visible via the middleware registry, but
// only when the pattern filter (if any) matches and the route isn't already
// present in `routes`.
func ensureSyntheticRoute(routes []map[string]any, filter, route, server string, methods []string, includeMiddleware bool, middlewareNames []string) []map[string]any {
	if filter != "" && !strings.Contains(route, filter) {
		return routes
	}
	for _, existing := range routes {
		if existing["pattern"] == route {
			return routes
		}
	}
	return append(routes, makeRouteInfo(route, server, methods, includeMiddleware, middlewareNames))
}

// DevGuideTool surfaces a short reference about the available developer tools.
type DevGuideTool struct {
	server *server.Server
}

// NewDevGuideTool creates a DevGuideTool.
func NewDevGuideTool(srv *server.Server) *DevGuideTool {
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
				{"name": "server_control", "purpose": "Inspect server health and adjust log level", "actions": []string{"get_status", "set_log_level"}},
				{"name": "route_inspector", "purpose": "View all registered HTTP routes", "features": []string{"filter by pattern", "show middleware chains"}},
				{"name": "dev_guide", "purpose": "This help tool", "topics": []string{"overview", "tools", "resources", "examples", "workflows"}},
			},
			"resources": []map[string]any{
				{"uri": "logs://server/stream", "purpose": "Real-time MCP server logs"},
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
						"get_status":    "Check if server is running, uptime, current log level",
						"set_log_level": "Change logging verbosity (DEBUG, INFO, WARN, ERROR)",
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
					"description": "Real-time MCP server log stream",
					"contents":    "Recent log entries with timestamp, level, message",
					"use_case":    "Monitor server activity during development",
				},
				{
					"uri":         "routes://server/all",
					"description": "Complete list of registered routes",
					"contents":    "Route patterns, HTTP methods, middleware chains",
					"use_case":    "Understand request routing and middleware pipeline",
				},
			},
		}, nil

	case "examples":
		return map[string]any{
			"common_examples": []map[string]any{
				{"task": "Enable debug logging", "tool": "server_control", "arguments": map[string]any{"action": "set_log_level", "log_level": "DEBUG"}},
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
						"3. Enable DEBUG logging to see route matching",
					},
				},
				{
					"workflow": "Performance debugging",
					"steps": []string{
						"1. Enable DEBUG logging",
						"2. Monitor logs://server/stream",
						"3. Check middleware execution in route_inspector",
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown topic: %s. Valid topics: overview, tools, resources, examples, workflows", topic)
	}
}
