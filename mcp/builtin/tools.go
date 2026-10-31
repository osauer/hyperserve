// Package builtin provides ready-to-register MCP tools and resources.
// Import it for its registration side effect before constructing a HyperServe
// server with built-in MCP tools, resources, or presets:
//
//	import _ "github.com/osauer/hyperserve/v2/mcp/builtin"
//
// The pure tools (Calculator, FileRead, ListDirectory) and the pure resources
// (Config, Metrics, System) have no dependency on the HyperServe server.
// Server-aware tools and resources live in server_tools.go and
// server_resources.go; they need *hyperserve.Server.
//
// File tools are always sandboxed: NewFileReadTool and NewListDirectoryTool
// require a non-empty rootDir and return an error otherwise. There is no
// unsandboxed fallback. Callers that enable WithMCPBuiltinTools(true) without
// configuring WithMCPFileToolRoot will see a warn-log and the file tools will
// not be registered.
package builtin

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"time"
)

var logger = slog.Default()

// The previously-exported SetDefaultLogger had zero callers and was removed.
// Builtin tools use the package logger. MCP log resources wrap only their
// handler's injected logger and never replace this package logger.

// closeWithLog closes an io.Closer and logs any error.
func closeWithLog(c io.Closer, name string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		logger.Debug("failed to close resource", "name", name, "error", err)
	}
}

// FileReadTool implements an MCP tool that reads files from a sandboxed root.
type FileReadTool struct {
	root *os.Root // Secure file access via os.Root, never nil.
}

// NewFileReadTool creates a FileReadTool sandboxed to rootDir.
// Returns an error when rootDir is empty: the unsandboxed fallback that
// previously existed allowed arbitrary file reads with the process UID and
// is intentionally not reachable.
func NewFileReadTool(rootDir string) (*FileReadTool, error) {
	if rootDir == "" {
		return nil, fmt.Errorf("file read tool requires a non-empty rootDir; configure WithMCPFileToolRoot")
	}
	r, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open root directory %q: %w", rootDir, err)
	}
	return &FileReadTool{root: r}, nil
}

func (t *FileReadTool) Name() string { return "read_file" }

func (t *FileReadTool) Description() string {
	return "Read the contents of a file within the configured sandbox root"
}

func (t *FileReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read (relative to the sandbox root)",
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

	file, err := t.root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer closeWithLog(file, path)

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return string(content), nil
}

// ListDirectoryTool implements an MCP tool that lists directory contents
// within a sandboxed root.
type ListDirectoryTool struct {
	root *os.Root // never nil
}

// NewListDirectoryTool creates a ListDirectoryTool sandboxed to rootDir.
// Returns an error when rootDir is empty (see NewFileReadTool).
func NewListDirectoryTool(rootDir string) (*ListDirectoryTool, error) {
	if rootDir == "" {
		return nil, fmt.Errorf("list directory tool requires a non-empty rootDir; configure WithMCPFileToolRoot")
	}
	r, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open root directory %q: %w", rootDir, err)
	}
	return &ListDirectoryTool{root: r}, nil
}

func (t *ListDirectoryTool) Name() string { return "list_directory" }

func (t *ListDirectoryTool) Description() string {
	return "List the contents of a directory within the configured sandbox root"
}

func (t *ListDirectoryTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the directory to list (relative to the sandbox root)",
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

	file, err := t.root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open directory: %w", err)
	}
	defer closeWithLog(file, path)

	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
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
