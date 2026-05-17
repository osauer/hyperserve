package builtin

import (
	"log/slog"

	"github.com/osauer/hyperserve/pkg/mcp"
	"github.com/osauer/hyperserve/pkg/server"
)

// init wires the builtin presets into pkg/server's auto-registration flow.
// When a user imports this package (directly or via `server.MCPDev()` /
// `server.MCPObservability()` shortcuts, which reference these names), the
// hooks become active and NewServer auto-registers the appropriate tools
// and resources based on the configured preset.
func init() {
	server.SetBuiltinPresetHooks(
		registerBuiltinTools,
		registerStandardResources,
		registerObservabilityResources,
		registerDeveloperPreset,
	)
}

// registerBuiltinTools registers the four general-purpose tools
// (read_file, list_directory, http_request, calculator) under the
// "hyperserve" namespace. File tools are sandboxed when MCPFileToolRoot is
// configured.
func registerBuiltinTools(srv *server.Server) {
	h := srv.MCPHandler()
	if h == nil {
		return
	}
	if fileReadTool, err := NewFileReadTool(srv.Options.MCPFileToolRoot); err != nil {
		logger.Warn("Failed to create file read tool", "error", err)
	} else {
		h.RegisterToolInNamespace(fileReadTool, "hyperserve")
	}
	if listDirTool, err := NewListDirectoryTool(srv.Options.MCPFileToolRoot); err != nil {
		logger.Warn("Failed to create list directory tool", "error", err)
	} else {
		h.RegisterToolInNamespace(listDirTool, "hyperserve")
	}
	h.RegisterToolInNamespace(NewHTTPRequestTool(), "hyperserve")
	h.RegisterToolInNamespace(NewCalculatorTool(), "hyperserve")
}

// registerStandardResources installs the standard built-in resources:
// generic config, metrics, system runtime info, and a recent-log buffer.
func registerStandardResources(srv *server.Server) {
	h := srv.MCPHandler()
	if h == nil {
		return
	}
	h.RegisterResource(NewConfigResource(srv.Options))
	h.RegisterResource(NewMetricsResource(srv))
	h.RegisterResource(NewSystemResource())
	h.RegisterResource(NewServerLogResource(srv.Options.MCPLogResourceSize))
}

// RegisterObservability wires the minimal "observability" preset onto an MCP
// handler:
//   - config://server/current
//   - health://server/status
//   - logs://server/recent
//
// When server debug mode is enabled, the log resource also intercepts the
// default logger so /recent reflects live output.
func RegisterObservability(srv *server.Server, handler *mcp.Handler) {
	if handler == nil {
		logger.Warn("Cannot register observability MCP resources: MCP handler not initialized")
		return
	}

	handler.RegisterResource(NewServerConfigResource(srv))
	handler.RegisterResource(NewServerHealthResource(srv))

	logResource := NewServerLogResource(srv.Options.MCPLogResourceSize)
	handler.RegisterResource(logResource)

	if srv.Options.DebugMode {
		original := server.DefaultLogger()
		logResource.handler = original.Handler()
		multi := slog.New(logResource)
		slog.SetDefault(multi)
		server.SetDefaultLogger(multi)
	}

	logger.Info("Observability MCP resources registered",
		"resources", []string{"config://server/current", "health://server/status", "logs://server/recent"})
}

func registerObservabilityResources(srv *server.Server) {
	RegisterObservability(srv, srv.MCPHandler())
}

// RegisterDeveloper wires the developer preset onto an MCP handler:
//   - tools: server_control, route_inspector, request_debugger, dev_guide
//   - resources: logs://server/stream, routes://server/all
//   - middleware: request capture (globally) for the debugger
func RegisterDeveloper(srv *server.Server, handler *mcp.Handler) {
	if handler == nil {
		logger.Warn("Cannot register developer MCP tools: MCP handler not initialized")
		return
	}

	// The big "MCP DEVELOPER MODE ENABLED" warning is emitted by pkg/server's
	// NewServer when the dev preset is enabled, regardless of whether this
	// hook gets installed. We just log what we register.

	debugger := NewRequestDebuggerTool(srv)

	handler.RegisterToolInNamespace(NewServerControlTool(srv), "hyperserve")
	handler.RegisterToolInNamespace(NewRouteInspectorTool(srv), "hyperserve")
	handler.RegisterToolInNamespace(debugger, "hyperserve")
	handler.RegisterToolInNamespace(NewDevGuideTool(srv), "hyperserve")

	srv.AddMiddleware("*", RequestCaptureMiddleware(debugger))
	logger.Info("Request capture middleware registered for MCP dev mode")

	handler.RegisterResource(&StreamingLogResource{
		ServerLogResource: NewServerLogResource(1000),
	})
	handler.RegisterResource(NewRouteListResource(srv))

	logger.Info("Developer MCP tools registered",
		"tools", []string{
			"mcp__hyperserve__server_control",
			"mcp__hyperserve__route_inspector",
			"mcp__hyperserve__request_debugger",
			"mcp__hyperserve__dev_guide",
		},
		"resources", []string{"logs://server/stream", "routes://server/all"},
	)
}

func registerDeveloperPreset(srv *server.Server) {
	RegisterDeveloper(srv, srv.MCPHandler())
}
