// Example: Using command-line flags to configure MCP
//
// This example shows how to use flags and environment variables
// to configure MCP support without hardcoding development settings.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/osauer/hyperserve/pkg/mcp"
	_ "github.com/osauer/hyperserve/pkg/mcp/builtin" // register builtin preset hooks
	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
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

	// Leaving a flag unset preserves file and environment configuration.
	var opts []serverpkg.ServerOptionFunc
	if *port != "" {
		opts = append(opts, serverpkg.WithAddr(":"+*port))
	}

	// Configure MCP based on flags
	if *mcpDev || *mcpObservability {
		var mcpConfigs []mcp.TransportConfig

		if *mcpTransport == "stdio" {
			mcpConfigs = append(mcpConfigs, mcp.OverStdio())
		}

		if *mcpDev {
			mcpConfigs = append(mcpConfigs, serverpkg.MCPDev())
		}

		if *mcpObservability {
			mcpConfigs = append(mcpConfigs, serverpkg.MCPObservability())
		}

		opts = append(opts, serverpkg.WithMCPSupport("MyApp", "1.0.0", mcpConfigs...))
	}

	// Create server with options
	srv, err := serverpkg.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}

	// Add some example routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from HyperServe!")
		fmt.Fprintln(w, "")
		if srv.MCPEnabled() {
			fmt.Fprintln(w, "MCP is enabled:")
			fmt.Fprintf(w, "- Transport: %s\n", transportName(srv.Options.MCPTransport))
			fmt.Fprintf(w, "- Developer Mode: %v\n", srv.Options.MCPDev)
			fmt.Fprintf(w, "- Observability: %v\n", srv.Options.MCPObservability)
		} else {
			fmt.Fprintln(w, "MCP is not enabled. Use --mcp-dev or --mcp-observability to enable.")
		}
	})

	srv.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "ok", "server": "hyperserve"}`)
	})

	// Print usage information
	printUsage(srv)

	// Run the server
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

func transportName(transport mcp.TransportType) string {
	if transport == mcp.StdioTransport {
		return "stdio"
	}
	return "http"
}

func printUsage(srv *serverpkg.Server) {
	log.Println("Server starting on", srv.Options.Addr)

	if !srv.MCPEnabled() {
		log.Println("MCP is disabled. To enable, use:")
		log.Println("  ./mcp-flags --mcp-dev                  # For development")
		log.Println("  ./mcp-flags --mcp-observability        # For production monitoring")
		log.Println("  ./mcp-flags --mcp-dev --mcp-transport=stdio  # For Claude Desktop")
		return
	}

	log.Println("MCP is enabled")
	if srv.Options.MCPTransport == mcp.StdioTransport {
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
		log.Printf("MCP endpoint %s on %s", srv.Options.MCPEndpoint, srv.Options.Addr)
		if srv.Options.MCPDev {
			log.Println("Developer tools enabled - use Claude Code to:")
			log.Println("  - Set log levels dynamically")
			log.Println("  - Inspect routes and middleware")
		}
		if srv.Options.MCPObservability {
			log.Println("Observability resources enabled:")
			log.Println("  - Server configuration (sanitized)")
			log.Println("  - Health metrics")
			log.Println("  - Recent logs")
		}
	}
}
