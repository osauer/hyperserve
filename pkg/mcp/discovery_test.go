package mcp

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// stubTool is the minimal Tool implementation used by discovery tests.
type stubTool struct {
	name string
}

func (t *stubTool) Name() string                               { return t.name }
func (t *stubTool) Description() string                        { return "stub" }
func (t *stubTool) Schema() map[string]any                     { return map[string]any{"type": "object"} }
func (t *stubTool) Execute(params map[string]any) (any, error) { return nil, nil }

// hiddenTool implements `interface{ IsDiscoverable() bool }` returning false,
// the canonical opt-out path for tools that should not appear in discovery.
type hiddenTool struct{ stubTool }

func (t *hiddenTool) IsDiscoverable() bool { return false }

// visibleTool implements the same interface returning true; covers the
// branch in shouldExposeToolInDiscovery that consults the assertion.
type visibleTool struct{ stubTool }

func (t *visibleTool) IsDiscoverable() bool { return true }

// stubResource is the minimal Resource implementation used by discovery tests.
type stubResource struct {
	uri string
}

func (r *stubResource) URI() string             { return r.uri }
func (r *stubResource) Name() string            { return "stub" }
func (r *stubResource) Description() string     { return "stub resource" }
func (r *stubResource) MimeType() string        { return "text/plain" }
func (r *stubResource) Read() (any, error)      { return "ok", nil }
func (r *stubResource) List() ([]string, error) { return []string{r.uri}, nil }

func newDiscoveryHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(ServerInfo{Name: "test", Version: "0.0.1"})
}

func newDiscoveryRequest(authz string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/.well-known/mcp.json", nil)
	r.Host = "example.test"
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	return r
}

func TestBuildDiscoveryInfoBasics(t *testing.T) {
	h := newDiscoveryHandler(t)
	h.RegisterTool(&stubTool{name: "calc"})
	h.RegisterResource(&stubResource{uri: "config://x"})

	info := h.BuildDiscoveryInfo(newDiscoveryRequest(""), DiscoveryConfig{
		MCPEndpoint: "/mcp",
		DefaultAddr: ":8080",
		Transport:   HTTPTransport,
		Policy:      DiscoveryPublic,
	})

	if info.Version != ProtocolVersion {
		t.Errorf("Version = %q, want %q", info.Version, ProtocolVersion)
	}
	if got := info.Endpoints["mcp"]; got != "http://example.test/mcp" {
		t.Errorf("Endpoints[mcp] = %q, want http://example.test/mcp", got)
	}
	if len(info.Transports) != 2 {
		t.Fatalf("len(Transports) = %d, want 2 (http + sse)", len(info.Transports))
	}
	tools, ok := info.Capabilities["tools"].(map[string]any)
	if !ok {
		t.Fatalf("Capabilities[tools] missing or wrong type")
	}
	if tools["count"].(int) != 1 {
		t.Errorf("tools.count = %v, want 1", tools["count"])
	}
	avail, ok := tools["available"].([]string)
	if !ok || len(avail) != 1 || avail[0] != "calc" {
		t.Errorf("tools.available = %v, want [calc]", tools["available"])
	}
}

func TestBuildDiscoveryInfoSchemeFromForwardedProto(t *testing.T) {
	h := newDiscoveryHandler(t)
	r := newDiscoveryRequest("")
	r.Header.Set("X-Forwarded-Proto", "https")

	info := h.BuildDiscoveryInfo(r, DiscoveryConfig{MCPEndpoint: "/mcp", DefaultAddr: ":8080"})

	if got := info.Endpoints["mcp"]; got != "https://example.test/mcp" {
		t.Errorf("Endpoints[mcp] = %q, want https://example.test/mcp (from X-Forwarded-Proto)", got)
	}
}

func TestBuildDiscoveryInfoStdioAddsCapability(t *testing.T) {
	h := newDiscoveryHandler(t)
	info := h.BuildDiscoveryInfo(newDiscoveryRequest(""), DiscoveryConfig{
		MCPEndpoint: "/mcp",
		DefaultAddr: ":8080",
		Transport:   StdioTransport,
	})
	if _, ok := info.Capabilities["stdio"]; !ok {
		t.Error("Capabilities[stdio] missing when Transport == StdioTransport")
	}
}

func TestShouldIncludeToolList(t *testing.T) {
	cases := []struct {
		name   string
		policy DiscoveryPolicy
		authz  string
		want   bool
	}{
		{"public", DiscoveryPublic, "", true},
		{"count", DiscoveryCount, "", false},
		{"none", DiscoveryNone, "", false},
		{"authenticated without auth", DiscoveryAuthenticated, "", false},
		{"authenticated with auth", DiscoveryAuthenticated, "Bearer x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIncludeToolList(tc.policy, newDiscoveryRequest(tc.authz))
			if got != tc.want {
				t.Errorf("shouldIncludeToolList(%v, authz=%q) = %v, want %v",
					tc.policy, tc.authz, got, tc.want)
			}
		})
	}
}

