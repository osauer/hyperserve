package main

import (
	"fmt"
	"log"
	"net/http"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	// Create a new HyperServe server with default options
	// This creates a server that will listen on port 8080
	srv, err := serverpkg.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Register a simple handler function for the root path "/"
	// This handler will be called whenever someone visits http://localhost:8080/
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Write a response to the client
		// fmt.Fprintf writes formatted text to the response writer
		fmt.Fprintf(w, "Hello, World from HyperServe!\n")
	})

	// Start the server
	// This will block until the server is stopped (e.g., with Ctrl+C)
	log.Println("Starting server on http://localhost:8080")
	log.Println("Press Ctrl+C to stop")

	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
