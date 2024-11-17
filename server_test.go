package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Content serving tests

func HandleTemplateValidTemplate(t *testing.T) {
	srv, _ := NewServer()
	srv.Options.TemplateDir = "./test_templates"

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

func HandleTemplateMissingTemplate(t *testing.T) {
	srv, _ := NewServer()
	srv.Options.TemplateDir = "./test_templates"

	// Test missing template
	srv.HandleTemplate("/missing", "missing.html", "Hello, World!")
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %v, got %v", http.StatusInternalServerError, rec.Code)
	}
	expectedError := "Failed to load content"
	if !strings.Contains(rec.Body.String(), expectedError) {
		t.Errorf("expected body to contain %v, got %v", expectedError, rec.Body.String())
	}
}

func HandleFuncDynamicValidTemplate(t *testing.T) {
	srv, _ := NewServer()
	srv.Options.TemplateDir = "./test_templates"

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

func HandleFuncDynamicMissingTemplate(t *testing.T) {
	srv, _ := NewServer()
	srv.Options.TemplateDir = "./test_templates"

	// Test missing dynamic template
	srv.HandleFuncDynamic("/missing", "missing.html", func(r *http.Request) interface{} {
		return map[string]interface{}{
			"timestamp": "2024-01-01 00:00:00",
		}
	})
	req := httptest.NewRequest("GET", "/missing", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %v, got %v", http.StatusInternalServerError, rec.Code)
	}
	expectedError := "Failed to load content"
	if !strings.Contains(rec.Body.String(), expectedError) {
		t.Errorf("expected body to contain %v, got %v", expectedError, rec.Body.String())
	}
}
