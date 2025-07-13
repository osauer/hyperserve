// Example: Simple app with MCP flag support
//
// This shows how applications should handle MCP flags themselves
// rather than expecting hyperserve to parse them.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/osauer/hyperserve"
)

func main() {
	// Define your app's flags
	var (
		port   = flag.String("port", "8080", "Server port")
		mcpDev = flag.Bool("mcp-dev", false, "Enable MCP developer mode")
	)
	
	flag.Parse()
	
	// Build server options
	opts := []hyperserve.ServerOptionFunc{
		hyperserve.WithAddr(":" + *port),
		hyperserve.WithMCPSupport("MyApp", "1.0.0"),
	}
	
	// Add MCP dev mode if requested
	if *mcpDev {
		opts[1] = hyperserve.WithMCPSupport("MyApp", "1.0.0", hyperserve.MCPDev())
		log.Println("⚠️  MCP Developer Mode enabled")
	}
	
	// Create server
	srv, err := hyperserve.NewServer(opts...)
	if err != nil {
		log.Fatal(err)
	}
	
	// Add routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello! MCP dev mode:", *mcpDev)
	})
	
	log.Printf("Server starting on port %s", *port)
	if *mcpDev {
		log.Println("MCP developer tools available at /mcp")
	}
	
	srv.Run()
}