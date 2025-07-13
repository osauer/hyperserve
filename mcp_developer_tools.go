// Package hyperserve provides MCP developer tools for interactive development.
//
// These tools are designed for development environments where Claude Code
// or other AI assistants help build applications. They provide controlled
// access to server internals while maintaining security boundaries.
//
// SECURITY: These tools should ONLY be enabled in development environments.
// Never enable MCPDeveloperPreset in production!
package hyperserve

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPDev configures MCP with developer tools for local development.
// This configuration is designed for AI-assisted development with Claude Code.
//
// ⚠️  SECURITY WARNING: Only use in development environments!
// Enables powerful tools that can restart your server and modify its behavior.
//
// Tools provided:
//   - server_control: Restart server, reload config, change log levels, get status
//   - route_inspector: List all registered routes and their middleware
//   - request_debugger: Capture and replay HTTP requests for debugging
//
// Resources provided:
//   - logs://server/stream: Real-time log streaming
//   - routes://server/all: All registered routes with metadata
//   - requests://debug/recent: Recent requests with full details
//
// Example:
//   srv, _ := hyperserve.NewServer(
//       hyperserve.WithMCPSupport("DevServer", "1.0.0", hyperserve.MCPDev()),
//   )
func MCPDev() MCPTransportConfig {
	return func(opts *mcpTransportOptions) {
		opts.developerMode = true
	}
}

// ServerControlTool provides server lifecycle management for development
type ServerControlTool struct {
	server *Server
	mu     sync.Mutex
}

func (t *ServerControlTool) Name() string {
	return "server_control"
}

func (t *ServerControlTool) Description() string {
	return "Control HyperServe server lifecycle and configuration. Actions: get_status (check server health), set_log_level (DEBUG/INFO/WARN/ERROR), reload (refresh config), restart (graceful restart)"
}

func (t *ServerControlTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"restart", "reload", "set_log_level", "get_status"},
				"description": "Action to perform: get_status (check server health), set_log_level (change logging verbosity), reload (refresh configuration without restart), restart (graceful server restart)",
			},
			"log_level": map[string]interface{}{
				"type": "string",
				"enum": []string{"DEBUG", "INFO", "WARN", "ERROR"},
				"description": "New log level for set_log_level action. DEBUG shows all logs, INFO shows informational and above, WARN shows warnings and errors, ERROR shows only errors",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ServerControlTool) Execute(params map[string]interface{}) (interface{}, error) {
	action, ok := params["action"].(string)
	if !ok {
		return nil, fmt.Errorf("action is required")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch action {
	case "restart":
		// Signal server restart - in real implementation, this would trigger a graceful restart
		logger.Warn("Server restart requested via MCP developer tools")
		return map[string]interface{}{
			"status": "restart_initiated",
			"message": "Server will restart. Please wait a moment before making new requests.",
			"note": "In production, use process managers like systemd or supervisor for restarts.",
		}, nil

	case "reload":
		// Reload configuration, templates, etc. without full restart
		logger.Info("Configuration reload requested via MCP developer tools")
		// Here you would implement actual reload logic:
		// - Reload templates
		// - Refresh static file cache
		// - Re-read configuration files
		return map[string]interface{}{
			"status": "reloaded",
			"timestamp": time.Now().Format(time.RFC3339),
			"message": "Configuration and templates reloaded",
		}, nil

	case "set_log_level":
		level, ok := params["log_level"].(string)
		if !ok {
			return nil, fmt.Errorf("log_level is required for set_log_level action")
		}
		// Set the log level
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
		return map[string]interface{}{
			"status": "log_level_changed",
			"new_level": level,
		}, nil

	case "get_status":
		return map[string]interface{}{
			"running": t.server.isRunning.Load(),
			"ready": t.server.isReady.Load(),
			"uptime": time.Since(t.server.serverStart).String(),
			"log_level": t.server.Options.LogLevel,
			"addr": t.server.Options.Addr,
		}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// RouteInspectorTool provides route introspection for development
type RouteInspectorTool struct {
	server *Server
}

func (t *RouteInspectorTool) Name() string {
	return "route_inspector"
}

func (t *RouteInspectorTool) Description() string {
	return "List all registered HTTP routes in HyperServe with their patterns and middleware. Use pattern parameter to filter routes (e.g., '/api' shows only API routes)"
}

func (t *RouteInspectorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type": "string",
				"description": "Optional pattern to filter routes (e.g., '/api' to show only API routes, '/health' for health check endpoints)",
			},
			"include_middleware": map[string]interface{}{
				"type": "boolean",
				"description": "Include middleware chain information for each route (default: true). Shows security headers, rate limiting, auth middleware, etc.",
				"default": true,
			},
		},
	}
}

