package hyperserve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMethodRouteHelpersDispatchByVerb registers a handler per verb on the
// same path and verifies each verb hits the matching handler. Catches any
// pattern-string typo (extra space, wrong constant) that would route every
// request to the same handler.
func TestMethodRouteHelpersDispatchByVerb(t *testing.T) {
	t.Parallel()

	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	type registrar struct {
		method  string
		install func(string, http.HandlerFunc)
	}
	registrars := []registrar{
		{http.MethodGet, srv.GET},
		{http.MethodPost, srv.POST},
		{http.MethodPut, srv.PUT},
		{http.MethodPatch, srv.PATCH},
		{http.MethodDelete, srv.DELETE},
		{http.MethodHead, srv.HEAD},
		{http.MethodOptions, srv.OPTIONS},
	}

	for _, reg := range registrars {
		method := reg.method
		reg.install("/resource", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Handled-By", method)
			w.WriteHeader(http.StatusOK)
		})
	}

	handler := srv.Handler()

	for _, reg := range registrars {
		t.Run(reg.method, func(t *testing.T) {
			req := httptest.NewRequest(reg.method, "/resource", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s /resource: expected 200, got %d", reg.method, rec.Code)
			}
			if got := rec.Header().Get("X-Handled-By"); got != reg.method {
				t.Fatalf("%s /resource: handler reported %q, want %q", reg.method, got, reg.method)
			}
		})
	}
}

// TestMethodRouteHelpersReturn405ForWrongMethod registers only a POST on a
// path and confirms a GET to that path falls through to net/http's
// automatic 405 (with an Allow header) — proving the helpers don't quietly
// register catch-all routes.
func TestMethodRouteHelpersReturn405ForWrongMethod(t *testing.T) {
	t.Parallel()

	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.POST("/users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /users, got %d", rec.Code)
	}

	allow := rec.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodPost) {
		t.Fatalf("expected Allow header to include POST, got %q", allow)
	}
}

// TestMethodRouteHelpersExposePathValues verifies that wildcards captured
// by the stdlib mux are reachable via r.PathValue when routes are
// registered through the new helpers.
func TestMethodRouteHelpersExposePathValues(t *testing.T) {
	t.Parallel()

	srv, err := New()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	srv.GET("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.PathValue("id"))
	})

	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "42" {
		t.Fatalf("expected body %q, got %q", "42", rec.Body.String())
	}
}
