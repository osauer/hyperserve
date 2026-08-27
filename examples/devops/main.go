// Example demonstrating DevOps features: debug logging and MCP resources
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
	_ "github.com/osauer/hyperserve/v2/mcp/builtin" // register builtin preset hooks
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Check if we should use STDIO transport for MCP
	useStdio := slices.Contains(os.Args[1:], "--mcp-stdio")

	// Create server options
	var opts []hyperserve.Option

	// Configure MCP with appropriate transport
	if useStdio {
		// For Claude Desktop - use STDIO transport with observability
		opts = append(opts, hyperserve.WithMCPSupport("ObservabilityExample", "1.0.0",
			mcp.OverStdio(),
			hyperserve.MCPObservability(),
		))
	} else {
		// For HTTP - use default transport with observability
		opts = append(opts, hyperserve.WithMCPSupport("ObservabilityExample", "1.0.0",
			hyperserve.MCPObservability(),
		))
	}

	// Create server
	app, err := hyperserve.New(opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Example endpoints
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Home page accessed", "path", r.URL.Path, "method", r.Method)
		fmt.Fprintln(w, "DevOps Example Server")
	})

	app.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Test endpoint hit", "remote", r.RemoteAddr)
		fmt.Fprintln(w, "Test endpoint")
	})

	app.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		slog.Error("Simulated error", "endpoint", "/error", "user_agent", r.UserAgent())
		http.Error(w, "Simulated error", http.StatusInternalServerError)
	})

	// Log startup information
	slog.Info("Starting DevOps example server",
		"debug_mode", app.Options().DebugMode,
		"log_level", app.Options().LogLevel,
		"mcp_enabled", app.Options().MCPEnabled,
	)

	// Run the server
	if useStdio {
		log.Println("Starting in MCP STDIO mode...")
		if err := app.RunStdio(); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("Starting server on %s", app.Options().Addr)
		log.Printf("MCP endpoint available at: http://localhost%s%s", app.Options().Addr, app.Options().MCPEndpoint)
		if err := app.Run(ctx); err != nil {
			log.Fatal(err)
		}
	}
}
