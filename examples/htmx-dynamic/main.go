package main

import (
	"log"
	"net/http"
	"time"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

type pageData struct {
	WelcomeMessage string
	PageTitle      string
	LoadTime       string
}

func main() {
	// Initialize the hyperserve server
	srv, err := serverpkg.NewServer()
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Configure server options
	srv.Options.TemplateDir = "./templates"
	srv.Options.StaticDir = "./static"

	// Middleware: Add security headers for all routes
	srv.AddMiddlewareStack("/", serverpkg.SecureWeb(srv.Options))

	// Static content route (e.g., CSS, JS)
	if err := srv.HandleStaticChecked("/static/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	// Main page route with HTMX support
	srv.HandleTemplate("/", "index.html", &pageData{
		WelcomeMessage: "Welcome to hyperserve with HTMX",
		PageTitle:      "hyperserve with HTMX",
	})

	// Dynamic content route for real-time updates
	srv.HandleFuncDynamic("/dynamic-content", "dynamic-content.html", currentTime)

	// Start the server
	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// currentTime provides the current timestamp for dynamic content
func currentTime(r *http.Request) any {
	return map[string]any{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}
}
