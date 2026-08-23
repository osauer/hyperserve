package main

import (
	"log"
	"net/http"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	// Create a server configured to serve static files
	// The default static directory is "static/"
	srv, err := serverpkg.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Add security headers middleware for our static content
	// This adds headers like X-Content-Type-Options, X-Frame-Options, etc.
	srv.AddMiddleware("*", serverpkg.HeadersMiddleware(srv.Options))

	// Serve static files from the ./static directory
	// When someone visits /, it will automatically serve static/index.html
	if err := srv.HandleStaticChecked("/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	// You can also add custom routes alongside static files
	srv.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok", "message": "Server is running"}`))
	})

	// Start the server
	log.Println("Starting static file server on http://localhost:8080")
	log.Println("Serving files from ./static directory")
	log.Println("Try these URLs:")
	log.Println("  http://localhost:8080/            (index.html)")
	log.Println("  http://localhost:8080/about.html")
	log.Println("  http://localhost:8080/api/status  (custom route)")
	log.Println("")
	log.Println("Press Ctrl+C to stop")

	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
