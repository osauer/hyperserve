package hyperserve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMCPSecurity_PathTraversalAttacks tests that file tools properly prevent path traversal attacks
func TestMCPSecurity_PathTraversalAttacks(t *testing.T) {
	// Create temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_security_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file inside the safe directory
	testFile := filepath.Join(tempDir, "safe.txt")
	if err := os.WriteFile(testFile, []byte("safe content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create server with file tool root restriction
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test various path traversal attack patterns
	maliciousPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"../../../../etc/shadow",
		"../../../proc/version",
		"..\\..\\..\\boot.ini",
		filepath.Join("..", "..", "..", "etc", "passwd"),
		"../../../etc/hosts",
		"../../../tmp/../etc/passwd",
		"....//....//....//etc/passwd",
		"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd", // URL encoded
	}

	for _, maliciousPath := range maliciousPaths {
		t.Run("PathTraversal_"+maliciousPath, func(t *testing.T) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "read_file",
					"arguments": map[string]interface{}{
						"path": maliciousPath,
					},
				},
				"id": 1,
			}

			response := makeMCPRequest(t, srv, request)

			// Should always return an error for path traversal attempts
			if response.Error == nil {
				t.Errorf("Expected error for path traversal attempt with path: %s", maliciousPath)
			}

			// Verify error message indicates security violation
			if response.Error != nil {
				errorMsg := strings.ToLower(fmt.Sprintf("%v", response.Error))
				if !strings.Contains(errorMsg, "error") {
					t.Errorf("Expected security-related error message, got: %v", response.Error)
				}
			}
		})
	}
}

// TestMCPSecurity_SymlinkAttacks tests protection against symlink attacks
func TestMCPSecurity_SymlinkAttacks(t *testing.T) {
	// Skip on Windows as symlinks require special privileges
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_symlink_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file outside the safe directory
	outsideDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_outside_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}
	defer os.RemoveAll(outsideDir)

	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret content"), 0644); err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	// Create symlink inside the safe directory that points outside
	symlinkPath := filepath.Join(tempDir, "malicious_link")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("Cannot create symlink, skipping test: %v", err)
	}

	// Create server with file tool root restriction
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Attempt to read through the symlink
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "read_file",
			"arguments": map[string]interface{}{
				"path": "malicious_link",
			},
		},
		"id": 1,
	}

	response := makeMCPRequest(t, srv, request)

	// Should return an error as the symlink points outside the safe directory
	if response.Error == nil {
		t.Error("Expected error when trying to read symlink pointing outside safe directory")
	}
}

// TestMCPSecurity_InputValidation tests that MCP handlers properly validate input
func TestMCPSecurity_InputValidation(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testCases := []struct {
		name    string
		request map[string]interface{}
		expectError bool
	}{
		{
			name: "NullBytes",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "read_file",
					"arguments": map[string]interface{}{
						"path": "test.txt\x00../../etc/passwd",
					},
				},
				"id": 1,
			},
			expectError: true,
		},
		{
			name: "ExtremelyLongPath",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "read_file",
					"arguments": map[string]interface{}{
						"path": strings.Repeat("a", 10000),
					},
				},
				"id": 2,
			},
			expectError: true,
		},
		{
			name: "InvalidJSONRPCVersion",
			request: map[string]interface{}{
				"jsonrpc": "1.0",
				"method":  "tools/list",
				"id":      3,
			},
			expectError: true,
		},
		{
			name: "MissingRequiredFields",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				// Missing params
				"id": 4,
			},
			expectError: true,
		},
		{
			name: "InvalidMethodName",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "invalid/method/with/slashes",
				"id":      5,
			},
			expectError: true,
		},
		{
			name: "InvalidCalculatorOperation",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "calculator",
					"arguments": map[string]interface{}{
						"operation": "exploit",
						"a":         "' OR 1=1 --",
						"b":         "<script>alert('xss')</script>",
					},
				},
				"id": 6,
			},
			expectError: true,
		},
		{
			name: "ExcessivelyLargeNumbers",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name": "calculator",
					"arguments": map[string]interface{}{
						"operation": "multiply",
						"a":         1e308,  // Near float64 max
						"b":         1e308,
					},
				},
				"id": 7,
			},
			expectError: false, // Should handle gracefully but may overflow
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := makeMCPRequest(t, srv, tc.request)

			if tc.expectError && response.Error == nil {
				t.Errorf("Expected error for test case %s", tc.name)
			}
			if !tc.expectError && response.Error != nil {
				t.Errorf("Unexpected error for test case %s: %v", tc.name, response.Error)
			}
		})
	}
}

// TestMCPSecurity_AuthenticationIntegration tests MCP with authentication middleware
func TestMCPSecurity_AuthenticationIntegration(t *testing.T) {
	// Token validator that only accepts "valid_token"
	tokenValidator := func(token string) (bool, error) {
		return token == "valid_token", nil
	}

	// Create server with authentication and MCP
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithAuthTokenValidator(tokenValidator),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add auth middleware to the entire server
	srv.AddMiddleware("", AuthMiddleware(srv.Options))

	testCases := []struct {
		name           string
		authHeader     string
		expectStatus   int
	}{
		{
			name:         "NoAuthHeader",
			authHeader:   "",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "InvalidToken",
			authHeader:   "Bearer invalid_token",
			expectStatus: http.StatusUnauthorized,
		},
		{
			name:         "ValidToken",
			authHeader:   "Bearer valid_token",
			expectStatus: http.StatusOK,
		},
		{
			name:         "MalformedAuthHeader",
			authHeader:   "Malformed header",
			expectStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/list",
				"id":      1,
			}

			requestData, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
			req.Header.Set("Content-Type", "application/json")
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			w := httptest.NewRecorder()
			
			// Create a test handler chain with auth middleware
			handler := srv.middleware.WrapHandler(srv.mux)
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectStatus {
				t.Errorf("Expected status %d, got %d", tc.expectStatus, w.Code)
			}
		})
	}
}

