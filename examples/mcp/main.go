// Package main demonstrates hyperserve's Model Context Protocol (MCP) support.
// MCP enables AI assistants to interact with your server through built-in tools and resources.
package main

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"

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
		hyperserve.WithMCPSupport(),
		hyperserve.WithMCPFileToolRoot(sandboxDir),
		hyperserve.WithMCPServerInfo("mcp-example", "1.0.0"),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Serve the MCP dashboard
	srv.HandleTemplate("/", "index.html", map[string]string{
		"MCPEndpoint": "/mcp",
		"SandboxDir":  sandboxDir,
	})

	// Add rate limiting to MCP endpoint for production use
	srv.AddMiddleware("/mcp", hyperserve.RateLimitMiddleware(srv))

	slog.Info("MCP server starting",
		"address", ":8080",
		"mcp_endpoint", "/mcp",
		"sandbox", sandboxDir,
		"dashboard", "http://localhost:8080")

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