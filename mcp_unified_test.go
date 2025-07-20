package hyperserve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// flusherRecorder is a ResponseRecorder that implements http.Flusher
type flusherRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flusherRecorder) Flush() {
	// No-op for testing, but satisfies the Flusher interface
}

func TestMCPUnifiedHandlerAutoDiscovery(t *testing.T) {
	// Create server with auto-discovery enabled
	srv, err := NewServer(
		WithAddr(":0"), // Use dynamic port
		WithMCPSupport("TestServer", "1.0.0", MCPAutoDiscovery()),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Verify auto-discovery is enabled
	if !srv.Options.MCPAutoDiscovery {
		t.Error("MCPAutoDiscovery should be enabled")
	}

	// Verify unified handler was created
	if srv.mcpUnifiedHandler == nil {
		t.Error("Unified handler should be created when auto-discovery is enabled")
	}

	// Test the unified endpoint with HTTP POST
	t.Run("HTTP_POST_initialize", func(t *testing.T) {
		initRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo":      map[string]interface{}{"name": "test-client", "version": "1.0.0"},
			},
			"id": 1,
		}

		body, _ := json.Marshal(initRequest)
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		
		recorder := httptest.NewRecorder()
		srv.mcpUnifiedHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Check response has the expected fields
		if response["jsonrpc"] != "2.0" {
			t.Error("Expected jsonrpc 2.0")
		}
		if response["id"] != float64(1) {
			t.Error("Expected id 1")
		}

		// Check that result contains server info and session info
		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected result object")
		}

		serverInfo, ok := result["serverInfo"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected serverInfo")
		}
		if serverInfo["name"] != "TestServer" {
			t.Errorf("Expected server name 'TestServer', got %v", serverInfo["name"])
		}

		sessionInfo, ok := result["sessionInfo"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected sessionInfo for unified handler")
		}
		if sessionInfo["transport"] != "unified" {
			t.Error("Expected transport 'unified'")
		}
	})

	// Test SSE endpoint headers and initial response
	t.Run("SSE_connection", func(t *testing.T) {
		// Create a custom ResponseRecorder that implements Flusher
		recorder := &flusherRecorder{
			ResponseRecorder: httptest.NewRecorder(),
		}
		
		req := httptest.NewRequest("GET", "/mcp", nil)
		req.Header.Set("Accept", "text/event-stream")
		
		// Start SSE in a goroutine since it will block
		done := make(chan struct{})
		go func() {
			defer close(done)
			srv.mcpUnifiedHandler.ServeHTTP(recorder, req)
		}()

		// Give SSE connection a moment to set headers
		time.Sleep(100 * time.Millisecond)

		// Check that the headers were set correctly
		if recorder.Header().Get("Content-Type") != "text/event-stream" {
			t.Errorf("Expected Content-Type: text/event-stream, got: %s", recorder.Header().Get("Content-Type"))
		}
		if recorder.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("Expected Cache-Control: no-cache, got: %s", recorder.Header().Get("Cache-Control"))
		}
		if recorder.Header().Get("Connection") != "keep-alive" {
			t.Errorf("Expected Connection: keep-alive, got: %s", recorder.Header().Get("Connection"))
		}
	})

	// Test discovery endpoints
	t.Run("Discovery_wellknown_mcp", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/mcp.json", nil)
		recorder := httptest.NewRecorder()
		
		srv.mcpUnifiedHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		var config MCPDiscoveryConfig
		if err := json.Unmarshal(recorder.Body.Bytes(), &config); err != nil {
			t.Fatalf("Failed to unmarshal discovery config: %v", err)
		}

		if config.Version != "1.0" {
			t.Error("Expected version 1.0")
		}

		if len(config.Servers) == 0 {
			t.Error("Expected at least one server in discovery config")
		}

		// Check server entry
		for _, server := range config.Servers {
			if server.Name != "TestServer" {
				t.Errorf("Expected server name 'TestServer', got %s", server.Name)
			}
			if server.Transport != "sse" {
				t.Errorf("Expected transport 'sse', got %s", server.Transport)
			}
		}
	})

	t.Run("Discovery_mcp_servers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.well-known/mcp-servers", nil)
		recorder := httptest.NewRecorder()
		
		srv.mcpUnifiedHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal servers response: %v", err)
		}

		servers, ok := response["servers"].([]interface{})
		if !ok || len(servers) == 0 {
			t.Error("Expected servers array")
		}
	})

	t.Run("Discovery_discover_endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mcp/discover", nil)
		recorder := httptest.NewRecorder()
		
		srv.mcpUnifiedHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		var info MCPDiscoveryInfo
		if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil {
			t.Fatalf("Failed to unmarshal discovery info: %v", err)
		}

		if info.Server.Name != "TestServer" {
			t.Error("Expected TestServer in discovery info")
		}

		if !info.Transport.HTTP.Supported || !info.Transport.SSE.Supported {
			t.Error("Both HTTP and SSE transports should be supported")
		}
	})

	// Clean up
	if srv.mcpUnifiedHandler != nil {
		srv.mcpUnifiedHandler.Close()
	}
}

