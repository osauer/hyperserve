package main

import (
	"log"
	"net/http"
	"time"

	"github.com/osauer/hyperserve"
)

type pageData struct {
	WelcomeMessage string
	PageTitle      string
	LoadTime       string
}

func main() {
	// Initialize the hyperserve server
	server, err := hyperserve.NewServer()
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Configure server options
	server.Options.TemplateDir = "examples/htmx/templates"
	server.Options.StaticDir = "examples/htmx/static"

	// Middleware: Add security headers for all routes
	server.AddMiddlewareStack("/", hyperserve.SecureWeb(*server.Options))

	// Static content route (e.g., CSS, JS)
	server.HandleStatic("/static/")

	// Main page route with HTMX support
	server.HandleTemplate("/", "index.html", &pageData{
		WelcomeMessage: "Welcome to hyperserve with HTMX",
		PageTitle:      "hyperserve with HTMX",
	})

	// Dynamic content route for real-time updates
	server.HandleFuncDynamic("/dynamic-content", "dynamic-content.html", currentTime)

	// Start the server
	if err := server.Run(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// currentTime provides the current timestamp for dynamic content
func currentTime(r *http.Request) interface{} {
	return map[string]interface{}{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}
}
