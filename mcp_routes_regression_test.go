package hyperserve_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hs "github.com/osauer/hyperserve/v2"
)

func TestMCPAndExactRootGETCompose(t *testing.T) {
	app, err := hs.New(hs.WithMCPSupport("witness", "1"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown(context.Background())
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("documented HTTP and MCP composition panics: %v", p)
		}
	}()
	app.GET("/{$}", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("home")) })
	h := app.Handler()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 200}, {"GET", "/mcp", 405}, {"POST", "/mcp", 200}, {"GET", "/.well-known/mcp.json", 200}, {"GET", "/mcp/discover", 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
}

// A methodless application fallback must not turn rejected protocol requests
// into successful HTML responses, including extension HTTP methods.
func TestMCPRejectsMethodsBeforeApplicationFallback(t *testing.T) {
	app, err := hs.New(hs.WithMCPSupport("witness", "1"), hs.WithMCPEndpoint("/rpc"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Shutdown(context.Background())
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("fallback")) })
	app.DELETE("/rpc", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) })
	app.UsePrefix("/rpc", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware", "yes")
			next.ServeHTTP(w, r)
		})
	})
	for _, path := range []string{"/rpc", "/rpc/discover", "/.well-known/mcp.json"} {
		for _, method := range []string{"PUT", "PATCH", "OPTIONS", "HEAD", "BREW"} {
			w := httptest.NewRecorder()
			app.Handler().ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 405 {
				t.Errorf("%s %s = %d, want 405", method, path, w.Code)
			}
			if strings.HasPrefix(path, "/rpc") && w.Header().Get("X-Middleware") != "yes" {
				t.Error("protocol rejection bypassed middleware")
			}
		}
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest("DELETE", "/rpc", nil))
	if w.Code != 202 {
		t.Fatalf("explicit application method route = %d, want 202", w.Code)
	}
}
