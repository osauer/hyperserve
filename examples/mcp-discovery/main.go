package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/mcp"
)

// CustomTool that opts out of discovery
type SecretTool struct{}

func (t *SecretTool) Name() string        { return "secret_operation" }
func (t *SecretTool) Description() string { return "Secret internal operation" }
func (t *SecretTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data": map[string]any{"type": "string"},
		},
	}
}
func (t *SecretTool) Execute(params map[string]any) (any, error) {
	return "Secret operation completed", nil
}

// Make it non-discoverable
func (t *SecretTool) IsDiscoverable() bool { return false }

// PublicTool that's always discoverable
type PublicTool struct{}

func (t *PublicTool) Name() string        { return "public_info" }
func (t *PublicTool) Description() string { return "Get public information" }
func (t *PublicTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *PublicTool) Execute(params map[string]any) (any, error) {
	return "Public information", nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Example 1: Count-only policy (most restrictive)
	app1, _ := hyperserve.New(
		hyperserve.WithAddr(":8081"),
		hyperserve.WithMCPSupport("discovery-demo", "1.0.0"),
		hyperserve.WithMCPDiscoveryPolicy(mcp.DiscoveryCount),
	)
	app1.RegisterMCPTool(&PublicTool{})
	app1.RegisterMCPTool(&SecretTool{})
	log.Println("Server 1 on :8081 - DiscoveryCount policy (only shows counts)")

	// Example 2: Authenticated policy
	app2, _ := hyperserve.New(
		hyperserve.WithAddr(":8082"),
		hyperserve.WithMCPSupport("discovery-demo", "1.0.0"),
		hyperserve.WithMCPDiscoveryPolicy(mcp.DiscoveryAuthenticated),
	)
	app2.RegisterMCPTool(&PublicTool{})
	app2.RegisterMCPTool(&SecretTool{})
	log.Println("Server 2 on :8082 - DiscoveryAuthenticated (requires Authorization header)")

	// Example 3: Custom filter based on IP
	app3, _ := hyperserve.New(
		hyperserve.WithAddr(":8083"),
		hyperserve.WithMCPSupport("discovery-demo", "1.0.0"),
		hyperserve.WithMCPDiscoveryFilter(func(toolName string, r *http.Request) bool {
			// Only show tools to localhost connections
			remoteAddr := r.RemoteAddr
			if strings.Contains(remoteAddr, "[::1]") || strings.HasPrefix(remoteAddr, "127.") {
				return true
			}
			// External connections only see public tools
			return strings.Contains(toolName, "public")
		}),
	)
	app3.RegisterMCPTool(&PublicTool{})
	app3.RegisterMCPTool(&SecretTool{})
	app3.RegisterMCPTool(&CustomTool{name: "admin_tool"})
	log.Println("Server 3 on :8083 - Custom filter (localhost sees all, others see public only)")

	// Start servers
	go app1.Run(ctx)
	go app2.Run(ctx)

	// Add demo endpoint to show discovery
	app3.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "MCP Discovery Policy Examples")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Test the discovery endpoints:")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "1. Count-only (port 8081):")
		fmt.Fprintln(w, "   curl http://localhost:8081/.well-known/mcp.json")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "2. Authenticated (port 8082):")
		fmt.Fprintln(w, "   # Without auth - only counts")
		fmt.Fprintln(w, "   curl http://localhost:8082/.well-known/mcp.json")
		fmt.Fprintln(w, "   # With auth - full list")
		fmt.Fprintln(w, "   curl -H 'Authorization: Bearer token' http://localhost:8082/.well-known/mcp.json")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "3. Custom filter (port 8083):")
		fmt.Fprintln(w, "   # From localhost - see all tools")
		fmt.Fprintln(w, "   curl http://localhost:8083/.well-known/mcp.json")
		fmt.Fprintln(w, "   # From external IP - only public tools")
	})

	app3.Run(ctx)
}

// CustomTool with configurable name
type CustomTool struct {
	name string
}

func (t *CustomTool) Name() string        { return t.name }
func (t *CustomTool) Description() string { return "Admin tool" }
func (t *CustomTool) Schema() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *CustomTool) Execute(params map[string]any) (any, error) {
	return "Admin operation", nil
}
