package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/osauer/hyperserve"
)

func main() {
	// Configure logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Get current directory for file tool root
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Create a sandbox directory for file operations
	sandboxDir := filepath.Join(currentDir, "sandbox")
	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		log.Fatalf("Failed to create sandbox directory: %v", err)
	}

	// Create some sample files for demonstration
	createSampleFiles(sandboxDir)

	// Create server with MCP support
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithHealthServer(),
		hyperserve.WithLoglevel(slog.LevelInfo),
		
		// Enable MCP with full configuration
		hyperserve.WithMCPSupport(),
		hyperserve.WithMCPEndpoint("/mcp"),
		hyperserve.WithMCPServerInfo("hyperserve-mcp-example", "1.0.0"),
		hyperserve.WithMCPFileToolRoot(sandboxDir),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Add a simple welcome handler
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := `
<!DOCTYPE html>
<html>
<head>
    <title>Hyperserve MCP Example</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; line-height: 1.6; }
        .container { max-width: 800px; margin: 0 auto; }
        .endpoint { background: #f4f4f4; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .code { background: #e8e8e8; padding: 10px; border-radius: 3px; font-family: monospace; }
        .success { color: #28a745; }
        .info { color: #17a2b8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 Hyperserve MCP Example</h1>
        <p class="success">Server is running with <strong>MCP (Model Context Protocol)</strong> support!</p>
        
        <h2>Available Endpoints</h2>
        
        <div class="endpoint">
            <h3>🔧 MCP Endpoint</h3>
            <p><strong>URL:</strong> <code>POST /mcp</code></p>
            <p><strong>Description:</strong> Main MCP protocol endpoint for JSON-RPC communication</p>
            <p><strong>Content-Type:</strong> <code>application/json</code></p>
        </div>
        
        <div class="endpoint">
            <h3>❤️ Health Check</h3>
            <p><strong>URLs:</strong></p>
            <ul>
                <li><code>GET :9080/healthz/</code> - General health</li>
                <li><code>GET :9080/readyz/</code> - Readiness check</li>
                <li><code>GET :9080/livez/</code> - Liveness check</li>
            </ul>
        </div>
        
        <h2>MCP Capabilities</h2>
        
        <h3>🛠️ Available Tools</h3>
        <ul>
            <li><strong>calculator</strong> - Perform basic mathematical operations (add, subtract, multiply, divide)</li>
            <li><strong>read_file</strong> - Read file contents from the sandbox directory</li>
            <li><strong>list_directory</strong> - List directory contents</li>
            <li><strong>http_request</strong> - Make HTTP requests to external services</li>
        </ul>
        
        <h3>📊 Available Resources</h3>
        <ul>
            <li><strong>config://server/options</strong> - Server configuration</li>
            <li><strong>metrics://server/stats</strong> - Performance metrics</li>
            <li><strong>system://runtime/info</strong> - System information</li>
            <li><strong>logs://server/recent</strong> - Recent log entries</li>
        </ul>
        
        <h2>Example MCP Requests</h2>
        
        <h3>Initialize Connection</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {
            "name": "example-client",
            "version": "1.0.0"
        }
    },
    "id": 1
}
        </div>
        
        <h3>List Available Tools</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 2
}
        </div>
        
        <h3>Use Calculator Tool</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
        "name": "calculator",
        "arguments": {
            "operation": "add",
            "a": 15,
            "b": 27
        }
    },
    "id": 3
}
        </div>
        
        <h3>Read a File</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
        "name": "read_file",
        "arguments": {
            "path": "welcome.txt"
        }
    },
    "id": 4
}
        </div>
        
        <h3>List Available Resources</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "resources/list",
    "id": 5
}
        </div>
        
        <h3>Read System Information</h3>
        <div class="code">
POST /mcp
Content-Type: application/json

{
    "jsonrpc": "2.0",
    "method": "resources/read",
    "params": {
        "uri": "system://runtime/info"
    },
    "id": 6
}
        </div>
        
        <div class="info">
            <p><strong>Note:</strong> This server restricts file operations to the <code>./sandbox/</code> directory for security.</p>
            <p><strong>Sandbox Location:</strong> <code>` + sandboxDir + `</code></p>
        </div>
    </div>
</body>
</html>`
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, html)
	})

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Starting MCP example server", "addr", ":8080", "mcp_endpoint", "/mcp")
		serverErr <- srv.Run()
	}()

	// Set up graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case sig := <-quit:
		slog.Info("Received shutdown signal", "signal", sig)
		
		// Create context with timeout for shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		if err := srv.Stop(); err != nil {
			slog.Error("Error during server shutdown", "error", err)
		} else {
			slog.Info("Server shutdown completed")
		}
	}
}

// createSampleFiles creates sample files in the sandbox directory for demonstration
func createSampleFiles(sandboxDir string) {
	files := map[string]string{
		"welcome.txt": `Welcome to Hyperserve MCP Example!

This file demonstrates the file reading capabilities of the MCP server.
You can read this file using the 'read_file' tool via the MCP protocol.

Features:
- Secure file access using Go 1.24's os.Root
- Restricted to sandbox directory
- JSON-RPC 2.0 protocol
- Multiple built-in tools and resources

Try using the MCP client to:
1. List files in this directory
2. Read this file's contents
3. Perform calculations
4. Make HTTP requests
5. Access server metrics and configuration

Happy coding with MCP! 🚀`,

		"data.json": `{
  "message": "Hello from MCP!",
  "timestamp": "2024-01-01T00:00:00Z",
  "server": "hyperserve",
  "protocol": "MCP (Model Context Protocol)",
  "capabilities": [
    "tools",
    "resources", 
    "file_operations",
    "calculations",
    "http_requests"
  ]
}`,

		"numbers.txt": `1
2
3
4
5
10
15
20
25
30`,

		"config.yaml": `# Sample configuration file
name: mcp-example
version: 1.0.0
features:
  - file_access
  - calculations
  - http_requests
  - system_info
sandbox: true
security: enabled`,
	}

	for filename, content := range files {
		filePath := filepath.Join(sandboxDir, filename)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			slog.Warn("Failed to create sample file", "file", filename, "error", err)
		} else {
			slog.Debug("Created sample file", "file", filename)
		}
	}
	
	// Create a subdirectory with a file
	subDir := filepath.Join(sandboxDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		slog.Warn("Failed to create subdirectory", "error", err)
	} else {
		subFile := filepath.Join(subDir, "nested.txt")
		content := "This file is in a subdirectory.\nIt demonstrates directory traversal capabilities."
		if err := os.WriteFile(subFile, []byte(content), 0644); err != nil {
			slog.Warn("Failed to create nested file", "error", err)
		}
	}
}