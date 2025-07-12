// Package main demonstrates best practices for using hyperserve.
// This example shows how to properly leverage hyperserve's built-in features
// without reimplementing functionality that already exists.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/osauer/hyperserve"
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
func (t *CustomTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"verbose": map[string]interface{}{
				"type":        "boolean",
				"description": "Include detailed metrics",
			},
		},
	}
}
func (t *CustomTool) Execute(params map[string]interface{}) (interface{}, error) {
	verbose, _ := params["verbose"].(bool)
	status := map[string]interface{}{
		"status":  "healthy",
		"version": "1.0.0",
		"uptime":  time.Since(startTime).String(),
	}
	if verbose {
		status["requests_handled"] = requestCount
	}
	return status, nil
}

var (
	startTime    = time.Now()
	requestCount int64
)

func main() {
	// BEST PRACTICE: Use hyperserve's configuration system
	// These can be overridden with environment variables:
	// HS_PORT=9090 HS_RATE_LIMIT=50 HS_LOG_LEVEL=debug ./best-practices
	srv, err := hyperserve.NewServer(
		// Basic configuration
		hyperserve.WithAddr(":8080"),
		hyperserve.WithHealthServer(), // Health checks on :8081

		// Security configuration
		hyperserve.WithRateLimit(100, 200),                            // 100 req/s, burst 200
		hyperserve.WithAuthTokenValidator(validateToken),              // Custom auth
		// Graceful shutdown timeout is configurable via timeouts

		// Feature configuration
		hyperserve.WithMCPSupport("best-practices", "1.0.0"),         // Enable MCP
		hyperserve.WithMCPFileToolRoot("./safe-directory"),            // Sandboxed file access
		hyperserve.WithTemplateDir("./templates"),                     // Template support
	)
	if err != nil {
		log.Fatal(err)
	}

	// BEST PRACTICE: Use middleware stacks for common patterns
	srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))        // Auth + rate limiting
	srv.AddMiddlewareStack("/", hyperserve.SecureWeb(srv.Options))   // Security headers

	// BEST PRACTICE: Register custom MCP tools properly
	if srv.MCPEnabled() {
		if err := srv.RegisterMCPTool(&CustomTool{}); err != nil {
			log.Printf("Warning: Failed to register custom MCP tool: %v", err)
		}
	}

	// Web routes
	srv.HandleFunc("/", handleHome)
	srv.HandleFuncDynamic("/about", "about.html", func(r *http.Request) interface{} {
		return AppData{
			Title:     "About",
			Message:   "Best practices example",
			Timestamp: time.Now(),
		}
	})

	// API routes
	srv.HandleFunc("/api/data", handleAPIData)
	srv.HandleFunc("/api/stream", handleSSEStream)

	// Static files with proper caching headers
	srv.AddMiddleware("/static/", hyperserve.HeadersMiddleware(srv.Options))
	srv.HandleStatic("/static/")

	// BEST PRACTICE: Let hyperserve handle graceful shutdown
	// No need for custom signal handling!
	fmt.Println("Server starting on http://localhost:8080")
	fmt.Println("Health checks on http://localhost:8081/healthz")
	fmt.Println("MCP endpoint on http://localhost:8080/mcp")
	fmt.Println("Press Ctrl+C for graceful shutdown")

	// Run blocks and handles shutdown automatically
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

// validateToken demonstrates custom auth validation
func validateToken(token string) (bool, error) {
	// In production, validate against your auth system
	return token == "secret-token-123", nil
}

// handleHome demonstrates a simple handler
func handleHome(w http.ResponseWriter, r *http.Request) {
	requestCount++
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>Hyperserve Best Practices</title>
</head>
<body>
    <h1>Hyperserve Best Practices Example</h1>
    <p>This example demonstrates proper usage of hyperserve's built-in features.</p>
    <h2>Features in Use:</h2>
    <ul>
        <li>✅ Automatic graceful shutdown (try Ctrl+C)</li>
        <li>✅ Built-in request logging (check console)</li>
        <li>✅ Rate limiting (100 req/s)</li>
        <li>✅ Health checks (<a href="http://localhost:8081/healthz">/healthz</a>)</li>
        <li>✅ MCP support (<a href="/api/mcp-test">/api/mcp-test</a>)</li>
        <li>✅ SSE streaming (<a href="/api/stream">/api/stream</a>)</li>
        <li>✅ Security headers (check DevTools)</li>
    </ul>
    <h2>Try These:</h2>
    <ul>
        <li><a href="/api/data">API endpoint with auth</a> (needs Bearer token)</li>
        <li><a href="/about">Template rendering</a></li>
        <li><code>curl -H "Authorization: Bearer secret-token-123" http://localhost:8080/api/data</code></li>
        <li><code>HS_LOG_LEVEL=debug ./best-practices</code> (debug logging)</li>
    </ul>
</body>
</html>
`)
}

// handleAPIData demonstrates a protected API endpoint
func handleAPIData(w http.ResponseWriter, r *http.Request) {
	// Auth is already validated by SecureAPI middleware stack
	requestCount++
	
	data := map[string]interface{}{
		"message":    "This is protected data",
		"timestamp":  time.Now(),
		"request_id": r.Header.Get("X-Request-ID"), // Added by middleware
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleSSEStream demonstrates proper SSE usage
func handleSSEStream(w http.ResponseWriter, r *http.Request) {
	// BEST PRACTICE: Use hyperserve's SSE helpers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send events using hyperserve's SSE format
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// BEST PRACTICE: Use NewSSEMessage helper
			data := map[string]interface{}{
				"time":     time.Now().Format(time.RFC3339),
				"requests": requestCount,
			}
			msg := hyperserve.NewSSEMessage(data)
			msg.Event = "time-update"
			fmt.Fprint(w, msg)
			flusher.Flush()

		case <-r.Context().Done():
			// Client disconnected
			return
		}
	}
}