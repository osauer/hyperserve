// Package main demonstrates hyperserve's Model Context Protocol (MCP) support.
// MCP enables AI assistants to interact with your server through built-in tools and resources.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/osauer/hyperserve"
)

func main() {
	// Create sandbox directory for file operations
	sandboxDir := filepath.Join(".", "sandbox")
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		log.Fatalf("Failed to create sandbox directory: %v", err)
	}
	
	// Initialize sample files
	if err := createSampleFiles(sandboxDir); err != nil {
		log.Printf("Warning: Failed to create some sample files: %v", err)
	}

	// Create server with MCP support
	// Note: We only set non-default options
	srv, err := hyperserve.NewServer(
		hyperserve.WithTemplateDir("./templates"),
		hyperserve.WithMCPSupport("mcp-example", "1.0.0"),
		hyperserve.WithMCPFileToolRoot(sandboxDir),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Serve the MCP dashboard
	srv.HandleTemplate("/", "index.html", map[string]string{
		"MCPEndpoint": "/mcp",
		"SandboxDir":  sandboxDir,
	})

	// Register custom MCP tool
	if srv.MCPEnabled() {
		if err := srv.RegisterMCPTool(&TimestampTool{}); err != nil {
			log.Printf("Failed to register timestamp tool: %v", err)
		}
		
		// Register custom MCP resource
		if err := srv.RegisterMCPResource(&ServerStatusResource{}); err != nil {
			log.Printf("Failed to register status resource: %v", err)
		}
	}

	// Add rate limiting to MCP endpoint for production use
	srv.AddMiddleware("/mcp", hyperserve.RateLimitMiddleware(srv))

	log.Printf("MCP server starting on http://localhost:8080 (sandbox: %s)", sandboxDir)

	// Run handles graceful shutdown automatically
	if err := srv.Run(); err != nil {
		log.Fatal("Server failed:", err)
	}
}

// createSampleFiles creates demonstration files in the sandbox
func createSampleFiles(dir string) error {
	files := map[string]string{
		"welcome.txt": `Welcome to Hyperserve MCP!

This file demonstrates the file reading capabilities.
Try using the 'read_file' tool to read this content.`,

		"data.json": `{
  "server": "hyperserve",
  "protocol": "MCP",
  "version": "2024-11-05",
  "features": ["tools", "resources", "sandboxed-files"]
}`,

		"numbers.csv": `value,squared
1,1
2,4
3,9
4,16
5,25`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	// Create subdirectory with nested file
	subdir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		return err
	}
	
	return os.WriteFile(
		filepath.Join(subdir, "info.txt"),
		[]byte("This file is in a subdirectory."),
		0644,
	)
}

// TimestampTool is a custom MCP tool that generates timestamps
type TimestampTool struct{}

func (t *TimestampTool) Name() string {
	return "timestamp"
}

func (t *TimestampTool) Description() string {
	return "Generate timestamps in various formats"
}

func (t *TimestampTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Timestamp format: unix, iso8601, or rfc3339",
				"enum":        []string{"unix", "iso8601", "rfc3339"},
				"default":     "iso8601",
			},
		},
	}
}

func (t *TimestampTool) Execute(params map[string]interface{}) (interface{}, error) {
	format, _ := params["format"].(string)
	if format == "" {
		format = "iso8601"
	}

	now := time.Now()
	var result string

	switch format {
	case "unix":
		result = fmt.Sprintf("%d", now.Unix())
	case "rfc3339":
		result = now.Format(time.RFC3339)
	case "iso8601":
		fallthrough
	default:
		result = now.Format("2006-01-02T15:04:05Z07:00")
	}

	return map[string]interface{}{
		"timestamp": result,
		"format":    format,
	}, nil
}

// ServerStatusResource is a custom MCP resource that provides server status
type ServerStatusResource struct{}

func (s *ServerStatusResource) URI() string {
	return "custom://server/status"
}

func (s *ServerStatusResource) Name() string {
	return "Server Status"
}

func (s *ServerStatusResource) Description() string {
	return "Current server status and uptime information"
}

func (s *ServerStatusResource) MimeType() string {
	return "application/json"
}

func (s *ServerStatusResource) Read() (interface{}, error) {
	status := map[string]interface{}{
		"status":    "running",
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    "N/A", // In a real implementation, track server start time
		"version":   "1.0.0",
	}

	return status, nil
}

func (s *ServerStatusResource) List() ([]string, error) {
	// Single resource, return self URI
	return []string{s.URI()}, nil
}