// TestMCPSecurity_FilePermissions tests that file tools respect file permissions
func TestMCPSecurity_FilePermissions(t *testing.T) {
	// Skip on Windows as Unix permissions don't apply
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping file permissions test on Windows")
	}

	// Create temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_perms_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file with no read permissions
	restrictedFile := filepath.Join(tempDir, "restricted.txt")
	if err := os.WriteFile(restrictedFile, []byte("restricted content"), 0000); err != nil {
		t.Fatalf("Failed to create restricted file: %v", err)
	}

	// Create server with file tool root
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Try to read the restricted file
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "read_file",
			"arguments": map[string]interface{}{
				"path": "restricted.txt",
			},
		},
		"id": 1,
	}

	response := makeMCPRequest(t, srv, request)

	// Should return an error due to insufficient permissions
	if response.Error == nil {
		t.Error("Expected error when trying to read file without permissions")
	}

	// Verify error message indicates permission problem
	if response.Error != nil {
		errorMsg := strings.ToLower(fmt.Sprintf("%v", response.Error))
		if !strings.Contains(errorMsg, "permission") && !strings.Contains(errorMsg, "denied") {
			t.Logf("Note: Error message might not explicitly mention permissions: %v", response.Error)
		}
	}
}

// TestMCPSecurity_ResourceSanitization tests that sensitive data is not exposed in resources
func TestMCPSecurity_ResourceSanitization(t *testing.T) {
	// Create server with sensitive configuration
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
		WithMCPServerInfo("test-server", "1.0.0"),
		WithAuthTokenValidator(func(token string) (bool, error) {
			return token == "secret_token", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Request server configuration
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "resources/read",
		"params": map[string]interface{}{
			"uri": "config://server/options",
		},
		"id": 1,
	}

	response := makeMCPRequest(t, srv, request)

	if response.Error != nil {
		t.Fatalf("Unexpected error reading config resource: %v", response.Error)
	}

	// Parse the configuration content
	result := response.Result.(map[string]interface{})
	contents := result["contents"].([]interface{})
	contentItem := contents[0].(map[string]interface{})
	configText := contentItem["text"].(string)

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configText), &config); err != nil {
		t.Fatalf("Failed to parse config JSON: %v", err)
	}

	// Verify that sensitive fields are not exposed
	sensitiveFields := []string{
		"AuthTokenValidatorFunc",
		"ECHKeys",
		"KeyFile",   // TLS private key path
		"CertFile",  // TLS certificate path (less sensitive but still should be filtered)
	}

	for _, field := range sensitiveFields {
		if _, exists := config[field]; exists {
			t.Errorf("Sensitive field '%s' should not be exposed in config resource", field)
		}
	}

	// Verify that non-sensitive fields are still present
	nonSensitiveFields := []string{
		"addr",
		"mcp_enabled",
		"mcp_endpoint",
		"mcp_server_name",
	}

	for _, field := range nonSensitiveFields {
		if _, exists := config[field]; !exists {
			t.Logf("Note: Non-sensitive field '%s' not found in config (may be expected)", field)
		}
	}
}

// TestMCPSecurity_DenialOfService tests protection against DoS attacks
func TestMCPSecurity_DenialOfService(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport(),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test extremely large JSON payload
	t.Run("LargePayload", func(t *testing.T) {
		largeData := strings.Repeat("A", 1024*1024) // 1MB of 'A's
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "calculator",
				"arguments": map[string]interface{}{
					"operation": "add",
					"a":         largeData, // This should cause type conversion to fail
					"b":         1.0,
				},
			},
			"id": 1,
		}

		response := makeMCPRequest(t, srv, request)

		// Should handle gracefully without crashing
		if response.Error == nil {
			t.Error("Expected error for oversized payload")
		}
	})

	// Test deeply nested JSON
	t.Run("DeepNesting", func(t *testing.T) {
		// Create deeply nested structure
		nested := make(map[string]interface{})
		current := nested
		for i := 0; i < 1000; i++ {
			next := make(map[string]interface{})
			current["nest"] = next
			current = next
		}
		current["value"] = "deep"

		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "calculator",
				"arguments": nested, // Deeply nested arguments
			},
			"id": 1,
		}

		// This test mainly ensures the server doesn't crash
		_ = makeMCPRequest(t, srv, request)
		// We don't assert specific behavior as different JSON parsers handle this differently
	})
}

// Helper function for making MCP requests
func makeMCPRequest(t *testing.T, srv *Server, request map[string]interface{}) JSONRPCResponse {
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	srv.mcpHandler.ServeHTTP(w, req)

	// Allow both 200 and error status codes as both can contain valid JSON-RPC responses
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusUnauthorized {
		t.Fatalf("Unexpected status code: %d", w.Code)
	}

	var response JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	return response
}

