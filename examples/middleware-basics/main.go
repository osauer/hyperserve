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

	"github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := newServer()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("middleware example listening on http://localhost:8080")
	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func newServer() (*server.Server, error) {
	srv, err := server.NewServer(
		server.WithRateLimit(5, 10),
	)
	if err != nil {
		return nil, err
	}

	// NewServer already installs metrics, request logging, and panic recovery.
	// Add only the policy this application needs on top of those defaults.
	srv.Use(exampleHeader)
	srv.Use(server.HeadersMiddleware(srv.Options()))

	// Prefix middleware applies to /api and its descendants, but not /api2.
	srv.UsePrefix("/api", server.RateLimitMiddleware(srv))

	srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "Public route: security headers, no API rate limit")
	})

	srv.GET("/api/data", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"message": "API route: the response includes X-RateLimit-* headers",
			"time":    time.Now().UTC(),
			"path":    r.URL.Path,
		})
	})

	srv.GET("/api/crash", func(http.ResponseWriter, *http.Request) {
		// The default recovery middleware converts this panic to a generic 500.
		panic("demonstration panic")
	})

	srv.GET("/stats", func(w http.ResponseWriter, _ *http.Request) {
		// Default metrics are available without registering metrics again.
		writeJSON(w, map[string]uint64{"requests": srv.TotalRequests()})
	})

	return srv, nil
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
