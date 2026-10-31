package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/osauer/hyperserve/v2"
)

type pageData struct {
	WelcomeMessage string
	PageTitle      string
	LoadTime       string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Initialize the hyperserve server
	app, err := hyperserve.New(
		hyperserve.WithTemplateDir("./templates"),
		hyperserve.WithStaticDir("./static"),
	)
	if err != nil {
		log.Fatalf("Error initializing server: %v", err)
	}

	// Add security headers to every route.
	app.Use(hyperserve.HeadersMiddleware(app.Options()))

	// Static content route (e.g., CSS, JS)
	if err := app.HandleStatic("/static/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	// Main page route with HTMX support
	app.HandleTemplate("/", "index.html", &pageData{
		WelcomeMessage: "Welcome to hyperserve with HTMX",
		PageTitle:      "hyperserve with HTMX",
	})

	// Dynamic content route for real-time updates
	app.HandleFuncDynamic("/dynamic-content", "dynamic-content.html", currentTime)

	// Start the server
	if err := app.Run(ctx); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// currentTime provides the current timestamp for dynamic content
func currentTime(r *http.Request) any {
	return map[string]any{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}
}