// TestShouldExposeToolInDiscovery exercises the per-tool gate including the
// v0.33 regression: tool names containing "debug"/"admin" are no longer
// silently hidden — that substring sniffing was a leaky guess about user
// intent and is now removed in favor of the IsDiscoverable interface.
func TestShouldExposeToolInDiscovery(t *testing.T) {
	cases := []struct {
		name     string
		toolName string
		toolImpl Tool
		cfg      DiscoveryConfig
		authz    string
		want     bool
	}{
		{
			name:     "plain tool exposed under public policy",
			toolName: "calc",
			toolImpl: &stubTool{name: "calc"},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     true,
		},
		{
			name:     "underscore-prefixed hidden",
			toolName: "_internal",
			toolImpl: &stubTool{name: "_internal"},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     false,
		},
		{
			name:     "internal_-prefixed hidden",
			toolName: "internal_audit",
			toolImpl: &stubTool{name: "internal_audit"},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     false,
		},
		{
			name:     "name containing 'admin' NOT hidden (v0.33 regression)",
			toolName: "tax_admin_lookup",
			toolImpl: &stubTool{name: "tax_admin_lookup"},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     true,
		},
		{
			name:     "name containing 'debug' NOT hidden (v0.33 regression)",
			toolName: "predebug_helper",
			toolImpl: &stubTool{name: "predebug_helper"},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     true,
		},
		{
			name:     "IsDiscoverable=false opts out",
			toolName: "shy",
			toolImpl: &hiddenTool{stubTool{name: "shy"}},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     false,
		},
		{
			name:     "IsDiscoverable=true allows",
			toolName: "loud",
			toolImpl: &visibleTool{stubTool{name: "loud"}},
			cfg:      DiscoveryConfig{Policy: DiscoveryPublic},
			want:     true,
		},
		{
			name:     "authenticated policy hides without Authorization",
			toolName: "calc",
			toolImpl: &stubTool{name: "calc"},
			cfg:      DiscoveryConfig{Policy: DiscoveryAuthenticated},
			want:     false,
		},
		{
			name:     "authenticated policy allows with Authorization",
			toolName: "calc",
			toolImpl: &stubTool{name: "calc"},
			cfg:      DiscoveryConfig{Policy: DiscoveryAuthenticated},
			authz:    "Bearer x",
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDiscoveryHandler(t)
			h.RegisterTool(tc.toolImpl)
			got := h.shouldExposeToolInDiscovery(tc.toolName, newDiscoveryRequest(tc.authz), tc.cfg)
			if got != tc.want {
				t.Errorf("shouldExposeToolInDiscovery(%q) = %v, want %v", tc.toolName, got, tc.want)
			}
		})
	}
}

func TestShouldExposeToolInDiscoveryFilterOverrides(t *testing.T) {
	h := newDiscoveryHandler(t)
	h.RegisterTool(&stubTool{name: "calc"})

	// A custom Filter must short-circuit all other rules — including the
	// underscore prefix and IsDiscoverable opt-outs. This is the documented
	// contract on DiscoveryConfig.Filter.
	called := false
	cfg := DiscoveryConfig{
		Policy: DiscoveryPublic,
		Filter: func(name string, r *http.Request) bool {
			called = true
			return name == "calc"
		},
	}

	if !h.shouldExposeToolInDiscovery("calc", newDiscoveryRequest(""), cfg) {
		t.Error("custom Filter returning true should expose the tool")
	}
	if !called {
		t.Error("custom Filter was not consulted")
	}
	if h.shouldExposeToolInDiscovery("anything_else", newDiscoveryRequest(""), cfg) {
		t.Error("custom Filter returning false should hide the tool")
	}
}

// TestBuildDiscoveryInfoAuthenticatedPolicy verifies that under
// DiscoveryAuthenticated, the tool list is omitted from the discovery
// payload when no Authorization header is present, and included when it is.
// This is the property the cache-poisoning fix in pkg/server/mcp.go protects.
func TestBuildDiscoveryInfoAuthenticatedPolicy(t *testing.T) {
	h := newDiscoveryHandler(t)
	h.RegisterTool(&stubTool{name: "calc"})

	cfg := DiscoveryConfig{
		MCPEndpoint: "/mcp",
		DefaultAddr: ":8080",
		Policy:      DiscoveryAuthenticated,
	}

	anon := h.BuildDiscoveryInfo(newDiscoveryRequest(""), cfg)
	tools := anon.Capabilities["tools"].(map[string]any)
	if _, hasAvail := tools["available"]; hasAvail {
		t.Error("anonymous DiscoveryAuthenticated response should not include tools.available")
	}
	if tools["count"].(int) != 1 {
		t.Errorf("tools.count = %v, want 1 (count exposed even when names are hidden)", tools["count"])
	}

	authed := h.BuildDiscoveryInfo(newDiscoveryRequest("Bearer x"), cfg)
	tools = authed.Capabilities["tools"].(map[string]any)
	avail, ok := tools["available"].([]string)
	if !ok || !slices.Contains(avail, "calc") {
		t.Errorf("authenticated DiscoveryAuthenticated response should include calc in tools.available; got %v", tools["available"])
	}
}
