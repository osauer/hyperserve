package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/ratelimit"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app, err := newApp()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("middleware example listening on http://localhost:8080")
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func newApp() (*hyperserve.Server, error) {
	return newAppWithAPIGate(ratelimit.Config{
		RequestsPerSecond: 5,
		Burst:             10,
	})
}

func newAppWithAPIGate(config ratelimit.Config) (*hyperserve.Server, error) {
	app, err := hyperserve.New()
	if err != nil {
		return nil, err
	}
	apiGate, err := ratelimit.New(config)
	if err != nil {
		return nil, err
	}

	// New already installs metrics, request logging, and panic recovery.
	// Add only the policy this application needs on top of those defaults.
	app.Use(exampleHeader)
	app.Use(hyperserve.HeadersMiddleware(app.Options()))

	// Prefix middleware applies to /api and its descendants, but not /api2.
	app.UsePrefix("/api", apiGate)

	app.GET("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "Public route: security headers, no API rate limit")
	})

	app.GET("/api/data", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"message": "API route: protected by the /api rate-limit gate",
			"time":    time.Now().UTC(),
			"path":    r.URL.Path,
		})
	})

	app.GET("/api/crash", func(http.ResponseWriter, *http.Request) {
		// The default recovery middleware converts this panic to a generic 500.
		panic("demonstration panic")
	})

	app.GET("/stats", func(w http.ResponseWriter, _ *http.Request) {
		// Default metrics are available without registering metrics again.
		writeJSON(w, map[string]uint64{"requests": app.TotalRequests()})
	})

	return app, nil
}

// exampleHeader has the ordinary net/http wrapper shape used by HyperServe.
func exampleHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Example-Middleware", "active")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
