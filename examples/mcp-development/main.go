// Example: MCP Developer Mode for AI-assisted development
//
// This example shows how to enable developer tools that allow Claude Code
// or other AI assistants to help build and debug your application.
//
// WARNING: NEVER enable developer mode in production!
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/osauer/hyperserve"
)

func main() {
	// Create server with developer tools
	srv, err := hyperserve.NewServer(
		hyperserve.WithMCPSupport("MyDevApp", "1.0.0",
			hyperserve.MCPDev(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Add some example routes
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Welcome to the development server!")
	})

	srv.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`)
	})

	srv.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"products": [{"id": 1, "name": "Widget", "price": 9.99}]}`)
	})

	// Simulate an error endpoint for debugging
	srv.HandleFunc("/api/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Simulated error for debugging", http.StatusInternalServerError)
	})

	log.Println("Starting development server with MCP developer tools enabled")
	log.Println("Available MCP tools:")
	log.Println("  - server_control: Restart server, change log levels")
	log.Println("  - route_inspector: List all routes and middleware")
	log.Println("  - request_debugger: Capture and replay requests")
	log.Println("")
	log.Println("Example usage with Claude:")
	log.Println("  'Show me all registered routes'")
	log.Println("  'Set log level to DEBUG'")
	log.Println("  'List recent requests to /api/error'")

	srv.Run()
}