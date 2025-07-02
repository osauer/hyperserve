package hyperserve

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCalculatorTool(t *testing.T) {
	calc := NewCalculatorTool()
	
	// Test tool metadata
	if calc.Name() != "calculator" {
		t.Errorf("Expected name 'calculator', got %s", calc.Name())
	}
	
	if calc.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	schema := calc.Schema()
	if schema == nil {
		t.Error("Schema should not be nil")
	}
	
	// Test addition
	result, err := calc.Execute(map[string]interface{}{
		"operation": "add",
		"a":         5.0,
		"b":         3.0,
	})
	if err != nil {
		t.Fatalf("Addition failed: %v", err)
	}
	
	resultMap := result.(map[string]interface{})
	if resultMap["result"] != 8.0 {
		t.Errorf("Expected 8.0, got %v", resultMap["result"])
	}
	
	// Test subtraction
	result, err = calc.Execute(map[string]interface{}{
		"operation": "subtract",
		"a":         10.0,
		"b":         4.0,
	})
	if err != nil {
		t.Fatalf("Subtraction failed: %v", err)
	}
	
	resultMap = result.(map[string]interface{})
	if resultMap["result"] != 6.0 {
		t.Errorf("Expected 6.0, got %v", resultMap["result"])
	}
	
	// Test multiplication
	result, err = calc.Execute(map[string]interface{}{
		"operation": "multiply",
		"a":         4.0,
		"b":         3.0,
	})
	if err != nil {
		t.Fatalf("Multiplication failed: %v", err)
	}
	
	resultMap = result.(map[string]interface{})
	if resultMap["result"] != 12.0 {
		t.Errorf("Expected 12.0, got %v", resultMap["result"])
	}
	
	// Test division
	result, err = calc.Execute(map[string]interface{}{
		"operation": "divide",
		"a":         15.0,
		"b":         3.0,
	})
	if err != nil {
		t.Fatalf("Division failed: %v", err)
	}
	
	resultMap = result.(map[string]interface{})
	if resultMap["result"] != 5.0 {
		t.Errorf("Expected 5.0, got %v", resultMap["result"])
	}
	
	// Test division by zero
	_, err = calc.Execute(map[string]interface{}{
		"operation": "divide",
		"a":         10.0,
		"b":         0.0,
	})
	if err == nil {
		t.Error("Expected division by zero error")
	}
	
	// Test invalid operation
	_, err = calc.Execute(map[string]interface{}{
		"operation": "invalid",
		"a":         5.0,
		"b":         3.0,
	})
	if err == nil {
		t.Error("Expected invalid operation error")
	}
	
	// Test missing parameters
	_, err = calc.Execute(map[string]interface{}{
		"operation": "add",
		"a":         5.0,
	})
	if err == nil {
		t.Error("Expected missing parameter error")
	}
}

func TestFileReadTool(t *testing.T) {
	// Create a temporary directory and file for testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, MCP World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Test with root directory restriction
	tool, err := NewFileReadTool(tempDir)
	if err != nil {
		t.Fatalf("Failed to create file read tool: %v", err)
	}
	
	// Test tool metadata
	if tool.Name() != "read_file" {
		t.Errorf("Expected name 'read_file', got %s", tool.Name())
	}
	
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	schema := tool.Schema()
	if schema == nil {
		t.Error("Schema should not be nil")
	}
	
	// Test reading existing file
	result, err := tool.Execute(map[string]interface{}{
		"path": "test.txt",
	})
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	
	if result != testContent {
		t.Errorf("Expected content '%s', got '%v'", testContent, result)
	}
	
	// Test reading non-existent file
	_, err = tool.Execute(map[string]interface{}{
		"path": "nonexistent.txt",
	})
	if err == nil {
		t.Error("Expected error reading non-existent file")
	}
	
	// Test missing path parameter
	_, err = tool.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing path parameter")
	}
}

