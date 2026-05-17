// Package builtin provides ready-to-register MCP tools and resources.
//
// The pure tools (Calculator, HTTPRequest, FileRead, ListDirectory) and the
// pure resources (Config, Metrics, System) have no dependency on the
// HyperServe server. Server-aware tools and resources live in server_tools.go
// and server_resources.go; they need *server.Server.
package builtin

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var logger = slog.Default()

// SetDefaultLogger overrides the logger used by the builtin package.
func SetDefaultLogger(l *slog.Logger) {
	if l == nil {
		logger = slog.Default()
		return
	}
	logger = l
}

// closeWithLog closes an io.Closer and logs any error.
func closeWithLog(c io.Closer, name string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		logger.Debug("failed to close resource", "name", name, "error", err)
	}
}

// FileReadTool implements an MCP tool that reads files from the filesystem.
type FileReadTool struct {
	root *os.Root // Secure file access via os.Root
}

// NewFileReadTool creates a FileReadTool. If rootDir is non-empty, all reads
// are restricted to that directory via os.OpenRoot.
func NewFileReadTool(rootDir string) (*FileReadTool, error) {
	var root *os.Root
	if rootDir != "" {
		r, err := os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
		root = r
	}
	return &FileReadTool{root: root}, nil
}

func (t *FileReadTool) Name() string { return "read_file" }

func (t *FileReadTool) Description() string {
	return "Read the contents of a file from the filesystem"
}

func (t *FileReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) Execute(params map[string]any) (any, error) {
	path, ok := params["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}
	path = filepath.Clean(path)

	var content []byte
	var err error
	if t.root != nil {
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer closeWithLog(file, path)

		content, err = io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}
	return string(content), nil
}

// ListDirectoryTool implements an MCP tool that lists directory contents.
type ListDirectoryTool struct {
	root *os.Root
}

// NewListDirectoryTool creates a ListDirectoryTool. If rootDir is non-empty,
// all listings are restricted to that directory via os.OpenRoot.
func NewListDirectoryTool(rootDir string) (*ListDirectoryTool, error) {
	var root *os.Root
	if rootDir != "" {
		r, err := os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
		root = r
	}
	return &ListDirectoryTool{root: root}, nil
}

func (t *ListDirectoryTool) Name() string { return "list_directory" }

func (t *ListDirectoryTool) Description() string { return "List the contents of a directory" }

func (t *ListDirectoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the directory to list",
				"default":     ".",
			},
		},
	}
}

func (t *ListDirectoryTool) Execute(params map[string]any) (any, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}
	path = filepath.Clean(path)

	var entries []os.DirEntry
	var err error
	if t.root != nil {
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open directory: %w", err)
		}
		defer closeWithLog(file, path)

		entries, err = file.ReadDir(-1)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
	} else {
		entries, err = os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}
	}

	var files []map[string]any
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, map[string]any{
			"name":    entry.Name(),
			"type":    fileType(entry),
			"size":    info.Size(),
			"modTime": info.ModTime().Format(time.RFC3339),
		})
	}
	return files, nil
}

func fileType(entry os.DirEntry) string {
	if entry.IsDir() {
		return "directory"
	}
	return "file"
}

// HTTPRequestTool implements an MCP tool that makes external HTTP requests.
type HTTPRequestTool struct {
	client *http.Client
}

// NewHTTPRequestTool creates an HTTPRequestTool with a 30s default timeout.
func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *HTTPRequestTool) Name() string { return "http_request" }

func (t *HTTPRequestTool) Description() string {
	return "Make HTTP requests to external services"
}

func (t *HTTPRequestTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to make the request to",
			},
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, DELETE, etc.)",
				"default":     "GET",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": "HTTP headers as key-value pairs",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Request body (for POST, PUT, etc.)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *HTTPRequestTool) Execute(params map[string]any) (any, error) {
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter is required and must be a string")
	}
	method := "GET"
	if m, ok := params["method"].(string); ok {
		method = strings.ToUpper(m)
	}
	var body io.Reader
	if b, ok := params["body"].(string); ok {
		body = strings.NewReader(b)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if headers, ok := params["headers"].(map[string]any); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer closeWithLog(resp.Body, "HTTP response body")

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return map[string]any{
		"status":     resp.Status,
		"statusCode": resp.StatusCode,
		"headers":    resp.Header,
		"body":       string(respBody),
	}, nil
}

// CalculatorTool implements an MCP tool for basic arithmetic.
type CalculatorTool struct{}

// NewCalculatorTool returns a CalculatorTool.
func NewCalculatorTool() *CalculatorTool { return &CalculatorTool{} }

func (t *CalculatorTool) Name() string { return "calculator" }

func (t *CalculatorTool) Description() string { return "Perform basic mathematical calculations" }

func (t *CalculatorTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Mathematical operation to perform",
				"enum":        []string{"add", "subtract", "multiply", "divide"},
			},
			"a": map[string]any{
				"type":        "number",
				"description": "First operand",
			},
			"b": map[string]any{
				"type":        "number",
				"description": "Second operand",
			},
		},
		"required": []string{"operation", "a", "b"},
	}
}

func (t *CalculatorTool) Execute(params map[string]any) (any, error) {
	operation, ok := params["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation parameter is required and must be a string")
	}

	var a, b float64
	if aVal, ok := params["a"].(float64); ok {
		a = aVal
	} else if aVal, ok := params["a"].(int); ok {
		a = float64(aVal)
	} else {
		return nil, fmt.Errorf("parameter 'a' must be a number")
	}
	if bVal, ok := params["b"].(float64); ok {
		b = bVal
	} else if bVal, ok := params["b"].(int); ok {
		b = float64(bVal)
	} else {
		return nil, fmt.Errorf("parameter 'b' must be a number")
	}

	var result float64
	switch operation {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		result = a / b
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}

	if math.IsInf(result, 0) || math.IsNaN(result) {
		return nil, fmt.Errorf("result is out of range: %v", result)
	}
	return map[string]any{
		"result":    result,
		"operation": fmt.Sprintf("%.2f %s %.2f", a, operation, b),
	}, nil
}
