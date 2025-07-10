package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/osauer/hyperserve"
)

// Step represents a middleware demonstration step
type Step struct {
	Number      int
	Name        string
	Description string
	RunFunc     func(*hyperserve.Server)
}

func main() {
	fmt.Println("=== HyperServe Middleware Basics ===")
	fmt.Println("This example demonstrates middleware layer by layer.")
	fmt.Println("We'll start with no middleware and add one at a time.")

	// Define our demonstration steps
	steps := []Step{
		{
			Number:      1,
			Name:        "No Middleware",
			Description: "Basic server with no middleware - raw performance",
			RunFunc:     runNoMiddleware,
		},
		{
			Number:      2,
			Name:        "Request Logging",
			Description: "Add logging to see each request",
			RunFunc:     runWithLogging,
		},
		{
			Number:      3,
			Name:        "Request Metrics",
			Description: "Add metrics to track request counts and timing",
			RunFunc:     runWithMetrics,
		},
		{
			Number:      4,
			Name:        "Rate Limiting",
			Description: "Add rate limiting to prevent abuse",
			RunFunc:     runWithRateLimiting,
		},
		{
			Number:      5,
			Name:        "Full Stack",
			Description: "All middleware combined with route-specific rules",
			RunFunc:     runFullStack,
		},
	}

	// Run each step
	for _, step := range steps {
		fmt.Printf("\n--- Step %d: %s ---\n", step.Number, step.Name)
		fmt.Printf("Description: %s\n", step.Description)
		fmt.Println("Press Enter to start this step...")
		fmt.Scanln()

		// Create a new server for each step
		server, err := hyperserve.NewServer()
		if err != nil {
			log.Fatalf("Failed to create server: %v", err)
		}

		// Run the step
		step.RunFunc(server)

		// Start server in background
		go func() {
			if err := server.Run(); err != nil && err != http.ErrServerClosed {
				log.Printf("Server error: %v", err)
			}
		}()

		// Wait for user to continue
		fmt.Println("\nServer running on http://localhost:8080")
		fmt.Println("Test with: curl http://localhost:8080/api/data")
		fmt.Println("Press Enter to stop and continue to next step...")
		fmt.Scanln()

		// Stop the server
		server.Stop()
		time.Sleep(100 * time.Millisecond) // Brief pause between steps
	}

	fmt.Println("\n=== Middleware demonstration complete! ===")
	fmt.Println("You've seen how middleware layers build on each other.")
	fmt.Println("Check the code to understand how each middleware works.")
}

// Step 1: No middleware
func runNoMiddleware(server *hyperserve.Server) {
	// Simple API endpoint
	server.HandleFunc("/api/data", apiHandler)
	
	fmt.Println("Running with NO middleware - baseline performance")
}

// Step 2: Add logging
func runWithLogging(server *hyperserve.Server) {
	// Add request logging middleware globally
	server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
		return hyperserve.RequestLoggerMiddleware(next)
	})
	
	server.HandleFunc("/api/data", apiHandler)
	
	fmt.Println("Added REQUEST LOGGING - watch the console for log entries")
}

// Step 3: Add metrics
func runWithMetrics(server *hyperserve.Server) {
	// Add both logging and metrics
	server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
		return hyperserve.RequestLoggerMiddleware(next)
	})
	server.AddMiddleware("*", hyperserve.MetricsMiddleware(server))
	
	server.HandleFunc("/api/data", apiHandler)
	
	// Add metrics endpoint
	server.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Since metrics are internal, we'll create a simple response
		// In a real app, you'd expose these through a proper API
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Metrics are being collected",
			"hint": "Check server logs for request counts and timing",
		})
	})
	
	fmt.Println("Added METRICS - check http://localhost:8080/metrics")
}

// Step 4: Add rate limiting
func runWithRateLimiting(server *hyperserve.Server) {
	// Configure rate limiting
	server.Options.RateLimit = 5 // 5 requests per second
	server.Options.Burst = 10
	
	// Add middleware stack
	server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
		return hyperserve.RequestLoggerMiddleware(next)
	})
	server.AddMiddleware("*", hyperserve.MetricsMiddleware(server))
	server.AddMiddleware("*", hyperserve.RateLimitMiddleware(server))
	
	server.HandleFunc("/api/data", apiHandler)
	server.HandleFunc("/metrics", metricsHandler(server))
	
	fmt.Println("Added RATE LIMITING - try rapid requests to see 429 errors")
	fmt.Println("Rate limit: 5 req/sec, burst: 10")
}

// Step 5: Full stack with route-specific middleware
func runFullStack(server *hyperserve.Server) {
	// Global middleware - applies to all routes
	server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
		return hyperserve.RequestLoggerMiddleware(next)
	})
	server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
		return hyperserve.RecoveryMiddleware(next)
	})
	
	// Route-specific middleware
	// Only apply rate limiting to API routes
	server.AddMiddleware("/api", hyperserve.RateLimitMiddleware(server))
	server.AddMiddleware("/api", hyperserve.MetricsMiddleware(server))
	
	// Public routes (no rate limiting)
	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<h1>Middleware Demo</h1>
			<p>Public route - no rate limiting</p>
			<ul>
				<li><a href="/api/data">API Data (rate limited)</a></li>
				<li><a href="/api/crash">Crash Test (recovery middleware)</a></li>
				<li><a href="/metrics">Metrics</a></li>
			</ul>
		`)
	})
	
	// API routes (rate limited)
	server.HandleFunc("/api/data", apiHandler)
	server.HandleFunc("/api/crash", func(w http.ResponseWriter, r *http.Request) {
		panic("Intentional panic to test recovery middleware!")
	})
	
	// Metrics route (no rate limiting)
	server.HandleFunc("/metrics", metricsHandler(server))
	
	fmt.Println("FULL STACK with route-specific rules:")
	fmt.Println("- Global: Logging, Recovery")
	fmt.Println("- /api/* only: Rate limiting, Metrics")
	fmt.Println("- Try the crash test to see recovery middleware!")
}

// Shared handlers
func apiHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"message":   "Hello from the API",
		"timestamp": time.Now().Format(time.RFC3339),
		"method":    r.Method,
		"path":      r.URL.Path,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func metricsHandler(server *hyperserve.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Since metrics are internal, we'll create a simple response
		// In a real app, you'd expose these through a proper API
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Metrics are being collected",
			"hint": "Check server logs for request counts and timing",
		})
	}
}