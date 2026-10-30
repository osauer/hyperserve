package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimiterFollowsConfiguredEndpoint(t *testing.T) {
	for _, endpoint := range []string{"/mcp", "/rpc"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Chdir("../..")
			t.Setenv("HS_MCP_ENDPOINT", endpoint)
			app, err := newApp()
			if err != nil {
				t.Fatal(err)
			}
			defer app.Shutdown(t.Context())
			handler := app.Handler()
			limited := false
			for i := range 1000 {
				r := httptest.NewRequest("POST", endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
				r.Header.Set("Content-Type", "application/json")
				r.RemoteAddr = "127.0.0.1:1234"
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if i == 0 && w.Code != 200 {
					t.Fatalf("endpoint unavailable: %d %s", w.Code, w.Body)
				}
				if w.Code == 429 {
					limited = true
					break
				}
			}
			if !limited {
				t.Fatal("configured endpoint bypasses limiter")
			}
		})
	}
}
