package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osauer/hyperserve/pkg/mcp"
)

// TestDiscoveryEndpointCacheVary pins the cache-poisoning fix: any response
// whose body may vary on the Authorization header (DiscoveryAuthenticated)
// must not be stored by shared caches, and Vary: Authorization must always
// be set so caches that honor it key on Authorization too.
func TestDiscoveryEndpointCacheVary(t *testing.T) {
	cases := []struct {
		name         string
		policy       mcp.DiscoveryPolicy
		wantVary     string
		wantCacheCtl string
	}{
		{"public policy: shared cache OK", mcp.DiscoveryPublic, "Authorization", "public, max-age=300"},
		{"count policy: shared cache OK", mcp.DiscoveryCount, "Authorization", "public, max-age=300"},
		{"authenticated policy: must be private", mcp.DiscoveryAuthenticated, "Authorization", "private, max-age=60"},
		{"none policy: shared cache OK", mcp.DiscoveryNone, "Authorization", "public, max-age=300"},
	}

	paths := []string{"/.well-known/mcp.json", "/mcp/discover"}

	for _, tc := range cases {
		for _, path := range paths {
			t.Run(tc.name+" "+path, func(t *testing.T) {
				srv, err := NewServer(
					WithMCPSupport("test", "1.0.0"),
					WithMCPDiscoveryPolicy(tc.policy),
				)
				if err != nil {
					t.Fatalf("NewServer: %v", err)
				}

				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				srv.mux.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200", rec.Code)
				}
				if got := rec.Header().Get("Vary"); got != tc.wantVary {
					t.Errorf("Vary = %q, want %q", got, tc.wantVary)
				}
				if got := rec.Header().Get("Cache-Control"); got != tc.wantCacheCtl {
					t.Errorf("Cache-Control = %q, want %q", got, tc.wantCacheCtl)
				}
			})
		}
	}
}
