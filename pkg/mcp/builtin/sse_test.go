package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/pkg/server"
)

func TestMCPSSEEndpoint(t *testing.T) {
	// Create a server with MCP enabled
	srv, err := server.NewServer(
		server.WithMCPSupport("test-server", "1.0.0"),
		//lint:ignore SA1019 This regression intentionally exercises legacy compatibility.
		server.WithMCPLegacyRoutedSSE(true),
		server.WithMCPBuiltinTools(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping: unable to bind listener: %v", err)
		return
	}

	httpSrv := &http.Server{Handler: srv.Handler()}
	done := make(chan struct{})
	go func() {
		_ = httpSrv.Serve(listener)
		close(done)
	}()
	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		<-done
	}()

	t.Run("SSE Connection", func(t *testing.T) {
		// Debug: Check if MCP is enabled
		if !srv.MCPEnabled() {
			t.Fatal("MCP is not enabled on the server")
		}

		// Debug: Check MCP endpoint
		t.Logf("MCP endpoint: %s", srv.Options().MCPEndpoint)
		t.Logf("MCP handler: %v", srv.MCPHandler())

		// First test base MCP endpoint
		baseResp, err := http.Get(baseURL + "/mcp")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Base MCP endpoint status: %d", baseResp.StatusCode)
		baseResp.Body.Close()

		// Connect to SSE endpoint
		req, err := http.NewRequest("GET", baseURL+"/mcp", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Debug: Print response status
		t.Logf("Response status: %d", resp.StatusCode)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response body: %s", body)
		}

		// Check headers
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Expected Content-Type text/event-stream, got %s", ct)
		}

		// Read first event (connection event)
		reader := bufio.NewReader(resp.Body)

		// Read until we get an event
		var eventData string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}

			if after, ok := strings.CutPrefix(line, "data: "); ok {
				eventData = after
				eventData = strings.TrimSpace(eventData)
				break
			}
		}

		// Parse connection event
		var connEvent map[string]any
		if err := json.Unmarshal([]byte(eventData), &connEvent); err != nil {
			t.Fatalf("Failed to parse connection event: %v", err)
		}

		if connEvent["type"] != "connection" {
			t.Errorf("Expected connection event, got %v", connEvent)
		}

		if clientID, ok := connEvent["clientId"].(string); !ok || clientID == "" {
			t.Error("Connection event missing clientId")
		}
	})

	t.Run("HTTP Request with SSE Client ID", func(t *testing.T) {
		// First connect to get a client ID
		req, err := http.NewRequest("GET", baseURL+"/mcp", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Read connection event to get the (clientID, bindingToken) pair.
		// Both are required on subsequent POSTs; without the binding the
		// server returns 403.
		reader := bufio.NewReader(resp.Body)
		var clientID, bindingToken string

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}

			if after, ok := strings.CutPrefix(line, "data: "); ok {
				eventData := strings.TrimSpace(after)
				var connEvent map[string]any
				if err := json.Unmarshal([]byte(eventData), &connEvent); err == nil {
					if id, ok := connEvent["clientId"].(string); ok {
						clientID = id
					}
					if tok, ok := connEvent["bindingToken"].(string); ok {
						bindingToken = tok
					}
					if clientID != "" && bindingToken != "" {
						break
					}
				}
			}
		}

		// Routed POST with both required headers — expect 202.
		reqBody := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"ping","id":1}`)
		req2, err := http.NewRequest("POST", baseURL+"/mcp", reqBody)
		if err != nil {
			t.Fatal(err)
		}
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-SSE-Client-ID", clientID)
		req2.Header.Set("X-SSE-Binding", bindingToken)

		resp2, err := http.DefaultClient.Do(req2)
		if err != nil {
			t.Fatal(err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(resp2.Body)
			t.Errorf("Expected status 202, got %d: %s", resp2.StatusCode, body)
		}

		// Same POST without the binding header must be rejected (403)
		// — the regression guard for the binding requirement.
		reqBodyBad := bytes.NewBufferString(`{"jsonrpc":"2.0","method":"ping","id":2}`)
		req3, err := http.NewRequest("POST", baseURL+"/mcp", reqBodyBad)
		if err != nil {
			t.Fatal(err)
		}
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("X-SSE-Client-ID", clientID)
		// intentionally no X-SSE-Binding
		resp3, err := http.DefaultClient.Do(req3)
		if err != nil {
			t.Fatal(err)
		}
		defer resp3.Body.Close()
		if resp3.StatusCode != http.StatusForbidden {
			t.Errorf("Expected status 403 without X-SSE-Binding, got %d", resp3.StatusCode)
		}
	})
}
