package hyperserve_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hs "github.com/osauer/hyperserve"
)

func TestMCPAndRootGETCompose(t *testing.T) {
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
	app.GET("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("home")) })
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
