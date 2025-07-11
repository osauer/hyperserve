package hyperserve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Mock types for testing
type mockTool struct {
	name        string
	executeFunc func(params map[string]interface{}) (interface{}, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "Mock tool for testing" }
func (t *mockTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{},
	}
}
func (t *mockTool) Execute(params map[string]interface{}) (interface{}, error) {
	if t.executeFunc != nil {
		return t.executeFunc(params)
	}
	return nil, nil
}

type mockResource struct {
	uri      string
	name     string
	readFunc func() (interface{}, error)
}

func (r *mockResource) URI() string        { return r.uri }
func (r *mockResource) Name() string       { return r.name }
func (r *mockResource) Description() string { return "Mock resource for testing" }
func (r *mockResource) MimeType() string   { return "application/json" }
func (r *mockResource) Read() (interface{}, error) {
	if r.readFunc != nil {
		return r.readFunc()
	}
	return nil, nil
}
func (r *mockResource) List() ([]string, error) { return nil, nil }

// TestMCPOptimizationsIntegration tests the optimizations in an integrated environment
func TestMCPOptimizationsIntegration(t *testing.T) {
	// Create server with MCP support
	srv, err := NewServer(
		WithMCPSupport(),
		WithMCPServerInfo("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register a custom tool that supports context
	customTool := &testContextTool{
		name: "context_aware_tool",
		executeFunc: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// Simulate work that can be cancelled
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return "completed", nil
			}
		},
	}
	srv.RegisterMCPTool(customTool)

	// Test context cancellation via timeout
	t.Run("tool_execution_with_timeout", func(t *testing.T) {
		t.Skip("Skipping timeout test - 30 second timeout is too long for integration tests")
		// Create a slow tool that takes longer than 30 seconds
		slowTool := &mockTool{
			name: "slow_tool",
			executeFunc: func(params map[string]interface{}) (interface{}, error) {
				time.Sleep(35 * time.Second) // Longer than default timeout
				return "should timeout", nil
			},
		}
		srv.RegisterMCPTool(slowTool)

		// Call the tool
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "slow_tool",
				"arguments": map[string]interface{}{},
			},
			"id": 1,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec, req)

		var response map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &response)

		// Should have error due to timeout
		if response["error"] == nil {
			t.Error("Expected timeout error")
		}
	})

	// Test resource caching
	t.Run("resource_caching", func(t *testing.T) {
		callCount := 0
		testResource := &mockResource{
			uri:  "test://cacheable",
			name: "Cacheable Resource",
			readFunc: func() (interface{}, error) {
				callCount++
				return map[string]interface{}{
					"count": callCount,
					"data":  "test data",
				}, nil
			},
		}
		srv.RegisterMCPResource(testResource)

		// First read
		request1 := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "resources/read",
			"params": map[string]interface{}{
				"uri": "test://cacheable",
			},
			"id": 1,
		}

		body1, _ := json.Marshal(request1)
		req1 := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body1))
		req1.Header.Set("Content-Type", "application/json")
		rec1 := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec1, req1)

		// Second read (should be cached)
		req2 := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body1))
		req2.Header.Set("Content-Type", "application/json")
		rec2 := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec2, req2)

		// Parse responses
		var response1, response2 map[string]interface{}
		json.Unmarshal(rec1.Body.Bytes(), &response1)
		json.Unmarshal(rec2.Body.Bytes(), &response2)

		// Both should have the same data (count=1 because cached)
		contents1 := response1["result"].(map[string]interface{})["contents"].([]interface{})[0].(map[string]interface{})
		contents2 := response2["result"].(map[string]interface{})["contents"].([]interface{})[0].(map[string]interface{})

		text1 := contents1["text"].(map[string]interface{})["count"].(float64)
		text2 := contents2["text"].(map[string]interface{})["count"].(float64)

		if text1 != 1 || text2 != 1 {
			t.Errorf("Expected cached value (count=1), got %v and %v", text1, text2)
		}

		if callCount != 1 {
			t.Errorf("Expected resource to be called once, got %d", callCount)
		}
	})

	// Test metrics collection
	t.Run("metrics_collection", func(t *testing.T) {
		// Make several requests to collect metrics
		pingRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "ping",
			"id":      1,
		}

		for i := 0; i < 5; i++ {
			body, _ := json.Marshal(pingRequest)
			req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.mux.ServeHTTP(rec, req)
		}

		// Get metrics
		handler := srv.mcpHandler
		if handler == nil {
			t.Skip("MCP handler not available")
		}

		metrics := handler.GetMetrics()
		if metrics == nil {
			t.Fatal("Expected metrics to be available")
		}

		totalRequests := metrics["total_requests"].(int64)
		if totalRequests < 5 {
			t.Errorf("Expected at least 5 requests, got %d", totalRequests)
		}

		// Check method metrics
		methods := metrics["methods"].(map[string]interface{})
		if pingStats, exists := methods["ping"]; exists {
			stats := pingStats.(map[string]interface{})
			if stats["count"].(int64) < 5 {
				t.Errorf("Expected at least 5 ping requests, got %v", stats["count"])
			}
		} else {
			t.Error("Expected ping method in metrics")
		}
	})
}

// Test concurrent tool execution safety
func TestMCPConcurrentToolExecution(t *testing.T) {
	srv, err := NewServer(
		WithMCPSupport(),
		WithMCPServerInfo("concurrent-test", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register a tool that tracks concurrent executions
	var activeCount int32
	var maxActive int32
	concurrentTool := &mockTool{
		name: "concurrent_tool",
		executeFunc: func(params map[string]interface{}) (interface{}, error) {
			// Increment active count
			current := atomic.AddInt32(&activeCount, 1)
			
			// Track max concurrent
			for {
				max := atomic.LoadInt32(&maxActive)
				if current <= max || atomic.CompareAndSwapInt32(&maxActive, max, current) {
					break
				}
			}
			
			// Simulate work
			time.Sleep(10 * time.Millisecond)
			
			// Decrement active count
			atomic.AddInt32(&activeCount, -1)
			
			return map[string]interface{}{"executed": true}, nil
		},
	}
	srv.RegisterMCPTool(concurrentTool)

	// Execute multiple concurrent requests
	const numRequests = 20
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]interface{}{
					"name":      "concurrent_tool",
					"arguments": map[string]interface{}{"id": id},
				},
				"id": id,
			}

			body, _ := json.Marshal(request)
			req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.mux.ServeHTTP(rec, req)

			var response map[string]interface{}
			json.Unmarshal(rec.Body.Bytes(), &response)

			if response["error"] != nil {
				t.Errorf("Request %d failed: %v", id, response["error"])
			}

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}

	// Check that we had concurrent executions
	maxConcurrent := atomic.LoadInt32(&maxActive)
	if maxConcurrent <= 1 {
		t.Errorf("Expected concurrent executions, but max was %d", maxConcurrent)
	}
	t.Logf("Max concurrent executions: %d", maxConcurrent)
}

// Test helper - context-aware tool
type testContextTool struct {
	name        string
	executeFunc func(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

func (t *testContextTool) Name() string                                        { return t.name }
func (t *testContextTool) Description() string                                 { return "Test tool" }
func (t *testContextTool) Schema() map[string]interface{}                      { return map[string]interface{}{} }
func (t *testContextTool) Execute(params map[string]interface{}) (interface{}, error) {
	return t.ExecuteWithContext(context.Background(), params)
}
func (t *testContextTool) ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, params)
	}
	return nil, nil
}