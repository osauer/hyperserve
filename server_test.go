package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Content serving tests

func TestHandleTemplateValidTemplate(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	// Use unique directory name to avoid conflicts in parallel tests
	srv.Options.TemplateDir = fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())

	// Create a temporary template file
	templateContent := "<html><body>{{.}}</body></html>"
	err := os.MkdirAll(srv.Options.TemplateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(srv.Options.TemplateDir+"/test.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	defer os.RemoveAll(srv.Options.TemplateDir)

	// Test valid template rendering
	srv.HandleTemplate("/test", "test.html", "Hello, World!")
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
	srv, _ := NewServer()
	// Use unique directory name to avoid conflicts in parallel tests
	srv.Options.TemplateDir = fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())

	// Create a directory but no template file
	err := os.MkdirAll(srv.Options.TemplateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	defer os.RemoveAll(srv.Options.TemplateDir)

	// Test missing template - HandleTemplate should fail
	err = srv.HandleTemplate("/missing", "missing.html", "Hello, World!")
	if err == nil {
		t.Errorf("expected error when template is missing, got nil")
	}
}

func TestHandleFuncDynamicValidTemplate(t *testing.T) {
	t.Parallel()
	srv, _ := NewServer()
	// Use unique directory name to avoid conflicts in parallel tests
	srv.Options.TemplateDir = fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())

	// Create a temporary template file
	templateContent := "<html><body>{{.timestamp}}</body></html>"
	err := os.MkdirAll(srv.Options.TemplateDir, 0755)
	if err != nil {
		t.Fatalf("error creating template directory: %v", err)
	}
	err = os.WriteFile(srv.Options.TemplateDir+"/time.html", []byte(templateContent), 0644)
	if err != nil {
		t.Fatalf("error writing template file: %v", err)
	}
	defer os.RemoveAll(srv.Options.TemplateDir)

	// Test valid dynamic template rendering
	srv.HandleFuncDynamic("/time", "time.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"timestamp": "2024-01-01 00:00:00",
		}
	})
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
	srv, _ := NewServer()
	// Use unique directory name to avoid conflicts in parallel tests
	srv.Options.TemplateDir = fmt.Sprintf("./test_templates_%d_%d", time.Now().UnixNano(), os.Getpid())

	// Test missing directory - HandleFuncDynamic should fail
	err := srv.HandleFuncDynamic("/missing", "missing.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"timestamp": "2024-01-01 00:00:00",
		}
	})
	if err == nil {
		t.Errorf("expected error when template directory is missing, got nil")
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