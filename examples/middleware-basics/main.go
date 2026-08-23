package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	srv, err := serverpkg.NewServer(
		serverpkg.WithRateLimit(5, 10),
		serverpkg.WithHardenedMode(),
	)
	if err != nil {
		log.Fatal(err)
	}

	// NewServer already installs metrics, request logging, and panic recovery.
	// Add only the policy this application needs on top of those defaults.
	srv.AddMiddleware(serverpkg.GlobalMiddlewareRoute, exampleHeader)
	srv.AddMiddlewareStack(serverpkg.GlobalMiddlewareRoute, serverpkg.SecureWeb(srv.Options))

	// Prefix middleware applies to /api and its descendants, but not /api2.
	srv.AddMiddleware("/api", serverpkg.RateLimitMiddleware(srv))

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

	log.Println("middleware example listening on http://localhost:8080")
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

// exampleHeader has the ordinary net/http wrapper shape used by HyperServe.
func exampleHeader(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Example-Middleware", "active")
		next.ServeHTTP(w, r)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
