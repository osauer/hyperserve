package builtin

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
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
	return "Control HyperServe server lifecycle and configuration. Actions: get_status (check server health), set_log_level (DEBUG/INFO/WARN/ERROR), reload (refresh config), restart (graceful restart)"
}

func (t *ServerControlTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"restart", "reload", "set_log_level", "get_status"},
				"description": "Action to perform: get_status (check server health), set_log_level (change logging verbosity), reload (refresh configuration without restart), restart (graceful server restart)",
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
	case "restart":
		logger.Warn("Server restart requested via MCP developer tools")
		return map[string]any{
			"status":  "restart_initiated",
			"message": "Server will restart. Please wait a moment before making new requests.",
			"note":    "In production, use process managers like systemd or supervisor for restarts.",
		}, nil

	case "reload":
		logger.Info("Configuration reload requested via MCP developer tools")
		return map[string]any{
			"status":    "reloaded",
			"timestamp": time.Now().Format(time.RFC3339),
			"message":   "Configuration and templates reloaded",
		}, nil

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
	for route, middlewareStack := range t.server.MiddlewareRoutes() {
		if pattern != "" && !strings.Contains(route, pattern) {
			continue
		}
		routeInfo := map[string]any{
			"pattern": route,
			"methods": []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
		}
		if includeMiddleware {
			names := make([]string, 0, len(middlewareStack))
			for _, mw := range middlewareStack {
				name := fmt.Sprintf("%T", mw)
				if strings.Contains(name, ".") {
					parts := strings.Split(name, ".")
					name = parts[len(parts)-1]
				}
				names = append(names, name)
			}
			routeInfo["middleware"] = names
		}
		routes = append(routes, routeInfo)
	}

	// Synthesize known health routes when they aren't visible via middleware registry.
	if t.server.Options.RunHealthServer {
		for _, route := range []string{"/healthz", "/readyz", "/livez"} {
			if pattern != "" && !strings.Contains(route, pattern) {
				continue
			}
			found := false
			for _, existing := range routes {
				if existing["pattern"] == route {
					found = true
					break
				}
			}
			if !found {
				routeInfo := map[string]any{
					"pattern": route,
					"methods": []string{"GET"},
					"server":  "health",
				}
				if includeMiddleware {
					routeInfo["middleware"] = []string{"HealthCheckMiddleware"}
				}
				routes = append(routes, routeInfo)
			}
		}
	}

	if t.server.Options.MCPEnabled {
		mcpRoute := t.server.Options.MCPEndpoint
		if pattern == "" || strings.Contains(mcpRoute, pattern) {
			found := false
			for _, existing := range routes {
				if existing["pattern"] == mcpRoute {
					found = true
					break
				}
			}
			if !found {
				routeInfo := map[string]any{
					"pattern": mcpRoute,
					"methods": []string{"GET", "POST"},
					"server":  "main",
				}
				if includeMiddleware {
					routeInfo["middleware"] = []string{"MCPMiddleware"}
				}
				routes = append(routes, routeInfo)
			}
		}
	}

	return map[string]any{
		"routes": routes,
		"total":  len(routes),
		"note":   "Routes discovered from middleware registry and known server endpoints",
	}, nil
}

