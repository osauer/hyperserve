// Example: Using command-line flags to configure MCP
//
// This example shows how to use flags and environment variables
// to configure MCP support without hardcoding development settings.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
	_ "github.com/osauer/hyperserve/v2/mcp/builtin" // register builtin preset hooks
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// APPLICATION handles flag parsing, not hyperserve
	var (
		mcpDev           = flag.Bool("mcp-dev", false, "Enable MCP developer tools")
		mcpObservability = flag.Bool("mcp-observability", false, "Enable MCP observability")
		mcpTransport     = flag.String("mcp-transport", "http", "MCP transport (http|stdio)")
		port             = flag.String("port", "", "Override server port")
	)

	flag.Parse()
	if *mcpTransport != "http" && *mcpTransport != "stdio" {
		log.Fatalf("unsupported MCP transport %q (want http or stdio)", *mcpTransport)
	}

	// Bind environment first; application flags appended below take precedence.
	opts := []hyperserve.Option{hyperserve.WithEnvironment()}
	if *port != "" {
		opts = append(opts, hyperserve.WithAddr(":"+*port))
	}

	// Configure MCP based on flags
	if *mcpDev || *mcpObservability {
		var mcpConfigs []mcp.TransportConfig

		if *mcpTransport == "stdio" {
			mcpConfigs = append(mcpConfigs, mcp.OverStdio())
		}

		if *mcpDev {
			mcpConfigs = append(mcpConfigs, hyperserve.MCPDev())
		}

		if *mcpObservability {
			mcpConfigs = append(mcpConfigs, hyperserve.MCPObservability())
		}

		opts = append(opts, hyperserve.WithMCPSupport("MyApp", "1.0.0", mcpConfigs...))
	}

	// Create server with options
	app, err := hyperserve.New(opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Add some example routes
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from HyperServe!")
		fmt.Fprintln(w, "")
		if app.MCPEnabled() {
			fmt.Fprintln(w, "MCP is enabled:")
			fmt.Fprintf(w, "- Transport: %s\n", transportName(app.Options().MCPTransport))
			fmt.Fprintf(w, "- Developer Mode: %v\n", app.Options().MCPDev)
			fmt.Fprintf(w, "- Observability: %v\n", app.Options().MCPObservability)
		} else {
			fmt.Fprintln(w, "MCP is not enabled. Use --mcp-dev or --mcp-observability to enable.")
		}
	})

	app.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "ok", "server": "hyperserve"}`)
	})

	// Print usage information
	printUsage(app)

	// Run the server
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func transportName(transport mcp.TransportType) string {
	if transport == mcp.StdioTransport {
		return "stdio"
	}
	return "http"
}

func printUsage(app *hyperserve.Server) {
	log.Println("Server starting on", app.Options().Addr)

	if !app.MCPEnabled() {
		log.Println("MCP is disabled. To enable, use:")
		log.Println("  ./mcp-flags --mcp-dev                  # For development")
		log.Println("  ./mcp-flags --mcp-observability        # Read-only; protect the endpoint")
		log.Println("  ./mcp-flags --mcp-dev --mcp-transport=stdio  # For Claude Desktop")
		return
	}

	log.Println("MCP is enabled")
	if app.Options().MCPTransport == mcp.StdioTransport {
		log.Println("Running in STDIO mode for Claude Desktop")
		log.Println("Configure Claude Desktop with:")
		log.Println(`{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/mcp-flags",
      "args": ["--mcp-dev", "--mcp-transport=stdio"]
    }
  }
}`)
	} else {
		log.Printf("MCP endpoint %s on %s", app.Options().MCPEndpoint, app.Options().Addr)
		if app.Options().MCPDev {
			log.Println("Developer tools enabled - use Claude Code to:")
			log.Println("  - Inspect server status")
			log.Println("  - Inspect routes and middleware")
		}
		if app.Options().MCPObservability {
			log.Println("Observability resources enabled:")
			log.Println("  - Server configuration (sanitized)")
			log.Println("  - Health metrics")
			log.Println("  - Recent logs")
		}
	}
}
