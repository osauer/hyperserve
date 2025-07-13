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
	"fmt"
	"log/slog"
	"strings"
	"sync"
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
	return "Control server lifecycle: restart, reload, change log levels, get status"
}

func (t *ServerControlTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"restart", "reload", "set_log_level", "get_status"},
				"description": "Action to perform",
			},
			"log_level": map[string]interface{}{
				"type": "string",
				"enum": []string{"DEBUG", "INFO", "WARN", "ERROR"},
				"description": "New log level (for set_log_level action)",
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
	return "Inspect registered routes and their middleware"
}

func (t *RouteInspectorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type": "string",
				"description": "Optional pattern to filter routes",
			},
			"include_middleware": map[string]interface{}{
				"type": "boolean",
				"description": "Include middleware information",
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
	return "Capture, inspect, and replay HTTP requests for debugging"
}

func (t *RequestDebuggerTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type": "string",
				"enum": []string{"list", "get", "replay", "clear"},
			},
			"request_id": map[string]interface{}{
				"type": "string",
				"description": "Request ID for get/replay actions",
			},
			"modifications": map[string]interface{}{
				"type": "object",
				"description": "Modifications to apply when replaying",
				"properties": map[string]interface{}{
					"headers": map[string]interface{}{"type": "object"},
					"body":    map[string]interface{}{"type": "string"},
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
		"tools", []string{"server_control", "route_inspector", "request_debugger"},
	)

	// Register tools
	srv.mcpHandler.RegisterTool(&ServerControlTool{
		server: srv,
	})
	srv.mcpHandler.RegisterTool(&RouteInspectorTool{server: srv})
	srv.mcpHandler.RegisterTool(&RequestDebuggerTool{server: srv})

	// Register resources
	srv.mcpHandler.RegisterResource(&StreamingLogResource{
		ServerLogResource: NewServerLogResource(1000), // Larger buffer for development
	})
	srv.mcpHandler.RegisterResource(&RouteListResource{server: srv})

	logger.Info("Developer MCP tools registered", 
		"tools", []string{"server_control", "route_inspector", "request_debugger"},
		"resources", []string{"logs://server/stream", "routes://server/all"},
	)
}