package builtin

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

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

// registerBuiltinTools registers the general-purpose tools (calculator plus
// sandboxed file tools when MCPFileToolRoot is configured) under the
// "hyperserve" namespace. File tools require a non-empty
// Options.MCPFileToolRoot — if unset, they are skipped with a warning rather
// than silently registered against the host filesystem.
//
// The http_request tool that previously shipped here was removed: it allowed
// any unauthenticated MCP client to make outbound HTTP calls from the server
// process (SSRF / cloud metadata exfil). If you need outbound HTTP, register
// a domain-allowlisted tool from your own code.
func registerBuiltinTools(srv *server.Server) {
	h := srv.MCPHandler()
	if h == nil {
		return
	}
	if srv.Options.MCPFileToolRoot != "" {
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
	} else {
		logger.Warn("Builtin file tools (read_file, list_directory) not registered: no sandbox root configured",
			"fix", "set WithMCPFileToolRoot(\"/path/to/safe/dir\") to enable")
	}
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
	logResource := NewServerLogResource(srv.Options.MCPLogResourceSize)
	h.RegisterResource(logResource)
	wireLogResource(logResource)
}

// registerObservability wires the minimal "observability" preset onto an MCP
// handler:
//   - config://server/current
//   - health://server/status
//   - logs://server/recent
//
// The log resource intercepts the default logger so /recent reflects live
// output. MCPObservability is a production preset, so this is not gated on
// DebugMode.
func registerObservability(srv *server.Server, handler *mcp.Handler) {
	if handler == nil {
		logger.Warn("Cannot register observability MCP resources: MCP handler not initialized")
		return
	}

	handler.RegisterResource(NewServerConfigResource(srv))
	handler.RegisterResource(NewServerHealthResource(srv))

	logResource := NewServerLogResource(srv.Options.MCPLogResourceSize)
	handler.RegisterResource(logResource)
	wireLogResource(logResource)

	logger.Info("Observability MCP resources registered",
		"resources", []string{"config://server/current", "health://server/status", "logs://server/recent"})
}

func registerObservabilityResources(srv *server.Server) {
	registerObservability(srv, srv.MCPHandler())
}

// registerDeveloper wires the developer preset onto an MCP handler:
//   - tools: server_control, route_inspector, dev_guide
//   - resources: logs://server/stream, routes://server/all
//
// The request_debugger tool that previously shipped here was removed: it
// captured every request's headers (including Authorization, Cookie, API
// keys) into a process-wide store readable by any MCP caller. If you need
// request inspection in development, wire a per-route handler that scrubs
// credentials before logging.
func registerDeveloper(srv *server.Server, handler *mcp.Handler) {
	if handler == nil {
		logger.Warn("Cannot register developer MCP tools: MCP handler not initialized")
		return
	}

	handler.RegisterToolInNamespace(NewServerControlTool(srv), "hyperserve")
	handler.RegisterToolInNamespace(NewRouteInspectorTool(srv), "hyperserve")
	handler.RegisterToolInNamespace(NewDevGuideTool(srv), "hyperserve")

	logResource := NewServerLogResource(1000)
	handler.RegisterResource(&StreamingLogResource{ServerLogResource: logResource})
	wireLogResource(logResource)
	handler.RegisterResource(NewRouteListResource(srv))

	logger.Info("Developer MCP tools registered",
		"tools", []string{
			"mcp__hyperserve__server_control",
			"mcp__hyperserve__route_inspector",
			"mcp__hyperserve__dev_guide",
		},
		"resources", []string{"logs://server/stream", "routes://server/all"},
	)
}

func registerDeveloperPreset(srv *server.Server) {
	registerDeveloper(srv, srv.MCPHandler())
}

func wireLogResource(logResource *ServerLogResource) {
	if logResource == nil {
		return
	}
	logResource.handler = downstreamLogHandler(server.DefaultLogger().Handler())
	multi := slog.New(logResource)
	slog.SetDefault(multi)
	server.SetDefaultLogger(multi)
	logger = multi
}

func downstreamLogHandler(handler slog.Handler) slog.Handler {
	// slog's package default handler writes through the standard log package.
	// slog.SetDefault redirects that package back into slog, so forwarding to
	// the old default handler would recurse. Use an equivalent direct stderr
	// text handler for that case; preserve explicit user/server handlers.
	if strings.Contains(fmt.Sprintf("%T", handler), "defaultHandler") {
		return slog.NewTextHandler(os.Stderr, nil)
	}
	return handler
}
