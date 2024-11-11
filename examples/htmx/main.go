package main

import (
	"log"
	"net/http"
	"time"

	"github.com/osauer/hyperserve"
)

// Main handler for serving the home page with data
func main() {
	server, err := hyperserve.NewServer(
		hyperserve.WithTemplateDir("examples/htmx/templates"))
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}

	// Define the index route with dynamic data
	server.HandleTemplate("/", "index.html", struct {
		WelcomeMessage string
		PageTitle      string
	}{
		WelcomeMessage: "Welcome to Hyperserve with HTMX! 🚀",
		PageTitle:      "Example for Dynamic Index Page",
	})

	// Add HTMX-enabled dynamic content route
	server.HandleFuncDynamic("/dynamic-content", "dynamic-content.html", currentTime)

	server.Run()
}

func currentTime(r *http.Request) interface{} {
	// not using the request, but it's available if needed
	t := map[string]interface{}{
		"timestamp": time.Now().Format("2006-01-02 15:04:05")}
	return t
}
