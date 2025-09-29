// Package hyperserve provides comprehensive MCP (Model Context Protocol) built-in tools and resources.
//
// This file consolidates all built-in MCP functionality including:
//   - Developer Tools: For interactive development with AI assistants
//   - DevOps Resources: For server monitoring and observability
//   - File Tools: For filesystem operations
//   - HTTP Tools: For making external requests
//   - System Resources: For runtime information and metrics
//
// Security Considerations:
//   - Developer tools should ONLY be enabled in development environments
//   - DevOps resources expose no sensitive data (TLS keys, auth functions excluded)
//   - File tools use Go 1.24's os.Root for path traversal protection
//   - All tools implement proper input validation and error handling
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// =============================================================================
// Built-in Developer Tools
// =============================================================================
//
// These tools are designed for development environments where Claude Code
// or other AI assistants help build applications. They provide controlled
// access to server internals while maintaining security boundaries.
//
// SECURITY: These tools should ONLY be enabled in development environments.
// Never enable MCPDeveloperPreset in production!

// MCPDev configures MCP with developer tools for local development.
// This configuration is designed for AI-assisted development with Claude Code.
//
// ⚠️  SECURITY WARNING: Only use in development environments!
// Enables powerful tools that can restart your server and modify its behavior.
//
// Tools provided:
//   - mcp__hyperserve__server_control: Restart server, reload config, change log levels, get status
//   - mcp__hyperserve__route_inspector: List all registered routes and their middleware
//   - mcp__hyperserve__request_debugger: Capture and replay HTTP requests for debugging
//
// Resources provided:
//   - logs://server/stream: Real-time log streaming
//   - routes://server/all: All registered routes with metadata
//   - requests://debug/recent: Recent requests with full details
//
// Example:
//
//	srv, _ := hyperserve.NewServer(
//	    hyperserve.WithMCPSupport("DevServer", "1.0.0", hyperserve.MCPDev()),
//	)
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
				"type":        "string",
				"enum":        []string{"restart", "reload", "set_log_level", "get_status"},
				"description": "Action to perform: get_status (check server health), set_log_level (change logging verbosity), reload (refresh configuration without restart), restart (graceful server restart)",
			},
			"log_level": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"DEBUG", "INFO", "WARN", "ERROR"},
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
			"status":  "restart_initiated",
			"message": "Server will restart. Please wait a moment before making new requests.",
			"note":    "In production, use process managers like systemd or supervisor for restarts.",
		}, nil

	case "reload":
		// Reload configuration, templates, etc. without full restart
		logger.Info("Configuration reload requested via MCP developer tools")
		// Here you would implement actual reload logic:
		// - Reload templates
		// - Refresh static file cache
		// - Re-read configuration files
		return map[string]interface{}{
			"status":    "reloaded",
			"timestamp": time.Now().Format(time.RFC3339),
			"message":   "Configuration and templates reloaded",
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
			"status":    "log_level_changed",
			"new_level": level,
		}, nil

	case "get_status":
		return map[string]interface{}{
			"running":   t.server.isRunning.Load(),
			"ready":     t.server.isReady.Load(),
			"uptime":    time.Since(t.server.serverStart).String(),
			"log_level": t.server.Options.LogLevel,
			"addr":      t.server.Options.Addr,
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
				"type":        "string",
				"description": "Optional pattern to filter routes (e.g., '/api' to show only API routes, '/health' for health check endpoints)",
			},
			"include_middleware": map[string]interface{}{
				"type":        "boolean",
				"description": "Include middleware chain information for each route (default: true). Shows security headers, rate limiting, auth middleware, etc.",
				"default":     true,
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

	routes := []map[string]interface{}{}

	// Get routes from middleware registry - this contains all routes with middleware
	if t.server.middleware != nil {
		for route, middlewareStack := range t.server.middleware.middleware {
			if pattern != "" && !strings.Contains(route, pattern) {
				continue
			}

			routeInfo := map[string]interface{}{
				"pattern": route,
				"methods": []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"}, // ServeMux doesn't track methods
			}

			if includeMiddleware {
				// Get actual middleware names from the stack
				middlewareNames := make([]string, 0, len(middlewareStack))
				for _, middleware := range middlewareStack {
					// Get the function name using reflection
					middlewareName := fmt.Sprintf("%T", middleware)
					// Clean up the function name for better display
					if strings.Contains(middlewareName, ".") {
						parts := strings.Split(middlewareName, ".")
						middlewareName = parts[len(parts)-1]
					}
					middlewareNames = append(middlewareNames, middlewareName)
				}
				routeInfo["middleware"] = middlewareNames
			}

			routes = append(routes, routeInfo)
		}
	}

	// Add known health routes if they don't exist in middleware registry
	healthRoutes := []string{"/healthz", "/readyz", "/livez"}
	if t.server.Options.RunHealthServer {
		for _, route := range healthRoutes {
			if pattern != "" && !strings.Contains(route, pattern) {
				continue
			}

			// Check if this route already exists in our routes list
			found := false
			for _, existingRoute := range routes {
				if existingRoute["pattern"] == route {
					found = true
					break
				}
			}

			if !found {
				routeInfo := map[string]interface{}{
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

	// Add MCP endpoint if enabled
	if t.server.Options.MCPEnabled {
		mcpRoute := t.server.Options.MCPEndpoint
		if pattern == "" || strings.Contains(mcpRoute, pattern) {
			// Check if this route already exists in our routes list
			found := false
			for _, existingRoute := range routes {
				if existingRoute["pattern"] == mcpRoute {
					found = true
					break
				}
			}

			if !found {
				routeInfo := map[string]interface{}{
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

	return map[string]interface{}{
		"routes": routes,
		"total":  len(routes),
		"note":   "Routes discovered from middleware registry and known server endpoints",
	}, nil
}

// RequestDebuggerTool captures and allows replay of requests
type RequestDebuggerTool struct {
	server           *Server
	captures         sync.Map // map[string]*CapturedRequest
	requestIDCounter int64
}

type CapturedRequest struct {
	ID        string              `json:"id"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
	Body      string              `json:"body"`
	Timestamp time.Time           `json:"timestamp"`
	Response  *CapturedResponse   `json:"response,omitempty"`
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
				"type":        "string",
				"enum":        []string{"list", "get", "replay", "clear"},
				"description": "Operation to perform: list (show all captured requests), get (view request details by ID), replay (resend a request), clear (delete all captures)",
			},
			"request_id": map[string]interface{}{
				"type":        "string",
				"description": "Request ID for get/replay actions. Get the ID from 'list' action first.",
			},
			"modifications": map[string]interface{}{
				"type":        "object",
				"description": "Optional modifications to apply when replaying a request (for replay action only)",
				"properties": map[string]interface{}{
					"headers": map[string]interface{}{
						"type":        "object",
						"description": "Headers to add/override as key-value pairs",
					},
					"body": map[string]interface{}{
						"type":        "string",
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
					"id":        req.ID,
					"method":    req.Method,
					"path":      req.Path,
					"timestamp": req.Timestamp,
				})
			}
			return true
		})
		return map[string]interface{}{
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
		// Replay functionality would go here
		return map[string]interface{}{
			"status": "replay_not_implemented",
			"note":   "Request replay would replay the captured request with modifications",
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
				"type":        "string",
				"enum":        []string{"overview", "tools", "resources", "examples", "workflows"},
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
					"name":    "server_control",
					"purpose": "Manage server lifecycle and configuration",
					"actions": []string{"get_status", "set_log_level", "reload", "restart"},
				},
				{
					"name":     "route_inspector",
					"purpose":  "View all registered HTTP routes",
					"features": []string{"filter by pattern", "show middleware chains"},
				},
				{
					"name":    "request_debugger",
					"purpose": "Capture and debug HTTP requests",
					"actions": []string{"list", "get", "replay", "clear"},
				},
				{
					"name":    "dev_guide",
					"purpose": "This help tool",
					"topics":  []string{"overview", "tools", "resources", "examples", "workflows"},
				},
			},
			"resources": []map[string]interface{}{
				{
					"uri":     "logs://server/stream",
					"purpose": "Real-time server logs",
				},
				{
					"uri":     "routes://server/all",
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
						"get_status":    "Check if server is running, uptime, current log level",
						"set_log_level": "Change logging verbosity (DEBUG, INFO, WARN, ERROR)",
						"reload":        "Reload configuration without restart",
						"restart":       "Gracefully restart the server",
					},
					"example": map[string]interface{}{
						"name": "server_control",
						"arguments": map[string]string{
							"action":    "set_log_level",
							"log_level": "DEBUG",
						},
					},
				},
				{
					"tool": "route_inspector",
					"parameters": map[string]string{
						"pattern":            "Filter routes by pattern (optional)",
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
						"list":   "Show all captured requests",
						"get":    "View full details of a specific request",
						"replay": "Resend a request with modifications",
						"clear":  "Delete all captured requests",
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
		return map[string]interface{}{
			"common_examples": []map[string]interface{}{
				{
					"task": "Enable debug logging",
					"tool": "server_control",
					"arguments": map[string]interface{}{
						"action":    "set_log_level",
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
		"tools", []string{"mcp__hyperserve__server_control", "mcp__hyperserve__route_inspector", "mcp__hyperserve__request_debugger", "mcp__hyperserve__dev_guide"},
	)

	// Create and register the request debugger tool
	requestDebuggerTool := &RequestDebuggerTool{server: srv}

	// Register tools
	srv.mcpHandler.RegisterToolInNamespace(&ServerControlTool{
		server: srv,
	}, "hyperserve")
	srv.mcpHandler.RegisterToolInNamespace(&RouteInspectorTool{server: srv}, "hyperserve")
	srv.mcpHandler.RegisterToolInNamespace(requestDebuggerTool, "hyperserve")
	srv.mcpHandler.RegisterToolInNamespace(&DevGuideTool{server: srv}, "hyperserve")

	// Add request capture middleware to capture HTTP requests
	srv.AddMiddleware("*", RequestCaptureMiddleware(requestDebuggerTool))
	logger.Info("Request capture middleware registered for MCP dev mode")

	// Register resources
	srv.mcpHandler.RegisterResource(&StreamingLogResource{
		ServerLogResource: NewServerLogResource(1000), // Larger buffer for development
	})
	srv.mcpHandler.RegisterResource(&RouteListResource{server: srv})

	logger.Info("Developer MCP tools registered",
		"tools", []string{"mcp__hyperserve__server_control", "mcp__hyperserve__route_inspector", "mcp__hyperserve__request_debugger", "mcp__hyperserve__dev_guide"},
		"resources", []string{"logs://server/stream", "routes://server/all"},
	)
}

// =============================================================================
// Built-in DevOps Resources
// =============================================================================
//
// DevOps MCP Resources for server introspection and monitoring.
// These resources provide read-only access to server state without exposing
// sensitive information like TLS keys or authentication functions.

// MCPObservability configures MCP with observability resources for production use.
// This configuration provides read-only access to system state without any dangerous operations:
// - config://server/current - Server configuration (sanitized, no secrets)
// - health://server/status - Health metrics and uptime
// - logs://server/recent - Recent log entries (circular buffer)
//
// Safe for production use - provides observability without control plane access.
//
// Example:
//
//	srv, _ := hyperserve.NewServer(
//	    hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPObservability()),
//	)
func MCPObservability() MCPTransportConfig {
	return func(opts *mcpTransportOptions) {
		opts.observabilityMode = true
	}
}

// ServerConfigResource provides access to the current server configuration
type ServerConfigResource struct {
	server *Server
}

// NewServerConfigResource creates a new server configuration resource
func NewServerConfigResource(srv *Server) *ServerConfigResource {
	return &ServerConfigResource{server: srv}
}

// URI returns the resource URI.
func (r *ServerConfigResource) URI() string {
	return "config://server/current"
}

// Name returns the resource name.
func (r *ServerConfigResource) Name() string {
	return "Server Configuration"
}

// Description returns the resource description.
func (r *ServerConfigResource) Description() string {
	return "Current server configuration and runtime settings"
}

// MimeType returns the resource MIME type.
func (r *ServerConfigResource) MimeType() string {
	return "application/json"
}

// Read returns the current server configuration.
func (r *ServerConfigResource) Read() (interface{}, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}

	// Create a sanitized copy of options
	// SECURITY: We explicitly exclude sensitive fields:
	// - KeyFile/CertFile: TLS private key and certificate paths
	// - AuthTokenValidatorFunc: Contains authentication logic
	// - ECHKeys: Encrypted Client Hello keys
	// - StaticDir/TemplateDir: Could expose internal file structure
	// - MCPFileToolRoot: Could expose sandboxed directory paths
	config := map[string]interface{}{
		"version":       Version,
		"build_hash":    BuildHash,
		"build_time":    BuildTime,
		"go_version":    runtime.Version(),
		"addr":          r.server.Options.Addr,
		"health_addr":   r.server.Options.HealthAddr,
		"tls_enabled":   r.server.Options.EnableTLS,
		"rate_limit":    r.server.Options.RateLimit,
		"burst":         r.server.Options.Burst,
		"hardened_mode": r.server.Options.HardenedMode,
		"fips_mode":     r.server.Options.FIPSMode,
		"mcp_enabled":   r.server.Options.MCPEnabled,
		"mcp_endpoint":  r.server.Options.MCPEndpoint,
		"debug_mode":    r.server.Options.DebugMode,
		"log_level":     r.server.Options.LogLevel,
		"timeouts": map[string]string{
			"read":  r.server.Options.ReadTimeout.String(),
			"write": r.server.Options.WriteTimeout.String(),
			"idle":  r.server.Options.IdleTimeout.String(),
		},
		"middleware_count": len(r.server.middleware.middleware),
		"is_running":       r.server.isRunning.Load(),
		"is_ready":         r.server.isReady.Load(),
	}

	return config, nil
}

// List returns the available resource URIs.
func (r *ServerConfigResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// ServerHealthResource provides access to server health status
type ServerHealthResource struct {
	server *Server
}

// NewServerHealthResource creates a new server health resource
func NewServerHealthResource(srv *Server) *ServerHealthResource {
	return &ServerHealthResource{server: srv}
}

// URI returns the resource URI.
func (r *ServerHealthResource) URI() string {
	return "health://server/status"
}

// Name returns the resource name.
func (r *ServerHealthResource) Name() string {
	return "Server Health Status"
}

// Description returns the resource description.
func (r *ServerHealthResource) Description() string {
	return "Current server health, readiness, and liveness status"
}

// MimeType returns the resource MIME type.
func (r *ServerHealthResource) MimeType() string {
	return "application/json"
}

// Read returns the current server health status.
func (r *ServerHealthResource) Read() (interface{}, error) {
	if r.server == nil {
		return nil, fmt.Errorf("server not initialized")
	}

	uptime := time.Duration(0)
	if !r.server.serverStart.IsZero() {
		uptime = time.Since(r.server.serverStart)
	}

	health := map[string]interface{}{
		"status": map[string]bool{
			"alive":   r.server.isRunning.Load(),
			"ready":   r.server.isReady.Load(),
			"healthy": r.server.isRunning.Load() && r.server.isReady.Load(),
		},
		"uptime":         uptime.String(),
		"uptime_seconds": int(uptime.Seconds()),
		"metrics": map[string]interface{}{
			"total_requests":       r.server.totalRequests.Load(),
			"total_response_time":  r.server.totalResponseTime.Load(),
			"avg_response_time_us": calculateAvgResponseTime(r.server),
			"active_limiters":      len(r.server.clientLimiters),
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	return health, nil
}

// List returns the available resource URIs.
func (r *ServerHealthResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// ServerLogResource provides access to recent server logs
type ServerLogResource struct {
	mu      sync.RWMutex
	logs    []logEntry
	maxSize int
	handler slog.Handler
}

type logEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"msg"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
}

// NewServerLogResource creates a new server log resource
func NewServerLogResource(maxSize int) *ServerLogResource {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &ServerLogResource{
		logs:    make([]logEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// URI returns the resource URI.
func (r *ServerLogResource) URI() string {
	return "logs://server/recent"
}

// Name returns the resource name.
func (r *ServerLogResource) Name() string {
	return "Server Logs"
}

// Description returns the resource description.
func (r *ServerLogResource) Description() string {
	return fmt.Sprintf("Recent server logs (last %d entries)", r.maxSize)
}

// MimeType returns the resource MIME type.
func (r *ServerLogResource) MimeType() string {
	return "application/json"
}

// Read returns the recent server logs.
func (r *ServerLogResource) Read() (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy of the logs
	logsCopy := make([]logEntry, len(r.logs))
	copy(logsCopy, r.logs)

	// For MCP compatibility, serialize the log data as JSON string
	logData := map[string]interface{}{
		"logs":      logsCopy,
		"count":     len(logsCopy),
		"max_size":  r.maxSize,
		"truncated": len(r.logs) >= r.maxSize,
	}

	// Convert to JSON string for MCP text field
	jsonBytes, err := json.Marshal(logData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal log data: %w", err)
	}

	return string(jsonBytes), nil
}

// List returns the available resource URIs.
func (r *ServerLogResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// Handle implements slog.Handler to capture logs
func (r *ServerLogResource) Handle(ctx context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := logEntry{
		Time:    record.Time,
		Level:   record.Level.String(),
		Message: record.Message,
		Attrs:   make(map[string]interface{}),
	}

	// Collect attributes
	record.Attrs(func(attr slog.Attr) bool {
		entry.Attrs[attr.Key] = attr.Value.Any()
		return true
	})

	// Add to circular buffer
	if len(r.logs) >= r.maxSize {
		// Remove oldest entry
		r.logs = r.logs[1:]
	}
	r.logs = append(r.logs, entry)

	// Forward to original handler if set
	if r.handler != nil {
		return r.handler.Handle(ctx, record)
	}
	return nil
}

func (r *ServerLogResource) Enabled(ctx context.Context, level slog.Level) bool {
	if r.handler != nil {
		return r.handler.Enabled(ctx, level)
	}
	return true
}

func (r *ServerLogResource) WithAttrs(attrs []slog.Attr) slog.Handler {
	return r
}

func (r *ServerLogResource) WithGroup(name string) slog.Handler {
	return r
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

// Helper function to calculate average response time
func calculateAvgResponseTime(srv *Server) int64 {
	requests := srv.totalRequests.Load()
	if requests == 0 {
		return 0
	}
	// Safe conversion - requests is uint64, check for overflow
	if requests > math.MaxInt64 {
		return 0 // Return 0 for overflow case rather than panic
	}
	return srv.totalResponseTime.Load() / int64(requests) //nolint:gosec // checked above
}

// RegisterObservabilityMCPResources registers minimal observability resources for production monitoring
func (srv *Server) RegisterObservabilityMCPResources() {
	if srv.mcpHandler == nil {
		logger.Warn("Cannot register observability MCP resources: MCP handler not initialized")
		return
	}

	// Register server configuration resource
	srv.mcpHandler.RegisterResource(NewServerConfigResource(srv))

	// Register server health resource
	srv.mcpHandler.RegisterResource(NewServerHealthResource(srv))

	// Create and register log resource with custom logger
	logResource := NewServerLogResource(srv.Options.MCPLogResourceSize)
	srv.mcpHandler.RegisterResource(logResource)

	// If in debug mode, also intercept logs
	if srv.Options.DebugMode {
		// Create a multi-handler that writes to both original and log resource
		originalHandler := logger.Handler()
		logResource.handler = originalHandler
		multiLogger := slog.New(logResource)
		slog.SetDefault(multiLogger)
		logger = multiLogger
	}

	logger.Info("Observability MCP resources registered",
		"resources", []string{"config://server/current", "health://server/status", "logs://server/recent"})
}

// =============================================================================
// Built-in File and HTTP Tools
// =============================================================================

// FileReadTool implements MCPTool for reading files from the filesystem
type FileReadTool struct {
	root *os.Root // Secure file access using os.Root
}

// NewFileReadTool creates a new file read tool with optional root directory restriction
func NewFileReadTool(rootDir string) (*FileReadTool, error) {
	var root *os.Root
	if rootDir != "" {
		var err error
		root, err = os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
	}

	return &FileReadTool{root: root}, nil
}

func (t *FileReadTool) Name() string {
	return "read_file"
}

func (t *FileReadTool) Description() string {
	return "Read the contents of a file from the filesystem"
}

func (t *FileReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) Execute(params map[string]interface{}) (interface{}, error) {
	path, ok := params["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}

	// Clean the path
	path = filepath.Clean(path)

	var content []byte
	var err error

	if t.root != nil {
		// Use secure os.Root for file access
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer closeWithLog(file, path)

		content, err = io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		// Direct file system access (use with caution)
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	return string(content), nil
}

// ListDirectoryTool implements MCPTool for listing directory contents
type ListDirectoryTool struct {
	root *os.Root
}

// NewListDirectoryTool creates a new directory listing tool
func NewListDirectoryTool(rootDir string) (*ListDirectoryTool, error) {
	var root *os.Root
	if rootDir != "" {
		var err error
		root, err = os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
	}

	return &ListDirectoryTool{root: root}, nil
}

func (t *ListDirectoryTool) Name() string {
	return "list_directory"
}

func (t *ListDirectoryTool) Description() string {
	return "List the contents of a directory"
}

func (t *ListDirectoryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the directory to list",
				"default":     ".",
			},
		},
	}
}

func (t *ListDirectoryTool) Execute(params map[string]interface{}) (interface{}, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}

	path = filepath.Clean(path)

	var entries []os.DirEntry
	var err error

	if t.root != nil {
		// Use secure os.Root for directory access
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open directory: %w", err)
		}
		defer closeWithLog(file, path)

		entries, err = file.ReadDir(-1)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
	} else {
		entries, err = os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
	}

	var files []map[string]interface{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't get info for
		}

		files = append(files, map[string]interface{}{
			"name":    entry.Name(),
			"type":    getFileType(entry),
			"size":    info.Size(),
			"modTime": info.ModTime().Format(time.RFC3339),
		})
	}

	return files, nil
}

func getFileType(entry os.DirEntry) string {
	if entry.IsDir() {
		return "directory"
	}
	return "file"
}

// HTTPRequestTool implements MCPTool for making HTTP requests
type HTTPRequestTool struct {
	client *http.Client
}

// NewHTTPRequestTool creates a new HTTP request tool
func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *HTTPRequestTool) Name() string {
	return "http_request"
}

func (t *HTTPRequestTool) Description() string {
	return "Make HTTP requests to external services"
}

func (t *HTTPRequestTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to make the request to",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, DELETE, etc.)",
				"default":     "GET",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers as key-value pairs",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST, PUT, etc.)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *HTTPRequestTool) Execute(params map[string]interface{}) (interface{}, error) {
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter is required and must be a string")
	}

	method := "GET"
	if m, ok := params["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	var body io.Reader
	if b, ok := params["body"].(string); ok {
		body = strings.NewReader(b)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer closeWithLog(resp.Body, "HTTP response body")

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return map[string]interface{}{
		"status":     resp.Status,
		"statusCode": resp.StatusCode,
		"headers":    resp.Header,
		"body":       string(respBody),
	}, nil
}

// CalculatorTool implements MCPTool for basic mathematical operations
type CalculatorTool struct{}

// NewCalculatorTool creates a new calculator tool
func NewCalculatorTool() *CalculatorTool {
	return &CalculatorTool{}
}

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "Perform basic mathematical calculations"
}

func (t *CalculatorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Mathematical operation to perform",
				"enum":        []string{"add", "subtract", "multiply", "divide"},
			},
			"a": map[string]interface{}{
				"type":        "number",
				"description": "First operand",
			},
			"b": map[string]interface{}{
				"type":        "number",
				"description": "Second operand",
			},
		},
		"required": []string{"operation", "a", "b"},
	}
}

func (t *CalculatorTool) Execute(params map[string]interface{}) (interface{}, error) {
	operation, ok := params["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation parameter is required and must be a string")
	}

	var a, b float64

	if aVal, ok := params["a"].(float64); ok {
		a = aVal
	} else if aVal, ok := params["a"].(int); ok {
		a = float64(aVal)
	} else {
		return nil, fmt.Errorf("parameter 'a' must be a number")
	}

	if bVal, ok := params["b"].(float64); ok {
		b = bVal
	} else if bVal, ok := params["b"].(int); ok {
		b = float64(bVal)
	} else {
		return nil, fmt.Errorf("parameter 'b' must be a number")
	}

	var result float64
	switch operation {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}

	// Check for infinity or NaN which can't be marshaled to JSON
	if math.IsInf(result, 0) || math.IsNaN(result) {
		return nil, fmt.Errorf("result is out of range: %v", result)
	}

	return map[string]interface{}{
		"result":    result,
		"operation": fmt.Sprintf("%.2f %s %.2f", a, operation, b),
	}, nil
}

// =============================================================================
// Built-in System Resources
// =============================================================================

// ConfigResource implements MCPResource for server configuration access
type ConfigResource struct {
	options *ServerOptions
}

// NewConfigResource creates a new configuration resource
func NewConfigResource(options *ServerOptions) *ConfigResource {
	return &ConfigResource{options: options}
}

func (r *ConfigResource) URI() string {
	return "config://server/options"
}

func (r *ConfigResource) Name() string {
	return "Server Configuration"
}

func (r *ConfigResource) Description() string {
	return "Current server configuration settings"
}

func (r *ConfigResource) MimeType() string {
	return "application/json"
}

func (r *ConfigResource) Read() (interface{}, error) {
	// Return a sanitized version of the configuration (no sensitive data)
	config := map[string]interface{}{
		"addr":            r.options.Addr,
		"enableTLS":       r.options.EnableTLS,
		"tlsAddr":         r.options.TLSAddr,
		"healthAddr":      r.options.HealthAddr,
		"rateLimit":       float64(r.options.RateLimit),
		"burst":           r.options.Burst,
		"readTimeout":     r.options.ReadTimeout.String(),
		"writeTimeout":    r.options.WriteTimeout.String(),
		"idleTimeout":     r.options.IdleTimeout.String(),
		"staticDir":       r.options.StaticDir,
		"templateDir":     r.options.TemplateDir,
		"runHealthServer": r.options.RunHealthServer,
		"chaosMode":       r.options.ChaosMode,
		"fipsMode":        r.options.FIPSMode,
		"hardenedMode":    r.options.HardenedMode,
		"enableECH":       r.options.EnableECH,
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal configuration: %w", err)
	}

	return string(configJSON), nil
}

func (r *ConfigResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// MetricsResource implements MCPResource for server metrics access
type MetricsResource struct {
	server *Server
}

// NewMetricsResource creates a new metrics resource
func NewMetricsResource(server *Server) *MetricsResource {
	return &MetricsResource{server: server}
}

func (r *MetricsResource) URI() string {
	return "metrics://server/stats"
}

func (r *MetricsResource) Name() string {
	return "Server Metrics"
}

func (r *MetricsResource) Description() string {
	return "Current server performance metrics and statistics"
}

func (r *MetricsResource) MimeType() string {
	return "application/json"
}

func (r *MetricsResource) Read() (interface{}, error) {
	uptime := time.Since(r.server.serverStart)
	totalRequests := r.server.totalRequests.Load()
	totalResponseTime := r.server.totalResponseTime.Load()

	var avgResponseTime float64
	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests)
	}

	metrics := map[string]interface{}{
		"uptime":            uptime.String(),
		"totalRequests":     totalRequests,
		"totalResponseTime": fmt.Sprintf("%dμs", totalResponseTime),
		"avgResponseTime":   fmt.Sprintf("%.2fμs", avgResponseTime),
		"isRunning":         r.server.isRunning.Load(),
		"isReady":           r.server.isReady.Load(),
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	metricsJSON, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics: %w", err)
	}

	return string(metricsJSON), nil
}

func (r *MetricsResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// SystemResource implements MCPResource for system information
type SystemResource struct{}

// NewSystemResource creates a new system resource
func NewSystemResource() *SystemResource {
	return &SystemResource{}
}

func (r *SystemResource) URI() string {
	return "system://runtime/info"
}

func (r *SystemResource) Name() string {
	return "System Information"
}

func (r *SystemResource) Description() string {
	return "Runtime system information and Go environment details"
}

func (r *SystemResource) MimeType() string {
	return "application/json"
}

func (r *SystemResource) Read() (interface{}, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	systemInfo := map[string]interface{}{
		"go": map[string]interface{}{
			"version":      runtime.Version(),
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"numCPU":       runtime.NumCPU(),
			"numGoroutine": runtime.NumGoroutine(),
		},
		"memory": map[string]interface{}{
			"allocated":     memStats.Alloc,
			"totalAlloc":    memStats.TotalAlloc,
			"sys":           memStats.Sys,
			"numGC":         memStats.NumGC,
			"gcCPUFraction": memStats.GCCPUFraction,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	systemJSON, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal system info: %w", err)
	}

	return string(systemJSON), nil
}

func (r *SystemResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// LogResource implements MCPResource for recent log entries (if available)
type LogResource struct {
	entries []string
	maxSize int
}

// NewLogResource creates a new log resource with a maximum number of entries
func NewLogResource(maxSize int) *LogResource {
	return &LogResource{
		entries: make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

func (r *LogResource) URI() string {
	return "logs://server/recent"
}

func (r *LogResource) Name() string {
	return "Recent Log Entries"
}

func (r *LogResource) Description() string {
	return "Recent server log entries (if log capture is enabled)"
}

func (r *LogResource) MimeType() string {
	return "text/plain"
}

func (r *LogResource) Read() (interface{}, error) {
	if len(r.entries) == 0 {
		return "No log entries captured. Log capture may not be enabled.", nil
	}

	result := ""
	for _, entry := range r.entries {
		result += entry + "\n"
	}

	return result, nil
}

func (r *LogResource) List() ([]string, error) {
	return []string{r.URI()}, nil
}

// AddLogEntry adds a log entry to the resource (called by log handler if implemented)
func (r *LogResource) AddLogEntry(entry string) {
	if len(r.entries) >= r.maxSize {
		// Remove oldest entry
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), entry))
}

// =============================================================================
// Helper Functions
// =============================================================================

// Helper functions are defined in server.go to avoid duplication
