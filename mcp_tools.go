package hyperserve

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileReadTool implements MCPTool for reading files from the filesystem
type FileReadTool struct {
	root *os.Root // Secure file access using os.Root
}

// NewFileReadTool creates a new file read tool with optional root directory restriction
func NewFileReadTool(rootDir string) (*FileReadTool, error) {
	var root *os.Root
	if rootDir != "" {
		var err error
		root, err = os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
	}
	
	return &FileReadTool{root: root}, nil
}

func (t *FileReadTool) Name() string {
	return "read_file"
}

func (t *FileReadTool) Description() string {
	return "Read the contents of a file from the filesystem"
}

func (t *FileReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *FileReadTool) Execute(params map[string]interface{}) (interface{}, error) {
	path, ok := params["path"].(string)
	if !ok {
		return nil, fmt.Errorf("path parameter is required and must be a string")
	}
	
	// Clean the path
	path = filepath.Clean(path)
	
	var content []byte
	var err error
	
	if t.root != nil {
		// Use secure os.Root for file access
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		
		content, err = io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	} else {
		// Direct file system access (use with caution)
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}
	
	return string(content), nil
}

// ListDirectoryTool implements MCPTool for listing directory contents
type ListDirectoryTool struct {
	root *os.Root
}

// NewListDirectoryTool creates a new directory listing tool
func NewListDirectoryTool(rootDir string) (*ListDirectoryTool, error) {
	var root *os.Root
	if rootDir != "" {
		var err error
		root, err = os.OpenRoot(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open root directory: %w", err)
		}
	}
	
	return &ListDirectoryTool{root: root}, nil
}

func (t *ListDirectoryTool) Name() string {
	return "list_directory"
}

func (t *ListDirectoryTool) Description() string {
	return "List the contents of a directory"
}

func (t *ListDirectoryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the directory to list",
				"default":     ".",
			},
		},
	}
}

func (t *ListDirectoryTool) Execute(params map[string]interface{}) (interface{}, error) {
	path := "."
	if p, ok := params["path"].(string); ok {
		path = p
	}
	
	path = filepath.Clean(path)
	
	var entries []os.DirEntry
	var err error
	
	if t.root != nil {
		// Use secure os.Root for directory access
		file, err := t.root.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open directory: %w", err)
		}
		defer file.Close()
		
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
	
	var files []map[string]interface{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't get info for
		}
		
		files = append(files, map[string]interface{}{
			"name":    entry.Name(),
			"type":    getFileType(entry),
			"size":    info.Size(),
			"modTime": info.ModTime().Format(time.RFC3339),
		})
	}
	
	return files, nil
}

func getFileType(entry os.DirEntry) string {
	if entry.IsDir() {
		return "directory"
	}
	return "file"
}

// HTTPRequestTool implements MCPTool for making HTTP requests
type HTTPRequestTool struct {
	client *http.Client
}

// NewHTTPRequestTool creates a new HTTP request tool
func NewHTTPRequestTool() *HTTPRequestTool {
	return &HTTPRequestTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *HTTPRequestTool) Name() string {
	return "http_request"
}

func (t *HTTPRequestTool) Description() string {
	return "Make HTTP requests to external services"
}

func (t *HTTPRequestTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to make the request to",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, DELETE, etc.)",
				"default":     "GET",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers as key-value pairs",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Request body (for POST, PUT, etc.)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *HTTPRequestTool) Execute(params map[string]interface{}) (interface{}, error) {
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
	
	// Add headers
	if headers, ok := params["headers"].(map[string]interface{}); ok {
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
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	return map[string]interface{}{
		"status":     resp.Status,
		"statusCode": resp.StatusCode,
		"headers":    resp.Header,
		"body":       string(respBody),
	}, nil
}

// CalculatorTool implements MCPTool for basic mathematical operations
type CalculatorTool struct{}

// NewCalculatorTool creates a new calculator tool
func NewCalculatorTool() *CalculatorTool {
	return &CalculatorTool{}
}

func (t *CalculatorTool) Name() string {
	return "calculator"
}

func (t *CalculatorTool) Description() string {
	return "Perform basic mathematical calculations"
}

func (t *CalculatorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Mathematical operation to perform",
				"enum":        []string{"add", "subtract", "multiply", "divide"},
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
	}
}

func (t *CalculatorTool) Execute(params map[string]interface{}) (interface{}, error) {
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
	
	return map[string]interface{}{
		"result":    result,
		"operation": fmt.Sprintf("%.2f %s %.2f", a, operation, b),
	}, nil
}