func TestMCPLegacyBackwardCompatibility(t *testing.T) {
	// Create server without auto-discovery (legacy mode)
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("LegacyServer", "1.0.0"), // No MCPAutoDiscovery()
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Verify auto-discovery is NOT enabled
	if srv.Options.MCPAutoDiscovery {
		t.Error("MCPAutoDiscovery should NOT be enabled for legacy servers")
	}

	// Verify legacy handler was created
	if srv.mcpHandler == nil {
		t.Error("Legacy handler should be created")
	}

	// Verify unified handler was NOT created
	if srv.mcpUnifiedHandler != nil {
		t.Error("Unified handler should NOT be created for legacy servers")
	}

	// Test that legacy endpoint still works
	t.Run("Legacy_HTTP_POST", func(t *testing.T) {
		initRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo":      map[string]interface{}{"name": "test-client", "version": "1.0.0"},
			},
			"id": 1,
		}

		body, _ := json.Marshal(initRequest)
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		
		recorder := httptest.NewRecorder()
		srv.mcpHandler.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Check response
		if response["jsonrpc"] != "2.0" {
			t.Error("Expected jsonrpc 2.0")
		}

		// Check that legacy handler doesn't include sessionInfo
		result, ok := response["result"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected result object")
		}

		if _, hasSessionInfo := result["sessionInfo"]; hasSessionInfo {
			t.Error("Legacy handler should not include sessionInfo")
		}
	})
}

func TestMCPDiscoveryHTMLResponse(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("HTMLTestServer", "1.0.0", MCPAutoDiscovery()),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test HTML response for discovery endpoint
	req := httptest.NewRequest("GET", "/mcp/discover", nil)
	req.Header.Set("Accept", "text/html")
	
	recorder := httptest.NewRecorder()
	srv.mcpUnifiedHandler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}

	contentType := recorder.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected HTML content type, got %s", contentType)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "HTMLTestServer") {
		t.Error("HTML response should contain server name")
	}
	if !strings.Contains(body, "Claude Code Ready!") {
		t.Error("HTML response should indicate Claude Code readiness")
	}

	// Clean up
	srv.mcpUnifiedHandler.Close()
}

func TestSessionStateManagement(t *testing.T) {
	// Test session state transitions
	session := NewMCPSession("test-session", TransportHTTP)

	// Initial state should be New
	if session.GetState() != SessionStateNew {
		t.Error("Initial state should be New")
	}

	// Test valid transitions
	err := session.SetState(SessionStateInitialized)
	if err != nil {
		t.Errorf("Should be able to transition from New to Initialized: %v", err)
	}

	err = session.SetState(SessionStateReady)
	if err != nil {
		t.Errorf("Should be able to transition from Initialized to Ready: %v", err)
	}

	err = session.SetState(SessionStateActive)
	if err != nil {
		t.Errorf("Should be able to transition from Ready to Active: %v", err)
	}

	err = session.SetState(SessionStateClosed)
	if err != nil {
		t.Errorf("Should be able to transition from Active to Closed: %v", err)
	}

	// Test invalid transitions
	err = session.SetState(SessionStateNew)
	if err == nil {
		t.Error("Should not be able to transition from Closed to any other state")
	}

	// Clean up
	session.Close()
}