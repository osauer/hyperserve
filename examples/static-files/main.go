package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Filesystem roots are deliberately explicit: an embedding application's
	// working directory must never become web content by convention alone.
	srv, err := serverpkg.NewServer(serverpkg.WithStaticDir("./static"))
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Add security headers middleware for our static content
	// This adds headers like X-Content-Type-Options, X-Frame-Options, etc.
	srv.Use(serverpkg.HeadersMiddleware(srv.Options()))

	// Serve static files from the ./static directory
	// When someone visits /, it will automatically serve static/index.html
	if err := srv.HandleStatic("/"); err != nil {
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

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