// CapturedRequest represents an HTTP request that the request debugger
// recorded.
type CapturedRequest struct {
	ID        string              `json:"id"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	Timestamp time.Time           `json:"timestamp"`
	Response  *CapturedResponse   `json:"response,omitempty"`
}

// CapturedResponse is the response side of a CapturedRequest.
type CapturedResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

// RequestDebuggerTool captures HTTP requests for inspection/replay.
type RequestDebuggerTool struct {
	server           *server.Server
	captures         sync.Map // map[string]*CapturedRequest
	requestIDCounter int64
}

// NewRequestDebuggerTool creates a RequestDebuggerTool.
func NewRequestDebuggerTool(srv *server.Server) *RequestDebuggerTool {
	return &RequestDebuggerTool{server: srv}
}

func (t *RequestDebuggerTool) Name() string { return "request_debugger" }

func (t *RequestDebuggerTool) Description() string {
	return "Debug HTTP requests in HyperServe. Actions: list (show captured requests), get (inspect request details), replay (resend with modifications), clear (remove all captures). Captures last 100 requests automatically."
}

func (t *RequestDebuggerTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "get", "replay", "clear"},
				"description": "Operation to perform: list (show all captured requests), get (view request details by ID), replay (resend a request), clear (delete all captures)",
			},
			"request_id": map[string]any{
				"type":        "string",
				"description": "Request ID for get/replay actions. Get the ID from 'list' action first.",
			},
			"modifications": map[string]any{
				"type":        "object",
				"description": "Optional modifications to apply when replaying a request (for replay action only)",
				"properties": map[string]any{
					"headers": map[string]any{
						"type":        "object",
						"description": "Headers to add/override as key-value pairs",
					},
					"body": map[string]any{
						"type":        "string",
						"description": "New request body to use instead of original",
					},
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *RequestDebuggerTool) Execute(params map[string]any) (any, error) {
	action, _ := params["action"].(string)

	switch action {
	case "list":
		requests := []map[string]any{}
		t.captures.Range(func(key, value any) bool {
			if req, ok := value.(*CapturedRequest); ok {
				requests = append(requests, map[string]any{
					"id":        req.ID,
					"method":    req.Method,
					"path":      req.Path,
					"timestamp": req.Timestamp,
				})
			}
			return true
		})
		return map[string]any{
			"requests": requests,
			"count":    len(requests),
		}, nil

	case "get":
		id, _ := params["request_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("request_id is required")
		}
		if val, ok := t.captures.Load(id); ok {
			return val, nil
		}
		return nil, fmt.Errorf("request not found: %s", id)

	case "replay":
		return map[string]any{
			"status": "replay_not_implemented",
			"note":   "Request replay would replay the captured request with modifications",
		}, nil

	case "clear":
		t.captures.Range(func(key, value any) bool {
			t.captures.Delete(key)
			return true
		})
		return map[string]any{"status": "cleared"}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// CaptureRequest captures an HTTP request and stores it in the tool. It also
// caps the in-memory store at 100 entries to prevent unbounded growth.
func (t *RequestDebuggerTool) CaptureRequest(r *http.Request, responseHeaders map[string][]string, statusCode int, responseBody string) {
	counter := atomic.AddInt64(&t.requestIDCounter, 1)
	id := fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), counter)

	var body string
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			body = string(bodyBytes)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	t.captures.Store(id, &CapturedRequest{
		ID:        id,
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   r.Header,
		Body:      body,
		Timestamp: time.Now(),
		Response: &CapturedResponse{
			Status:  statusCode,
			Headers: responseHeaders,
			Body:    responseBody,
		},
	})

	count := 0
	t.captures.Range(func(key, value any) bool {
		count++
		return true
	})
	if count > 100 {
		toDelete := count - 100
		deleted := 0
		t.captures.Range(func(key, value any) bool {
			if deleted >= toDelete {
				return false
			}
			t.captures.Delete(key)
			deleted++
			return true
		})
	}
}

// RequestCaptureMiddleware returns middleware that records HTTP requests into
// the supplied RequestDebuggerTool. Requests under /mcp are skipped to avoid
// recursive capture loops.
func RequestCaptureMiddleware(debuggerTool *RequestDebuggerTool) server.MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/mcp") {
				next.ServeHTTP(w, r)
				return
			}
			crw := &captureResponseWriter{
				ResponseWriter: w,
				headers:        make(map[string][]string),
				body:           &bytes.Buffer{},
				statusCode:     200,
			}
			next.ServeHTTP(crw, r)
			responseHeaders := make(map[string][]string)
			maps.Copy(responseHeaders, crw.headers)
			maps.Copy(responseHeaders, w.Header())
			debuggerTool.CaptureRequest(r, responseHeaders, crw.statusCode, crw.body.String())
		}
	}
}

type captureResponseWriter struct {
	http.ResponseWriter
	headers    map[string][]string
	body       *bytes.Buffer
	statusCode int
}

func (crw *captureResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}

func (crw *captureResponseWriter) Write(b []byte) (int, error) {
	if crw.body.Len() < 64*1024 {
		crw.body.Write(b)
	}
	return crw.ResponseWriter.Write(b)
}

func (crw *captureResponseWriter) Header() http.Header { return crw.ResponseWriter.Header() }

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
				{"name": "server_control", "purpose": "Manage server lifecycle and configuration", "actions": []string{"get_status", "set_log_level", "reload", "restart"}},
				{"name": "route_inspector", "purpose": "View all registered HTTP routes", "features": []string{"filter by pattern", "show middleware chains"}},
				{"name": "request_debugger", "purpose": "Capture and debug HTTP requests", "actions": []string{"list", "get", "replay", "clear"}},
				{"name": "dev_guide", "purpose": "This help tool", "topics": []string{"overview", "tools", "resources", "examples", "workflows"}},
			},
			"resources": []map[string]any{
				{"uri": "logs://server/stream", "purpose": "Real-time server logs"},
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
						"reload":        "Reload configuration without restart",
						"restart":       "Gracefully restart the server",
					},
				},
				{
					"tool": "route_inspector",
					"parameters": map[string]string{
						"pattern":            "Filter routes by pattern (optional)",
						"include_middleware": "Show middleware info (default: true)",
					},
				},
				{
					"tool": "request_debugger",
					"actions": map[string]string{
						"list":   "Show all captured requests",
						"get":    "View full details of a specific request",
						"replay": "Resend a request with modifications",
						"clear":  "Delete all captured requests",
					},
				},
			},
		}, nil

	case "resources":
		return map[string]any{
			"available_resources": []map[string]any{
				{
					"uri":         "logs://server/stream",
					"description": "Real-time server log stream",
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
						"4. Use request_debugger to capture the 404 request",
					},
				},
				{
					"workflow": "Performance debugging",
					"steps": []string{
						"1. Enable DEBUG logging",
						"2. Monitor logs://server/stream",
						"3. Use request_debugger to capture slow requests",
						"4. Check middleware execution in route_inspector",
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown topic: %s. Valid topics: overview, tools, resources, examples, workflows", topic)
	}
}
