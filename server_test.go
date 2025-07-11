package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Content serving tests

func TestHandleTemplateValidTemplate(t *testing.T) {
	t.Parallel()
	// Use unique directory name to avoid conflicts in parallel tests
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create a temporary template file before server creation
	templateContent := "<html><body>{{.}}</body></html>"
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(templateDir+"/test.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	
	// Create server with template directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test valid template rendering
	err = srv.HandleTemplate("/test", "test.html", "Hello, World!")
	if err != nil {
		t.Fatalf("failed to add template handler: %v", err)
	}
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	expectedBody := "<html><body>Hello, World!</body></html>"
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %v, got %v", expectedBody, rec.Body.String())
	}
}

func TestHandleTemplateMissingTemplate(t *testing.T) {
	t.Parallel()
	// Use unique directory name to avoid conflicts in parallel tests
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create a directory but no template file
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	
	// Create server with template directory that has no templates
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test missing template - HandleTemplate should fail
	err = srv.HandleTemplate("/missing", "missing.html", "Hello, World!")
	if err == nil {
		t.Errorf("expected error when template is missing, got nil")
	}
}

func TestHandleFuncDynamicValidTemplate(t *testing.T) {
	t.Parallel()
	// Use unique directory name to avoid conflicts in parallel tests
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create a temporary template file before server creation
	templateContent := "<html><body>{{.timestamp}}</body></html>"
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(templateDir+"/time.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	
	// Create server with template directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test valid dynamic template rendering
	err = srv.HandleFuncDynamic("/time", "time.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"timestamp": "2024-01-01 00:00:00",
		}
	})
	if err != nil {
		t.Fatalf("failed to add dynamic template handler: %v", err)
	}
	req := httptest.NewRequest("GET", "/time", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	expectedBody := "<html><body>2024-01-01 00:00:00</body></html>"
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %v, got %v", expectedBody, rec.Body.String())
	}
}

func TestHandleFuncDynamicMissingTemplate(t *testing.T) {
	t.Parallel()
	// Use unique directory name to avoid conflicts in parallel tests  
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	
	// Create the template directory first
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("failed to create template directory: %v", err)
	}
	defer os.RemoveAll(templateDir)
	
	// Create server with existing template directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test missing template file - HandleFuncDynamic should fail
	err = srv.HandleFuncDynamic("/missing", "missing.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"timestamp": "2024-01-01 00:00:00",
		}
	})
	if err == nil {
		t.Errorf("expected error when template file is missing, got nil")
	}
}

func TestHandleStatic(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	// Use unique directory name to avoid conflicts in parallel tests
	srv.Options.StaticDir = fmt.Sprintf("./test_static_%d_%d", time.Now().UnixNano(), os.Getpid())

	// Create a temporary static file
	staticContent := "Hello, Static World!"
	err := os.MkdirAll(srv.Options.StaticDir, 0755)
	if err != nil {
		t.Fatalf("error creating static directory: %v", err)
	}
	err = os.WriteFile(srv.Options.StaticDir+"/test.txt", []byte(staticContent), 0644)
	if err != nil {
		t.Fatalf("error writing static file: %v", err)
	}
	defer os.RemoveAll(srv.Options.StaticDir)

	// Test static file serving
	srv.HandleStatic("/static/")
	req := httptest.NewRequest("GET", "/static/test.txt", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != staticContent {
		t.Errorf("expected body %v, got %v", staticContent, rec.Body.String())
	}
}

func TestFIPSMode(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(WithFIPSMode())
	if err != nil {
		t.Fatalf("failed to create server with FIPS mode: %v", err)
	}

	if !srv.Options.FIPSMode {
		t.Error("expected FIPSMode to be enabled")
	}

	// Verify TLS config has FIPS-compliant settings
	tlsConfig := srv.tlsConfig()
	if len(tlsConfig.CipherSuites) != 6 { // FIPS mode has 6 cipher suites
		t.Errorf("expected 6 FIPS-compliant cipher suites, got %d", len(tlsConfig.CipherSuites))
	}
	if len(tlsConfig.CurvePreferences) != 2 { // FIPS mode only allows P256 and P384
		t.Errorf("expected 2 FIPS-compliant curves, got %d", len(tlsConfig.CurvePreferences))
	}
}

func TestEncryptedClientHello(t *testing.T) {
	t.Parallel()
	echKey := []byte("test-ech-key")
	srv, err := NewServer(WithEncryptedClientHello(echKey))
	if err != nil {
		t.Fatalf("failed to create server with ECH: %v", err)
	}

	if !srv.Options.EnableECH {
		t.Error("expected ECH to be enabled")
	}

	if len(srv.Options.ECHKeys) != 1 {
		t.Errorf("expected 1 ECH key, got %d", len(srv.Options.ECHKeys))
	}
}

// Template Directory Validation Tests
func TestWithTemplateDirInvalidDirectory(t *testing.T) {
	t.Parallel()
	
	// Test with non-existent directory
	_, err := NewServer(WithTemplateDir("/non/existent/directory"))
	if err == nil {
		t.Error("expected error for non-existent template directory")
	}
	
	expectedError := "template directory not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error to contain '%s', got: %s", expectedError, err.Error())
	}
}

