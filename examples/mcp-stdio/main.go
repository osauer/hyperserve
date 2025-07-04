// Package main demonstrates hyperserve's MCP support as a stdio server for Claude Desktop.
// This allows Claude to interact with your local system through MCP tools and resources.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/osauer/hyperserve"
)

func main() {
	var sandboxDir string
	var verbose bool

	// Parse command line flags
	flag.StringVar(&sandboxDir, "sandbox", "", "Directory for sandboxed file operations")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging to stderr")
	flag.Parse()

	// Set up sandbox directory
	if sandboxDir == "" {
		homeDir, _ := os.UserHomeDir()
		sandboxDir = filepath.Join(homeDir, ".hyperserve-mcp", "sandbox")
	}
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		log.Fatalf("Failed to create sandbox directory: %v", err)
	}

	// Create sample files
	createSampleFiles(sandboxDir)

	// Create server with MCP stdio support
	opts := []hyperserve.ServerOptionFunc{
		hyperserve.WithMCPSupport(hyperserve.MCPOverStdio()),
		hyperserve.WithMCPFileToolRoot(sandboxDir),
		hyperserve.WithMCPServerInfo("hyperserve-mcp-stdio", "1.0.0"),
	}

	if verbose {
		log.SetOutput(os.Stderr)
		log.Printf("MCP stdio server starting (sandbox: %s)", sandboxDir)
	}

	srv, err := hyperserve.NewServer(opts...)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Run the stdio server
	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}


func createSampleFiles(dir string) {
	files := map[string]string{
		"hello.txt": "Hello from Hyperserve MCP stdio server!",
		"test.json": `{"message": "This is a test file", "server": "hyperserve"}`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte(content), 0644)
	}
}