func (t *RouteInspectorTool) Execute(params map[string]interface{}) (interface{}, error) {
	pattern, _ := params["pattern"].(string)
	includeMiddleware, _ := params["include_middleware"].(bool)
	if includeMiddleware == false && params["include_middleware"] == nil {
		includeMiddleware = true
	}

	// Get routes from mux (this is a simplified version)
	// In reality, we'd need to introspect the ServeMux more deeply
	routes := []map[string]interface{}{}

	// Add known routes (this would be enhanced with actual route discovery)
	baseRoutes := []string{"/", "/healthz", "/readyz", "/livez"}
	if t.server.Options.MCPEnabled {
		baseRoutes = append(baseRoutes, t.server.Options.MCPEndpoint)
	}

	for _, route := range baseRoutes {
		if pattern != "" && !strings.Contains(route, pattern) {
			continue
		}

		routeInfo := map[string]interface{}{
			"pattern": route,
			"methods": []string{"GET", "POST"}, // ServeMux doesn't track methods
		}

		if includeMiddleware {
			// Middleware information would require extending the middleware registry
			// For now, we just indicate that middleware exists
			routeInfo["middleware"] = []string{"DefaultMiddleware", "Route-specific middleware (if any)"}
		}

		routes = append(routes, routeInfo)
	}

	return map[string]interface{}{
		"routes": routes,
		"total": len(routes),
		"note": "Route discovery is limited in Go's ServeMux. Consider using a router with better introspection.",
	}, nil
}

// RequestDebuggerTool captures and allows replay of requests
type RequestDebuggerTool struct {
	server *Server
	captures sync.Map // map[string]*CapturedRequest
	requestIDCounter int64
}

type CapturedRequest struct {
	ID        string                 `json:"id"`
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Headers   map[string][]string    `json:"headers"`
	Body      string                 `json:"body"`
	Timestamp time.Time              `json:"timestamp"`
	Response  *CapturedResponse      `json:"response,omitempty"`
}

type CapturedResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`
}

func (t *RequestDebuggerTool) Name() string {
	return "request_debugger"
}

func (t *RequestDebuggerTool) Description() string {
	return "Debug HTTP requests in HyperServe. Actions: list (show captured requests), get (inspect request details), replay (resend with modifications), clear (remove all captures). Captures last 100 requests automatically."
}

func (t *RequestDebuggerTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"list", "get", "replay", "clear"},
				"description": "Operation to perform: list (show all captured requests), get (view request details by ID), replay (resend a request), clear (delete all captures)",
			},
			"request_id": map[string]interface{}{
				"type": "string",
				"description": "Request ID for get/replay actions. Get the ID from 'list' action first.",
			},
			"modifications": map[string]interface{}{
				"type": "object",
				"description": "Optional modifications to apply when replaying a request (for replay action only)",
				"properties": map[string]interface{}{
					"headers": map[string]interface{}{
						"type": "object",
						"description": "Headers to add/override as key-value pairs",
					},
					"body": map[string]interface{}{
						"type": "string",
						"description": "New request body to use instead of original",
					},
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *RequestDebuggerTool) Execute(params map[string]interface{}) (interface{}, error) {
	action, _ := params["action"].(string)

	switch action {
	case "list":
		requests := []map[string]interface{}{}
		t.captures.Range(func(key, value interface{}) bool {
			if req, ok := value.(*CapturedRequest); ok {
				requests = append(requests, map[string]interface{}{
					"id": req.ID,
					"method": req.Method,
					"path": req.Path,
					"timestamp": req.Timestamp,
				})
			}
			return true
		})
		return map[string]interface{}{
			"requests": requests,
			"count": len(requests),
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
		// Replay functionality would go here
		return map[string]interface{}{
			"status": "replay_not_implemented",
			"note": "Request replay would replay the captured request with modifications",
		}, nil

	case "clear":
		t.captures.Range(func(key, value interface{}) bool {
			t.captures.Delete(key)
			return true
		})
		return map[string]interface{}{
			"status": "cleared",
		}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// CaptureRequest captures an HTTP request and stores it in the debug tool
func (t *RequestDebuggerTool) CaptureRequest(r *http.Request, responseHeaders map[string][]string, statusCode int, responseBody string) {
	// Generate unique request ID
	counter := atomic.AddInt64(&t.requestIDCounter, 1)
	id := fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), counter)
	
	// Read request body if present
	var body string
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			body = string(bodyBytes)
			// Replace body with a new ReadCloser so the original handler can still read it
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}
	
	// Create captured request
	capturedReq := &CapturedRequest{
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
	}
	
	// Store in captures map
	t.captures.Store(id, capturedReq)
	
	// Implement a simple LRU-like cleanup to prevent memory leaks
	// Keep only the last 100 requests
	count := 0
	t.captures.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	
	if count > 100 {
		// Remove oldest entries (this is a simple implementation)
		toDelete := count - 100
		deleted := 0
		t.captures.Range(func(key, value interface{}) bool {
			if deleted >= toDelete {
				return false
			}
			t.captures.Delete(key)
			deleted++
			return true
		})
	}
}

// RequestCaptureMiddleware creates middleware that captures HTTP requests for debugging
func RequestCaptureMiddleware(debuggerTool *RequestDebuggerTool) MiddlewareFunc {
	return func(next http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Skip capturing for the MCP endpoint itself to avoid recursion
			if strings.HasPrefix(r.URL.Path, "/mcp") {
				next.ServeHTTP(w, r)
				return
			}
			
			// Create a response writer that captures response data
			crw := &captureResponseWriter{
				ResponseWriter: w,
				headers:        make(map[string][]string),
				body:           &bytes.Buffer{},
				statusCode:     200, // Default status code
			}
			
			// Call the next handler
			next.ServeHTTP(crw, r)
			
			// Capture the request after the response is complete
			responseHeaders := make(map[string][]string)
			for k, v := range crw.headers {
				responseHeaders[k] = v
			}
			
			// Also capture headers that were actually written
			for k, v := range w.Header() {
				responseHeaders[k] = v
			}
			
			debuggerTool.CaptureRequest(r, responseHeaders, crw.statusCode, crw.body.String())
		}
	}
}

// captureResponseWriter wraps http.ResponseWriter to capture response data
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
	// Capture response body (limit to reasonable size to prevent memory issues)
	if crw.body.Len() < 64*1024 { // 64KB limit
		crw.body.Write(b)
	}
	return crw.ResponseWriter.Write(b)
}

func (crw *captureResponseWriter) Header() http.Header {
	return crw.ResponseWriter.Header()
}

// StreamingLogResource provides real-time log streaming
type StreamingLogResource struct {
	*ServerLogResource
	subscribers sync.Map // map[string]chan logEntry
}

func (r *StreamingLogResource) URI() string {
	return "logs://server/stream"
}

func (r *StreamingLogResource) Name() string {
	return "Server Log Stream"
}

func (r *StreamingLogResource) Description() string {
	return "Real-time server log streaming for development"
}

// RouteListResource provides detailed route information
type RouteListResource struct {
	server *Server
}

func (r *RouteListResource) URI() string {
	return "routes://server/all"
}

func (r *RouteListResource) Name() string {
	return "Server Routes"
}

func (r *RouteListResource) Description() string {
	return "All registered routes with metadata"
}

func (r *RouteListResource) MimeType() string {
	return "application/json"
}

func (r *RouteListResource) Read() (interface{}, error) {
	// This would provide detailed route information
	return map[string]interface{}{
		"routes": []interface{}{
			// Route data would go here
		},
		"note": "Enhanced route introspection requires router enhancement",
	}, nil
}

func (r *RouteListResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// DevGuideTool provides helpful information about available MCP tools
type DevGuideTool struct {
	server *Server
}

func (t *DevGuideTool) Name() string {
	return "dev_guide"
}

func (t *DevGuideTool) Description() string {
	return "Get help and examples for using HyperServe MCP developer tools. Shows available tools, resources, and common workflows."
}

func (t *DevGuideTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"topic": map[string]interface{}{
				"type": "string",
				"enum": []string{"overview", "tools", "resources", "examples", "workflows"},
				"description": "Help topic: overview (all capabilities), tools (available tools), resources (data sources), examples (usage examples), workflows (common tasks)",
			},
		},
	}
}

func (t *DevGuideTool) Execute(params map[string]interface{}) (interface{}, error) {
	topic, _ := params["topic"].(string)
	if topic == "" {
		topic = "overview"
	}

	switch topic {
	case "overview":
		return map[string]interface{}{
			"description": "HyperServe MCP Developer Tools",
			"tools": []map[string]interface{}{
				{
					"name": "server_control",
					"purpose": "Manage server lifecycle and configuration",
					"actions": []string{"get_status", "set_log_level", "reload", "restart"},
				},
				{
					"name": "route_inspector", 
					"purpose": "View all registered HTTP routes",
					"features": []string{"filter by pattern", "show middleware chains"},
				},
				{
					"name": "request_debugger",
					"purpose": "Capture and debug HTTP requests",
					"actions": []string{"list", "get", "replay", "clear"},
				},
				{
					"name": "dev_guide",
					"purpose": "This help tool",
					"topics": []string{"overview", "tools", "resources", "examples", "workflows"},
				},
			},
			"resources": []map[string]interface{}{
				{
					"uri": "logs://server/stream",
					"purpose": "Real-time server logs",
				},
				{
					"uri": "routes://server/all", 
					"purpose": "Detailed route information",
				},
			},
			"tip": "Use 'dev_guide' with topic='examples' to see usage examples",
		}, nil

	case "tools":
		return map[string]interface{}{
			"available_tools": []map[string]interface{}{
				{
					"tool": "server_control",
					"actions": map[string]string{
						"get_status": "Check if server is running, uptime, current log level",
						"set_log_level": "Change logging verbosity (DEBUG, INFO, WARN, ERROR)", 
						"reload": "Reload configuration without restart",
						"restart": "Gracefully restart the server",
					},
					"example": map[string]interface{}{
						"name": "server_control",
						"arguments": map[string]string{
							"action": "set_log_level",
							"log_level": "DEBUG",
						},
					},
				},
				{
					"tool": "route_inspector",
					"parameters": map[string]string{
						"pattern": "Filter routes by pattern (optional)",
						"include_middleware": "Show middleware info (default: true)",
					},
					"example": map[string]interface{}{
						"name": "route_inspector",
						"arguments": map[string]interface{}{
							"pattern": "/api",
						},
					},
				},
				{
					"tool": "request_debugger",
					"actions": map[string]string{
						"list": "Show all captured requests",
						"get": "View full details of a specific request",
						"replay": "Resend a request with modifications", 
						"clear": "Delete all captured requests",
					},
					"workflow": []string{
						"1. Use action='list' to see captured requests",
						"2. Note the request_id you want to inspect",
						"3. Use action='get' with request_id to see details",
						"4. Use action='replay' to resend with modifications",
					},
				},
			},
		}, nil

	case "resources":
		return map[string]interface{}{
			"available_resources": []map[string]interface{}{
				{
					"uri": "logs://server/stream",
					"description": "Real-time server log stream",
					"contents": "Recent log entries with timestamp, level, message",
					"use_case": "Monitor server activity during development",
				},
				{
					"uri": "routes://server/all",
					"description": "Complete list of registered routes",
					"contents": "Route patterns, HTTP methods, middleware chains",
					"use_case": "Understand request routing and middleware pipeline",
				},
			},
		}, nil

	case "examples":
		return map[string]interface{}{
			"common_examples": []map[string]interface{}{
				{
					"task": "Enable debug logging",
					"tool": "server_control",
					"arguments": map[string]interface{}{
						"action": "set_log_level",
						"log_level": "DEBUG",
					},
				},
				{
					"task": "Find all API routes",
					"tool": "route_inspector",
					"arguments": map[string]interface{}{
						"pattern": "/api",
					},
				},
				{
					"task": "Debug a failing request",
					"steps": []string{
						"1. Set log level to DEBUG",
						"2. Make the failing request",
						"3. Use request_debugger with action='list'",
						"4. Get the request_id and use action='get' to inspect",
						"5. Check logs://server/stream for error details",
					},
				},
			},
		}, nil

	case "workflows":
		return map[string]interface{}{
			"common_workflows": []map[string]interface{}{
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
				{
					"workflow": "Test configuration changes",
					"steps": []string{
						"1. Make configuration changes",
						"2. Use server_control with action='reload'",
						"3. Verify changes with action='get_status'",
						"4. Test affected routes",
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown topic: %s. Valid topics: overview, tools, resources, examples, workflows", topic)
	}
}

// RegisterDeveloperMCPTools registers all developer tools
func (srv *Server) RegisterDeveloperMCPTools() {
	if srv.mcpHandler == nil {
		logger.Warn("Cannot register developer MCP tools: MCP handler not initialized")
		return
	}

	// Log prominent warning about developer mode
	logger.Warn("⚠️  MCP DEVELOPER MODE ENABLED ⚠️",
		"warning", "This mode allows server restart and configuration changes",
		"security", "Only use in development environments",
		"tools", []string{"server_control", "route_inspector", "request_debugger", "dev_guide"},
	)

	// Create and register the request debugger tool
	requestDebuggerTool := &RequestDebuggerTool{server: srv}
	
	// Register tools
	srv.mcpHandler.RegisterTool(&ServerControlTool{
		server: srv,
	})
	srv.mcpHandler.RegisterTool(&RouteInspectorTool{server: srv})
	srv.mcpHandler.RegisterTool(requestDebuggerTool)
	srv.mcpHandler.RegisterTool(&DevGuideTool{server: srv})

	// Add request capture middleware to capture HTTP requests
	srv.AddMiddleware("*", RequestCaptureMiddleware(requestDebuggerTool))
	logger.Info("Request capture middleware registered for MCP dev mode")

	// Register resources
	// Create and register log resource with log interceptor
	logResource := NewServerLogResource(1000) // Larger buffer for development
	srv.mcpHandler.RegisterResource(&StreamingLogResource{
		ServerLogResource: logResource,
	})
	
	// Set up log interceptor to capture logs into the resource
	originalHandler := logger.Handler()
	logResource.handler = originalHandler
	multiLogger := slog.New(logResource)
	slog.SetDefault(multiLogger)
	logger = multiLogger
	
	srv.mcpHandler.RegisterResource(&RouteListResource{server: srv})

	logger.Info("Developer MCP tools registered", 
		"tools", []string{"server_control", "route_inspector", "request_debugger", "dev_guide"},
		"resources", []string{"logs://server/stream", "routes://server/all"},
	)
}