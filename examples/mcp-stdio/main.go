// Package main demonstrates hyperserve's MCP support as a stdio server for Claude Desktop.
// This allows Claude to interact with your local system through MCP tools and resources.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/osauer/hyperserve/pkg/mcp"
	_ "github.com/osauer/hyperserve/pkg/mcp/builtin" // register builtin preset hooks
	serverpkg "github.com/osauer/hyperserve/pkg/server"
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
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Resolve home directory: %v", err)
		}
		sandboxDir = filepath.Join(homeDir, ".hyperserve-mcp", "sandbox")
	}
	if err := os.MkdirAll(sandboxDir, 0o755); err != nil {
		log.Fatalf("Failed to create sandbox directory: %v", err)
	}

	// Create sample files
	if err := createSampleFiles(sandboxDir); err != nil {
		log.Fatalf("Create sample files: %v", err)
	}

	// Create server with MCP stdio support
	opts := []serverpkg.ServerOptionFunc{
		serverpkg.WithMCPSupport("hyperserve-mcp-stdio", "1.0.0", mcp.OverStdio()),
		serverpkg.WithMCPBuiltinTools(true),
		serverpkg.WithMCPBuiltinResources(true),
		serverpkg.WithMCPFileToolRoot(sandboxDir),
	}

	if verbose {
		log.SetOutput(os.Stderr)
		log.Printf("MCP stdio server starting (sandbox: %s)", sandboxDir)
	}

	srv, err := serverpkg.NewServer(opts...)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Run the stdio server
	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func createSampleFiles(dir string) error {
	files := map[string]string{
		"hello.txt": "Hello from Hyperserve MCP stdio server!",
		"test.json": `{"message": "This is a test file", "server": "hyperserve"}`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
