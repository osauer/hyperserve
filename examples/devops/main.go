// Example demonstrating DevOps features: debug logging and MCP resources
package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/osauer/hyperserve/pkg/mcp"
	_ "github.com/osauer/hyperserve/pkg/mcp/builtin" // register builtin preset hooks
	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	// Check if we should use STDIO transport for MCP
	useStdio := slices.Contains(os.Args[1:], "--mcp-stdio")

	// Create server options
	var opts []serverpkg.ServerOptionFunc

	// Configure MCP with appropriate transport
	if useStdio {
		// For Claude Desktop - use STDIO transport with observability
		opts = append(opts, serverpkg.WithMCPSupport("ObservabilityExample", "1.0.0",
			mcp.OverStdio(),
			serverpkg.MCPObservability(),
		))
	} else {
		// For HTTP - use default transport with observability
		opts = append(opts, serverpkg.WithMCPSupport("ObservabilityExample", "1.0.0",
			serverpkg.MCPObservability(),
		))
	}

	// Create server
	srv, err := serverpkg.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Example endpoints
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Home page accessed", "path", r.URL.Path, "method", r.Method)
		fmt.Fprintln(w, "DevOps Example Server")
	})

	srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Test endpoint hit", "remote", r.RemoteAddr)
		fmt.Fprintln(w, "Test endpoint")
	})

	srv.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		slog.Error("Simulated error", "endpoint", "/error", "user_agent", r.UserAgent())
		http.Error(w, "Simulated error", http.StatusInternalServerError)
	})

	// Log startup information
	slog.Info("Starting DevOps example server",
		"debug_mode", srv.Options.DebugMode,
		"log_level", srv.Options.LogLevel,
		"mcp_enabled", srv.Options.MCPEnabled,
	)

	// Run the server
	if useStdio {
		log.Println("Starting in MCP STDIO mode...")
		if err := srv.MCPHandler().RunStdioLoop(); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("Starting server on %s", srv.Options.Addr)
		log.Printf("MCP endpoint available at: http://localhost%s%s", srv.Options.Addr, srv.Options.MCPEndpoint)
		srv.Run()
	}
}