func TestWithTemplateDirValidDirectory(t *testing.T) {
	t.Parallel()
	
	// Create a temporary directory
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)
	
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	
	// Test with valid directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Errorf("unexpected error for valid template directory: %v", err)
	}
	
	if srv.Options.TemplateDir != templateDir {
		t.Errorf("expected template directory to be %s, got %s", templateDir, srv.Options.TemplateDir)
	}
}

// Hardened Mode Tests
func TestWithHardenedMode(t *testing.T) {
	t.Parallel()
	
	srv, err := NewServer(WithHardenedMode())
	if err != nil {
		t.Fatalf("failed to create server with hardened mode: %v", err)
	}
	
	if !srv.Options.HardenedMode {
		t.Error("expected hardened mode to be enabled")
	}
}

// Environment Variable Parsing Tests  
func TestHardenedModeEnvironmentVariable(t *testing.T) {
	t.Parallel()
	
	// Save original environment
	originalEnv := os.Getenv("HS_HARDENED_MODE")
	defer func() {
		if originalEnv == "" {
			os.Unsetenv("HS_HARDENED_MODE")
		} else {
			os.Setenv("HS_HARDENED_MODE", originalEnv)
		}
	}()
	
	// Test with "true"
	os.Setenv("HS_HARDENED_MODE", "true")
	options := NewServerOptions()
	if !options.HardenedMode {
		t.Error("expected hardened mode to be enabled with HS_HARDENED_MODE=true")
	}
	
	// Test with "1"
	os.Setenv("HS_HARDENED_MODE", "1")
	options = NewServerOptions()
	if !options.HardenedMode {
		t.Error("expected hardened mode to be enabled with HS_HARDENED_MODE=1")
	}
	
	// Test with "false"
	os.Setenv("HS_HARDENED_MODE", "false")
	options = NewServerOptions()
	if options.HardenedMode {
		t.Error("expected hardened mode to be disabled with HS_HARDENED_MODE=false")
	}
	
	// Test with empty value
	os.Setenv("HS_HARDENED_MODE", "")
	options = NewServerOptions()
	if options.HardenedMode {
		t.Error("expected hardened mode to be disabled with empty HS_HARDENED_MODE")
	}
}

// Enhanced Template Error Handling Tests
func TestHandleFuncDynamicTemplateErrors(t *testing.T) {
	t.Parallel()
	
	// Use unique directory name to avoid conflicts in parallel tests
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create a temporary template file with invalid syntax
	templateContent := "<html><body>{{.invalid syntax}}</body></html>"
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(templateDir+"/invalid.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	
	// Create server with template directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test that invalid template syntax is caught during HandleFuncDynamic
	err = srv.HandleFuncDynamic("/invalid", "invalid.html", func(r *http.Request) interface{} {
		return map[string]interface{}{"test": "data"}
	})
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestHandleFuncDynamicNonExistentTemplate(t *testing.T) {
	t.Parallel()
	
	// Use unique directory name to avoid conflicts in parallel tests
	templateDir := fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())
	defer os.RemoveAll(templateDir)

	// Create empty template directory
	err := os.MkdirAll(templateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	
	// Create server with template directory
	srv, err := NewServer(WithTemplateDir(templateDir))
	if err != nil {
		t.Fatalf("error creating server: %v", err)
	}

	// Test that non-existent template is caught during HandleFuncDynamic
	err = srv.HandleFuncDynamic("/missing", "missing.html", func(r *http.Request) interface{} {
		return map[string]interface{}{"test": "data"}
	})
	if err == nil {
		t.Error("expected error for non-existent template")
	}
	
	expectedError := "template missing.html not found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error to contain '%s', got: %s", expectedError, err.Error())
	}
}
