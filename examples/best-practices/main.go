// Package main demonstrates several HyperServe features in one process.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/auth"
	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

// AppData represents our application's template data
type AppData struct {
	Title     string
	Message   string
	Timestamp time.Time
}

// CustomTool demonstrates how to add custom MCP tools
type CustomTool struct{}

func (t *CustomTool) Name() string        { return "app_status" }
func (t *CustomTool) Description() string { return "Get application status" }
func (t *CustomTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"verbose": map[string]any{
				"type":        "boolean",
				"description": "Include detailed metrics",
			},
		},
	}
}
func (t *CustomTool) Execute(params map[string]any) (any, error) {
	verbose, _ := params["verbose"].(bool)
	status := map[string]any{
		"status":  "healthy",
		"version": "1.0.0",
		"uptime":  time.Since(startTime).String(),
	}
	if verbose {
		status["requests_handled"] = requestCount.Load()
	}
	return status, nil
}

var (
	startTime    = time.Now()
	requestCount atomic.Int64
)

func main() {
	// The executable chooses its shutdown signals. HyperServe follows the
	// resulting application context without taking ownership of the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Bind deployment environment explicitly, before application-owned capabilities.
	// HS_PORT=9090 HS_RATE_LIMIT=50 HS_LOG_LEVEL=DEBUG ./best-practices
	srv, err := serverpkg.NewServer(
		// Basic configuration
		serverpkg.WithAddr(":8080"),
		serverpkg.WithRateLimit(100, 200), // 100 req/s, burst 200
		serverpkg.WithEnvironment(),       // Deployment overrides the baseline above.

		// Application-owned capabilities and security policy
		serverpkg.WithHealthServer(),
		serverpkg.WithHealthAddr(":9080"),

		// Feature configuration
		serverpkg.WithMCPSupport("best-practices", "1.0.0"), // Enable MCP
		serverpkg.WithTemplateDir("./templates"),            // Template support
	)
	if err != nil {
		log.Fatal(err)
	}

	// Name each authentication step so the policy reads left to right.
	verifier := auth.TokenVerifierFunc(verifyToken)
	apiIdentity := auth.Bearer(verifier)
	requireIdentity := auth.Require(apiIdentity)
	srv.Use(serverpkg.HeadersMiddleware(srv.Options()))
	srv.UsePrefix("/api", requireIdentity, serverpkg.RateLimitMiddleware(srv))
	srv.UsePrefix("/mcp", requireIdentity)

	if srv.MCPEnabled() {
		if err := srv.RegisterMCPTool(&CustomTool{}); err != nil {
			log.Printf("Warning: Failed to register custom MCP tool: %v", err)
		}
	}

	// Web routes
	srv.GET("/", handleHome)
	srv.HandleFuncDynamic("/about", "about.html", func(r *http.Request) any {
		return AppData{
			Title:     "About",
			Message:   "Composition reference",
			Timestamp: time.Now(),
		}
	})

	// API routes
	srv.GET("/api/data", handleAPIData)
	srv.GET("/api/stream", handleSSEStream)

	fmt.Println("Server starting on http://localhost:8080")
	fmt.Println("Health checks on http://localhost:9080/healthz/")
	fmt.Println("MCP endpoint on http://localhost:8080/mcp")
	fmt.Println("Press Ctrl+C for graceful shutdown")

	// Run blocks until the application context is cancelled or the server exits.
	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func verifyToken(_ context.Context, token string) (auth.Principal, error) {
	if token != "secret-token-123" {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	return auth.Principal{Issuer: "best-practices", Subject: "demo-user"}, nil
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	requestCount.Add(1)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	    <title>HyperServe composition</title>
</head>
<body>
	    <h1>HyperServe composition example</h1>
	    <p>Several independent server concerns share one lifecycle.</p>
	    <h2>Features in use</h2>
	    <ul>
	        <li>Context-driven graceful shutdown</li>
	        <li>Default request logging, metrics, and recovery</li>
	        <li>Rate limiting and authentication on /api</li>
	        <li>Health checks on <a href="http://localhost:9080/healthz/">:9080/healthz/</a></li>
	        <li>MCP protected by the same identity middleware</li>
	        <li>SSE streaming that follows request cancellation</li>
	    </ul>
	    <h2>Try it</h2>
    <ul>
        <li><a href="/api/data">API endpoint with auth</a> (needs Bearer token)</li>
        <li><a href="/about">Template rendering</a></li>
        <li><code>curl -H "Authorization: Bearer secret-token-123" http://localhost:8080/api/data</code></li>
        <li><code>HS_LOG_LEVEL=DEBUG ./best-practices</code> (debug logging)</li>
    </ul>
</body>
</html>
`)
}

func handleAPIData(w http.ResponseWriter, r *http.Request) {
	// The /api prefix middleware has already established a principal.
	requestCount.Add(1)

	data := map[string]any{
		"message":   "This is protected data",
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func handleSSEStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			data := map[string]any{
				"time":     time.Now().Format(time.RFC3339),
				"requests": requestCount.Load(),
			}
			msg := serverpkg.NewSSEMessage(data)
			msg.Event = "time-update"
			fmt.Fprint(w, msg)
			flusher.Flush()

		case <-r.Context().Done():
			// Client disconnected
			return
		}
	}
}