func TestListDirectoryTool(t *testing.T) {
	// Create a temporary directory with some files for testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create test files and directories
	testFile1 := filepath.Join(tempDir, "file1.txt")
	testFile2 := filepath.Join(tempDir, "file2.txt")
	testDir := filepath.Join(tempDir, "subdir")
	
	if err := os.WriteFile(testFile1, []byte("content1"), 0644); err != nil {
		t.Fatalf("Failed to create test file1: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("content2"), 0644); err != nil {
		t.Fatalf("Failed to create test file2: %v", err)
	}
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	
	// Test without root directory restriction
	tool, err := NewListDirectoryTool("")
	if err != nil {
		t.Fatalf("Failed to create list directory tool: %v", err)
	}
	
	// Test tool metadata
	if tool.Name() != "list_directory" {
		t.Errorf("Expected name 'list_directory', got %s", tool.Name())
	}
	
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	schema := tool.Schema()
	if schema == nil {
		t.Error("Schema should not be nil")
	}
	
	// Test listing directory
	result, err := tool.Execute(map[string]interface{}{
		"path": tempDir,
	})
	if err != nil {
		t.Fatalf("Failed to list directory: %v", err)
	}
	
	files, ok := result.([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected slice of maps, got %T", result)
	}
	
	if len(files) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(files))
	}
	
	// Check that we have the expected entries
	foundFile1, foundFile2, foundDir := false, false, false
	for _, file := range files {
		name := file["name"].(string)
		switch name {
		case "file1.txt":
			foundFile1 = true
			if file["type"] != "file" {
				t.Errorf("Expected file1.txt to be type 'file', got %v", file["type"])
			}
		case "file2.txt":
			foundFile2 = true
			if file["type"] != "file" {
				t.Errorf("Expected file2.txt to be type 'file', got %v", file["type"])
			}
		case "subdir":
			foundDir = true
			if file["type"] != "directory" {
				t.Errorf("Expected subdir to be type 'directory', got %v", file["type"])
			}
		}
	}
	
	if !foundFile1 || !foundFile2 || !foundDir {
		t.Error("Not all expected entries were found")
	}
	
	// Test listing non-existent directory
	_, err = tool.Execute(map[string]interface{}{
		"path": "/nonexistent/directory",
	})
	if err == nil {
		t.Error("Expected error listing non-existent directory")
	}
}

func TestHTTPRequestTool(t *testing.T) {
	tool := NewHTTPRequestTool()
	
	// Test tool metadata
	if tool.Name() != "http_request" {
		t.Errorf("Expected name 'http_request', got %s", tool.Name())
	}
	
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	
	schema := tool.Schema()
	if schema == nil {
		t.Error("Schema should not be nil")
	}
	
	// Note: For a comprehensive test, we would need a test HTTP server
	// For now, test error conditions
	
	// Test missing URL parameter
	_, err := tool.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing URL parameter")
	}
	
	// Test invalid URL
	_, err = tool.Execute(map[string]interface{}{
		"url": "not-a-valid-url",
	})
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestCalculatorTool_IntegerParams(t *testing.T) {
	calc := NewCalculatorTool()
	
	// Test with integer parameters (should be converted to float64)
	result, err := calc.Execute(map[string]interface{}{
		"operation": "add",
		"a":         5,    // int
		"b":         3.5,  // float64
	})
	if err != nil {
		t.Fatalf("Addition with mixed types failed: %v", err)
	}
	
	resultMap := result.(map[string]interface{})
	if resultMap["result"] != 8.5 {
		t.Errorf("Expected 8.5, got %v", resultMap["result"])
	}
}

func TestFileReadTool_WithoutRoot(t *testing.T) {
	// Test file read tool without root directory restriction
	tool, err := NewFileReadTool("")
	if err != nil {
		t.Fatalf("Failed to create file read tool: %v", err)
	}
	
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "hyperserve_test_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	
	testContent := "Test content without root restriction"
	if _, err := tempFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()
	
	// Test reading the file
	result, err := tool.Execute(map[string]interface{}{
		"path": tempFile.Name(),
	})
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	
	if result != testContent {
		t.Errorf("Expected content '%s', got '%v'", testContent, result)
	}
}

func TestNewFileReadTool_InvalidRoot(t *testing.T) {
	// Test with invalid root directory
	_, err := NewFileReadTool("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error creating file read tool with invalid root")
	}
}

func TestNewListDirectoryTool_InvalidRoot(t *testing.T) {
	// Test with invalid root directory
	_, err := NewListDirectoryTool("/nonexistent/directory")
	if err == nil {
		t.Error("Expected error creating list directory tool with invalid root")
	}
}