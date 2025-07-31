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

	"github.com/osauer/hyperserve"
)

func main() {
	// APPLICATION handles flag parsing, not hyperserve
	var (
		mcpDev           = flag.Bool("mcp-dev", false, "Enable MCP developer tools")
		mcpObservability = flag.Bool("mcp-observability", false, "Enable MCP observability")
		mcpTransport     = flag.String("mcp-transport", "http", "MCP transport (http|stdio)")
		port             = flag.String("port", "8080", "Server port")
	)
	
	flag.Parse()
	
	// Build server options programmatically based on parsed flags
	opts := []hyperserve.ServerOptionFunc{
		hyperserve.WithAddr(":" + *port),
	}
	
	// Configure MCP based on flags
	if *mcpDev || *mcpObservability {
		var mcpConfigs []hyperserve.MCPTransportConfig
		
		if *mcpTransport == "stdio" {
			mcpConfigs = append(mcpConfigs, hyperserve.MCPOverStdio())
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
	srv, err := hyperserve.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}
	
	// Add some example routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello from HyperServe!")
		fmt.Fprintln(w, "")
		if srv.MCPEnabled() {
			fmt.Fprintln(w, "MCP is enabled:")
			fmt.Fprintf(w, "- Transport: %s\n", *mcpTransport)
			fmt.Fprintf(w, "- Developer Mode: %v\n", *mcpDev)
			fmt.Fprintf(w, "- Observability: %v\n", *mcpObservability)
		} else {
			fmt.Fprintln(w, "MCP is not enabled. Use --mcp-dev or --mcp-observability to enable.")
		}
	})
	
	srv.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "ok", "server": "hyperserve"}`)
	})
	
	// Print usage information
	printUsage(*mcpDev, *mcpObservability, *mcpTransport, *port)
	
	// Run the server
	srv.Run()
}

func printUsage(mcpDev, mcpObservability bool, transport, port string) {
	log.Println("Server starting on port", port)
	
	if !mcpDev && !mcpObservability {
		log.Println("MCP is disabled. To enable, use:")
		log.Println("  ./mcp-flags --mcp-dev                  # For development")
		log.Println("  ./mcp-flags --mcp-observability        # For production monitoring")
		log.Println("  ./mcp-flags --mcp-dev --mcp-transport=stdio  # For Claude Desktop")
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