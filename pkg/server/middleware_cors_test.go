package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadersMiddlewareCORSAllowedOrigin(t *testing.T) {
	srv, err := NewServer(WithCORS(&CORSOptions{
		AllowedOrigins:   []string{"http://localhost:*"},
		AllowCredentials: true,
	}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.AddMiddlewareStack("/cors", SecureWeb(srv.Options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodGet, "/cors", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow origin to echo request origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials to be allowed, got %q", got)
	}

	vary := rec.Header().Values("Vary")
	if len(vary) == 0 {
		t.Fatalf("expected Vary header to be set")
	}
}

func TestHeadersMiddlewareCORSPreflight(t *testing.T) {
	srv, err := NewServer(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://app.example.com"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.AddMiddlewareStack("/cors", SecureWeb(srv.Options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected allow origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("expected allow methods header to be set")
	}
}

func TestHeadersMiddlewareCORSDisallowedOrigin(t *testing.T) {
	srv, err := NewServer(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://trusted.example"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.AddMiddlewareStack("/cors", SecureWeb(srv.Options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed origin, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow origin header, got %q", got)
	}
}

func TestHeadersMiddlewareCORSSimpleWildcard(t *testing.T) {
	srv, err := NewServer(WithCORS(&CORSOptions{AllowedOrigins: []string{"*"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.AddMiddlewareStack("/cors", SecureWeb(srv.Options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodGet, "/cors", nil)
	req.Header.Set("Origin", "https://another.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard allow origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("expected credentials header to be cleared, got %q", got)
	}
}

func TestHeadersMiddlewareCORSNoOriginPreflight(t *testing.T) {
	srv, err := NewServer(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://allowed.example"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.AddMiddlewareStack("/cors", SecureWeb(srv.Options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS without origin, got %d", rec.Code)
	}
}
