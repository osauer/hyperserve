package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeadersMiddlewareCORSAllowedOrigin(t *testing.T) {
	srv, err := New(WithCORS(&CORSOptions{
		AllowedOrigins:   []string{"http://localhost:*"},
		AllowCredentials: true,
	}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.UsePrefix("/cors", SecureWeb(srv.options))
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
	srv, err := New(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://app.example.com"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.UsePrefix("/cors", SecureWeb(srv.options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
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
	srv, err := New(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://trusted.example"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.UsePrefix("/cors", SecureWeb(srv.options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
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
	srv, err := New(WithCORS(&CORSOptions{AllowedOrigins: []string{"*"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.UsePrefix("/cors", SecureWeb(srv.options))
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

func TestHeadersMiddlewareCORSOptionsWithoutOrigin(t *testing.T) {
	srv, err := New(WithCORS(&CORSOptions{AllowedOrigins: []string{"https://allowed.example"}}))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.UsePrefix("/cors", SecureWeb(srv.options))
	srv.HandleFunc("/cors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srv.middleware.applyToMux(srv.mux)

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected handler response for OPTIONS without origin, got %d", rec.Code)
	}
}

func TestSecureWebPreservesApplicationOptions(t *testing.T) {
	for _, tt := range []struct {
		name          string
		cors          *CORSOptions
		origin        string
		requestMethod string
	}{
		{"no CORS", nil, "", ""},
		{"no CORS with preflight headers", nil, "https://app.example", "POST"},
		{"CORS without preflight headers", &CORSOptions{AllowedOrigins: []string{"*"}}, "", ""},
		{"CORS without request method", &CORSOptions{AllowedOrigins: []string{"*"}}, "https://app.example", ""},
		{"CORS without origin", &CORSOptions{AllowedOrigins: []string{"*"}}, "", "POST"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := New(WithCORS(tt.cors))
			if err != nil {
				t.Fatal(err)
			}
			srv.Use(SecureWeb(srv.Options()))
			srv.OPTIONS("/resource", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Allow", "GET, OPTIONS")
				w.WriteHeader(http.StatusOK)
			})
			r := httptest.NewRequest(http.MethodOptions, "/resource", nil)
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("Access-Control-Request-Method", tt.requestMethod)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Header().Get("Allow") != "GET, OPTIONS" {
				t.Fatalf("application OPTIONS handler bypassed: status=%d headers=%v", w.Code, w.Header())
			}
		})
	}
}
