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
	"os"

	"github.com/osauer/hyperserve"
)

func main() {
	// Define command-line flags
	var (
		mcpEnabled       = flag.Bool("mcp", false, "Enable MCP support")
		mcpName          = flag.String("mcp-name", "MyApp", "MCP application name")
		mcpVersion       = flag.String("mcp-version", "1.0.0", "MCP version")
		mcpTransport     = flag.String("mcp-transport", "http", "MCP transport (http|stdio)")
		mcpDev           = flag.Bool("mcp-dev", false, "Enable MCP developer tools")
		mcpObservability = flag.Bool("mcp-observability", false, "Enable MCP observability")
		port             = flag.String("port", "8080", "Server port")
	)
	
	flag.Parse()
	
	// Configure server options based on flags
	opts := []hyperserve.ServerOptionFunc{
		hyperserve.WithAddr(":" + *port),
	}
	
	// Add MCP support if enabled
	if *mcpEnabled {
		// Set environment variables for MCP configuration
		os.Setenv("HS_MCP_ENABLED", "true")
		os.Setenv("HS_MCP_SERVER_NAME", *mcpName)
		os.Setenv("HS_MCP_SERVER_VERSION", *mcpVersion)
		os.Setenv("HS_MCP_TRANSPORT", *mcpTransport)
		
		if *mcpDev {
			os.Setenv("HS_MCP_DEV", "true")
		}
		if *mcpObservability {
			os.Setenv("HS_MCP_OBSERVABILITY", "true")
		}
	}
	
	// Create server - MCP will be auto-configured from environment
	srv, err := hyperserve.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}
	
	// Add some example routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from HyperServe!")
		fmt.Fprintln(w, "")
		if srv.MCPEnabled() {
			fmt.Fprintln(w, "MCP is enabled with the following configuration:")
			fmt.Fprintf(w, "- Name: %s\n", *mcpName)
			fmt.Fprintf(w, "- Version: %s\n", *mcpVersion)
			fmt.Fprintf(w, "- Transport: %s\n", *mcpTransport)
			fmt.Fprintf(w, "- Developer Mode: %v\n", *mcpDev)
			fmt.Fprintf(w, "- Observability: %v\n", *mcpObservability)
		} else {
			fmt.Fprintln(w, "MCP is not enabled. Use --mcp flag to enable.")
		}
	})
	
	srv.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "ok", "server": "hyperserve"}`)
	})
	
	// Print usage information
	printUsage(*mcpEnabled, *mcpDev, *mcpObservability, *mcpTransport, *port)
	
	// Run the server
	srv.Run()
}

func printUsage(mcpEnabled, mcpDev, mcpObservability bool, transport, port string) {
	log.Println("Server starting on port", port)
	
	if !mcpEnabled {
		log.Println("MCP is disabled. To enable, use:")
		log.Println("  ./mcp-flags --mcp")
		log.Println("")
		log.Println("For development with Claude Code:")
		log.Println("  ./mcp-flags --mcp --mcp-dev")
		log.Println("")
		log.Println("For production monitoring:")
		log.Println("  ./mcp-flags --mcp --mcp-observability")
		log.Println("")
		log.Println("For Claude Desktop integration:")
		log.Println("  ./mcp-flags --mcp --mcp-dev --mcp-transport=stdio")
		return
	}
	
	log.Println("MCP is enabled")
	if transport == "stdio" {
		log.Println("Running in STDIO mode for Claude Desktop")
		log.Println("Configure Claude Desktop with:")
		log.Println(`{
  "mcpServers": {
    "myapp": {
      "command": "/path/to/mcp-flags",
      "args": ["--mcp", "--mcp-dev", "--mcp-transport=stdio"]
    }
  }
}`)
	} else {
		log.Println("MCP endpoint available at: http://localhost:" + port + "/mcp")
		if mcpDev {
			log.Println("Developer tools enabled - use Claude Code to:")
			log.Println("  - Set log levels dynamically")
			log.Println("  - Inspect routes and middleware")
			log.Println("  - Capture and replay requests")
		}
		if mcpObservability {
			log.Println("Observability resources enabled:")
			log.Println("  - Server configuration (sanitized)")
			log.Println("  - Health metrics")
			log.Println("  - Recent logs")
		}
	}
}