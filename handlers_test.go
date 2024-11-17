package hyperserve

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func WriteErrorResponseSetsCorrectHeadersAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErrorResponse(rec, http.StatusBadRequest, "Bad Request")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %v, got %v", http.StatusBadRequest, rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %v", rec.Header().Get("Content-Type"))
	}
	expectedBody := `{"error":"Bad Request"}`
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %v, got %v", expectedBody, rec.Body.String())
	}
}

func TemplateHandlerRendersTemplate(t *testing.T) {
	templates = template.Must(template.New("test").Parse("<html><body>{{.}}</body></html>"))
	handler := templateHandler("test", "Hello, World!")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	expectedBody := "<html><body>Hello, World!</body></html>"
	if rec.Body.String() != expectedBody {
		t.Errorf("expected body %v, got %v", expectedBody, rec.Body.String())
	}
}

func TemplateHandlerReturnsErrorOnMissingTemplate(t *testing.T) {
	handler := templateHandler("missing", "Hello, World!")
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %v, got %v", http.StatusInternalServerError, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Error rendering template") {
		t.Errorf("expected body to contain 'Error rendering template', got %v", rec.Body.String())
	}
}

func HealthCheckHandlerReturnsNoContent(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	HealthCheckHandler(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status %v, got %v", http.StatusNoContent, rec.Code)
	}
}

func LivezHandlerReturnsAliveWhenRunning(t *testing.T) {
	srv := &Server{}
	isRunning.Store(true)
	req := httptest.NewRequest("GET", "/livez", nil)
	rec := httptest.NewRecorder()
	srv.livezHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "alive" {
		t.Errorf("expected body 'alive', got %v", rec.Body.String())
	}
}

func LivezHandlerReturnsUnhealthyWhenNotRunning(t *testing.T) {
	srv := &Server{}
	isRunning.Store(false)
	req := httptest.NewRequest("GET", "/livez", nil)
	rec := httptest.NewRecorder()
	srv.livezHandler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %v, got %v", http.StatusServiceUnavailable, rec.Code)
	}
	if rec.Body.String() != "unhealthy" {
		t.Errorf("expected body 'unhealthy', got %v", rec.Body.String())
	}
}

func ReadyzHandlerReturnsReadyWhenReady(t *testing.T) {
	srv := &Server{}
	isReady.Store(true)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.readyzHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ready" {
		t.Errorf("expected body 'ready', got %v", rec.Body.String())
	}
}

func ReadyzHandlerReturnsUnhealthyWhenNotReady(t *testing.T) {
	srv := &Server{}
	isReady.Store(false)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.readyzHandler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %v, got %v", http.StatusServiceUnavailable, rec.Code)
	}
	if rec.Body.String() != "unhealthy" {
		t.Errorf("expected body 'unhealthy', got %v", rec.Body.String())
	}
}

func HealthzHandlerReturnsOkWhenRunning(t *testing.T) {
	srv := &Server{}
	isRunning.Store(true)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.healthzHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %v, got %v", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %v", rec.Body.String())
	}
}

func HealthzHandlerReturnsUnhealthyWhenNotRunning(t *testing.T) {
	srv := &Server{}
	isRunning.Store(false)
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.healthzHandler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %v, got %v", http.StatusServiceUnavailable, rec.Code)
	}
	if rec.Body.String() != "unhealthy" {
		t.Errorf("expected body 'unhealthy', got %v", rec.Body.String())
	}
}

func PanicHandlerCausesPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic, but code did not panic")
		}
	}()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	PanicHandler(rec, req)
}
