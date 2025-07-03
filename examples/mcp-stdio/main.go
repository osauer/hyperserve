// Package main demonstrates hyperserve's MCP support as a stdio server for Claude Desktop.
// This allows Claude to interact with your local system through MCP tools and resources.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// JSON-RPC 2.0 message types
type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP-specific types
type initializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type resourceReadParams struct {
	URI string `json:"uri"`
}

var (
	sandboxDir string
	verbose    bool
)

func main() {
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

	// Set up logging to stderr (stdout is for JSON-RPC)
	if verbose {
		log.SetOutput(os.Stderr)
		log.Printf("MCP stdio server starting (sandbox: %s)", sandboxDir)
	} else {
		log.SetOutput(io.Discard)
	}

	// Create stdio transport
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	// Main message loop
	for scanner.Scan() {
		var msg jsonrpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			log.Printf("Failed to parse JSON-RPC message: %v", err)
			continue
		}

		log.Printf("Received: %s (id: %v)", msg.Method, msg.ID)

		// Handle the request
		response := handleRequest(msg)
		if response != nil {
			response.JSONRPC = "2.0"
			response.ID = msg.ID
			
			if err := encoder.Encode(response); err != nil {
				log.Printf("Failed to encode response: %v", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
}

func handleRequest(msg jsonrpcMessage) *jsonrpcMessage {
	switch msg.Method {
	case "initialize":
		return handleInitialize(msg)
	case "initialized":
		// No response needed
		return nil
	case "tools/list":
		return handleToolsList()
	case "tools/call":
		return handleToolCall(msg)
	case "resources/list":
		return handleResourcesList()
	case "resources/read":
		return handleResourceRead(msg)
	default:
		return &jsonrpcMessage{
			Error: &jsonrpcError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
	}
}

func handleInitialize(msg jsonrpcMessage) *jsonrpcMessage {
	var params initializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errorResponse(-32602, "Invalid params")
	}

	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": false,
				},
				"resources": map[string]interface{}{
					"subscribe":   false,
					"listChanged": false,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "hyperserve-mcp-stdio",
				"version": "1.0.0",
			},
		},
	}
}

func handleToolsList() *jsonrpcMessage {
	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "calculator",
					"description": "Perform basic mathematical operations",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"operation": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"add", "subtract", "multiply", "divide"},
								"description": "The mathematical operation to perform",
							},
							"a": map[string]interface{}{
								"type":        "number",
								"description": "First operand",
							},
							"b": map[string]interface{}{
								"type":        "number",
								"description": "Second operand",
							},
						},
						"required": []string{"operation", "a", "b"},
					},
				},
				{
					"name":        "read_file",
					"description": "Read the contents of a file from the sandbox",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "Path to the file (relative to sandbox)",
							},
						},
						"required": []string{"path"},
					},
				},
				{
					"name":        "list_directory",
					"description": "List files in a directory",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "Directory path (relative to sandbox)",
								"default":     ".",
							},
						},
					},
				},
			},
		},
	}
}

func handleToolCall(msg jsonrpcMessage) *jsonrpcMessage {
	var params toolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errorResponse(-32602, "Invalid params")
	}

	switch params.Name {
	case "calculator":
		return handleCalculator(params.Arguments)
	case "read_file":
		return handleReadFile(params.Arguments)
	case "list_directory":
		return handleListDirectory(params.Arguments)
	default:
		return errorResponse(-32602, fmt.Sprintf("Unknown tool: %s", params.Name))
	}
}

func handleCalculator(args map[string]interface{}) *jsonrpcMessage {
	op, _ := args["operation"].(string)
	a, _ := args["a"].(float64)
	b, _ := args["b"].(float64)

	var result float64
	var err error

	switch op {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			err = fmt.Errorf("division by zero")
		} else {
			result = a / b
		}
	default:
		err = fmt.Errorf("unknown operation: %s", op)
	}

	if err != nil {
		return errorResponse(-32603, err.Error())
	}

	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("%.2f %s %.2f = %.2f", a, op, b, result),
				},
			},
		},
	}
}

func handleReadFile(args map[string]interface{}) *jsonrpcMessage {
	path, _ := args["path"].(string)
	
	// Ensure path is within sandbox
	fullPath := filepath.Join(sandboxDir, filepath.Clean(path))
	// Use strings.HasPrefix after cleaning both paths
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(sandboxDir)) {
		return errorResponse(-32603, "Access denied: path outside sandbox")
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return errorResponse(-32603, err.Error())
	}

	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": string(content),
				},
			},
		},
	}
}

func handleListDirectory(args map[string]interface{}) *jsonrpcMessage {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	// Ensure path is within sandbox
	fullPath := filepath.Join(sandboxDir, filepath.Clean(path))
	// Use strings.HasPrefix after cleaning both paths
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(sandboxDir)) {
		return errorResponse(-32603, "Access denied: path outside sandbox")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return errorResponse(-32603, err.Error())
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			files = append(files, entry.Name()+"/")
		} else {
			files = append(files, entry.Name())
		}
	}

	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": fmt.Sprintf("Files in %s:\n%s", path, formatFileList(files)),
				},
			},
		},
	}
}

func handleResourcesList() *jsonrpcMessage {
	return &jsonrpcMessage{
		Result: map[string]interface{}{
			"resources": []map[string]interface{}{
				{
					"uri":         "config://sandbox/info",
					"name":        "Sandbox Information",
					"description": "Information about the sandbox directory",
					"mimeType":    "application/json",
				},
			},
		},
	}
}

func handleResourceRead(msg jsonrpcMessage) *jsonrpcMessage {
	var params resourceReadParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return errorResponse(-32602, "Invalid params")
	}

	switch params.URI {
	case "config://sandbox/info":
		info, _ := os.Stat(sandboxDir)
		return &jsonrpcMessage{
			Result: map[string]interface{}{
				"contents": []map[string]interface{}{
					{
						"uri":      params.URI,
						"mimeType": "application/json",
						"text": mustMarshalJSON(map[string]interface{}{
							"path":        sandboxDir,
							"exists":      info != nil,
							"writable":    true,
							"description": "Sandbox directory for file operations",
						}),
					},
				},
			},
		}
	default:
		return errorResponse(-32602, fmt.Sprintf("Unknown resource: %s", params.URI))
	}
}

func errorResponse(code int, message string) *jsonrpcMessage {
	return &jsonrpcMessage{
		Error: &jsonrpcError{
			Code:    code,
			Message: message,
		},
	}
}

func formatFileList(files []string) string {
	if len(files) == 0 {
		return "(empty)"
	}
	result := ""
	for _, f := range files {
		result += "- " + f + "\n"
	}
	return result
}

func mustMarshalJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
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