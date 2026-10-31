package hyperserve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osauer/hyperserve/v2/mcp"
)

func TestWithMCPEndpointRejectsMalformedPaths(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{
		"",
		"mcp",
		"GET /mcp",
		"/",
		"/mcp/",
		"/mcp//v",
		"/mcp/./v",
		"/mcp/../v",
		"/mcp/%2e/v",
		"/mcp/%61",
		"/mcp?debug=true",
		"/mcp#fragment",
		"/mcp/{client}",
		"/mcp/%zz",
		mcpDiscoveryEndpoint,
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			if _, err := New(WithMCPEndpoint(endpoint)); err == nil {
				t.Fatalf("New(WithMCPEndpoint(%q)) error = nil", endpoint)
			}
		})
	}
}

func TestReservedMCPDiscoveryEndpointReturnsErrorWithoutPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("New panicked for reserved MCP endpoint: %v", recovered)
		}
	}()
	if _, err := New(
		WithMCPSupport("test", "1.0.0"),
		WithMCPEndpoint(mcpDiscoveryEndpoint),
	); err == nil {
		t.Fatal("New accepted the reserved MCP discovery endpoint")
	}
}

func TestNewValidatesResolvedMCPEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("options snapshot", func(t *testing.T) {
		t.Parallel()
		options := DefaultOptions()
		options.MCPEnabled = true
		options.MCPEndpoint = "mcp"
		if _, err := New(WithOptions(options)); err == nil {
			t.Fatal("New accepted malformed MCP endpoint from Options")
		}
	})

	t.Run("transport config", func(t *testing.T) {
		t.Parallel()
		malformedEndpoint := func(options *mcp.TransportOptions) {
			options.Transport = mcp.HTTPTransport
			options.Endpoint = "mcp"
		}
		if _, err := New(WithMCPSupport("test", "1.0.0", malformedEndpoint)); err == nil {
			t.Fatal("New accepted malformed MCP endpoint from transport config")
		}
	})

	t.Run("disabled zero options", func(t *testing.T) {
		t.Parallel()
		if _, err := New(WithOptions(Options{})); err != nil {
			t.Fatalf("New rejected unused empty MCP endpoint: %v", err)
		}
	})
}

func TestWithMCPEndpointAcceptsLiteralPaths(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"/mcp", "/mcp-v2"} {
		if _, err := New(WithMCPEndpoint(endpoint)); err != nil {
			t.Errorf("New(WithMCPEndpoint(%q)) error = %v", endpoint, err)
		}
	}
}

func TestCustomMCPEndpointKeepsDiscoveryReachable(t *testing.T) {
	t.Parallel()

	app, err := New(
		WithMCPSupport("test", "1.0.0"),
		WithMCPEndpoint("/custom-mcp"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, endpoint := range []string{"/custom-mcp/discover", "/.well-known/mcp.json"} {
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", endpoint, recorder.Code, http.StatusOK)
		}
	}
}
