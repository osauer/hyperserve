// Complete Example - Demonstrating HyperServe Features
//
// This example shows correct usage of hyperserve's features including
// zero-config defaults and opt-in middleware stacks.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/auth"
	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

// Mock user store for authentication demo
var validTokens = map[string]string{
	"demo-token-123": "alice",
	"demo-token-456": "bob",
}

// Mock data for SSE streaming
type SystemStatus struct {
	CPU      int       `json:"cpu"`
	Memory   int       `json:"memory"`
	Requests int       `json:"requests"`
	Time     time.Time `json:"time"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Create server with configuration
	srv, err := serverpkg.NewServer(
		// Basic configuration
		serverpkg.WithAddr(":8080"),
		serverpkg.WithHealthServer(), // Health checks on :8081

		// Advanced features
		serverpkg.WithMCPSupport("complete-example", "1.0.0"),
		serverpkg.WithMCPEndpoint("/mcp"),

		// Rate limiting configuration
		serverpkg.WithRateLimit(100, 200),
		serverpkg.WithTemplateDir("./templates"),
		serverpkg.WithStaticDir("./static"),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Apply middleware stacks
	// Metrics, request logging, and recovery are already applied by NewServer.

	// SecureWeb adds security headers for web routes
	srv.UsePrefix("/", serverpkg.SecureWeb(srv.Options()))

	verifier := auth.TokenVerifierFunc(verifyToken)
	apiIdentity := auth.Bearer(verifier)
	requireIdentity := auth.Require(apiIdentity)
	srv.UsePrefix("/api", requireIdentity, serverpkg.RateLimitMiddleware(srv))
	srv.UsePrefix("/mcp", requireIdentity)

	// ===== ROUTE HANDLERS =====

	// 1. Home page with template
	srv.HandleFuncDynamic("/", "index.html", func(r *http.Request) any {
		return map[string]any{
			"title":    "HyperServe Complete Example",
			"features": getFeatureList(),
			"time":     time.Now().Format(time.RFC3339),
		}
	})

	// 2. Static file serving
	if err := srv.HandleStatic("/static/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	// 3. Public API endpoint (no auth required, but has security headers)
	srv.GET("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "running",
			"version": "1.0.0",
			"time":    time.Now(),
		})
	})

	// 4. Protected API endpoint. Authentication establishes identity;
	// application code still owns authorization and domain lookup.
	srv.GET("/api/user", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFromRequest(r)
		if !ok {
			http.Error(w, "missing authenticated principal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"username": principal.Subject,
			"role":     "user",
			"lastSeen": time.Now(),
		})
	})

	// 5. Server-Sent Events (SSE) for real-time updates
	srv.HandleFunc("/api/stream", sseHandler)

	// 6. Demonstrate error handling (recovery middleware handles panics)
	srv.HandleFunc("/api/error", func(w http.ResponseWriter, r *http.Request) {
		if rand.Float32() < 0.5 {
			panic("Simulated panic - recovery middleware will handle this!")
		}
		http.Error(w, "Simulated error", http.StatusInternalServerError)
	})

	// 7. File upload demonstration
	srv.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse multipart form
		err := r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Failed to get file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"message":  "File received",
			"filename": header.Filename,
			"size":     header.Size,
			"type":     header.Header.Get("Content-Type"),
		})
	})

	// 8. Metrics endpoint (demonstrates built-in metrics collection)
	srv.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Note: Real metrics are internal. This is a demo endpoint.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"message": "Metrics are collected automatically by the server defaults",
			"info":    "Request count and timing are tracked for all requests",
		})
	})

	// ===== START SERVER =====

	printStartupBanner()

	// The application owns signals; Run provides:
	// - Connection draining
	// - Context cancellation
	// - Resource cleanup
	// - Health checks on separate port
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// verifyToken is named business code; auth.Bearer owns only HTTP extraction.
func verifyToken(_ context.Context, token string) (auth.Principal, error) {
	user, valid := validTokens[token]
	if !valid {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	return auth.Principal{Issuer: "complete-example", Subject: user}, nil
}

// SSE handler for real-time updates
func sseHandler(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	// Create ticker for periodic updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send initial message
	msg := serverpkg.NewSSEMessage(map[string]any{
		"type":    "connected",
		"message": "SSE stream connected",
		"time":    time.Now(),
	})
	fmt.Fprintf(w, "%s", msg)
	w.(http.Flusher).Flush()

	// Stream updates until client disconnects
	for {
		select {
		case <-r.Context().Done():
			log.Printf("SSE client disconnected: %v", r.Context().Err())
			return

		case <-ticker.C:
			// Send system status
			status := SystemStatus{
				CPU:      rand.Intn(100),
				Memory:   rand.Intn(100),
				Requests: rand.Intn(1000),
				Time:     time.Now(),
			}

			msg := serverpkg.NewSSEMessage(status)
			if _, err := fmt.Fprintf(w, "%s", msg); err != nil {
				log.Printf("SSE write error: %v", err)
				return
			}
			w.(http.Flusher).Flush()
		}
	}
}

// Get feature list for template
func getFeatureList() []map[string]string {
	return []map[string]string{
		{"name": "Graceful Shutdown", "status": "context-driven", "endpoint": "srv.Run(ctx)"},
		{"name": "Health Checks", "status": "configured", "endpoint": "http://localhost:8081/healthz"},
		{"name": "Request Logging", "status": "automatic", "endpoint": "Server default"},
		{"name": "Panic Recovery", "status": "automatic", "endpoint": "Server default"},
		{"name": "Metrics Collection", "status": "automatic", "endpoint": "Server default"},
		{"name": "Security Headers", "status": "configured", "endpoint": "All routes via SecureWeb"},
		{"name": "Authentication", "status": "configured", "endpoint": "/api/* via auth.Require"},
		{"name": "Rate Limiting", "status": "configured", "endpoint": "/api/* middleware"},
		{"name": "Server-Sent Events", "status": "active", "endpoint": "/api/stream"},
		{"name": "Static Files", "status": "active", "endpoint": "/static/*"},
		{"name": "Templates", "status": "active", "endpoint": "/ (home page)"},
		{"name": "MCP Support", "status": "active", "endpoint": "/mcp"},
		{"name": "Error Recovery", "status": "automatic", "endpoint": "/api/error (50% panic rate)"},
	}
}

// Print helpful startup banner
func printStartupBanner() {
	fmt.Println("\nHyperServe Complete Example")
	fmt.Println("===========================")
	fmt.Println("\nEndpoints:")
	fmt.Println("  http://localhost:8080/              - Home page (template)")
	fmt.Println("  http://localhost:8080/static/       - Static files")
	fmt.Println("  http://localhost:8080/status        - Public API")
	fmt.Println("  http://localhost:8080/api/user      - Protected API (needs Bearer token)")
	fmt.Println("  http://localhost:8080/api/stream    - SSE real-time updates")
	fmt.Println("  http://localhost:8080/api/error     - Error handling demo")
	fmt.Println("  http://localhost:8080/api/upload    - File upload")
	fmt.Println("  http://localhost:8080/api/metrics   - Metrics info")
	fmt.Println("  http://localhost:8080/mcp           - MCP endpoint")
	fmt.Println("  http://localhost:8081/healthz       - Health check")
	fmt.Println("\nAuthentication:")
	fmt.Println("  Use these Bearer tokens for /api/user:")
	fmt.Println("  - demo-token-123 (user: alice)")
	fmt.Println("  - demo-token-456 (user: bob)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/status")
	fmt.Println("  curl -H 'Authorization: Bearer demo-token-123' http://localhost:8080/api/user")
	fmt.Println("  curl -N http://localhost:8080/api/stream  # SSE stream")
	fmt.Println("\nPress Ctrl+C for graceful shutdown")
}
