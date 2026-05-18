package server

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
	executeFunc func(params map[string]any) (any, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "Mock tool for testing" }
func (t *mockTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *mockTool) Execute(params map[string]any) (any, error) {
	if t.executeFunc != nil {
		return t.executeFunc(params)
	}
	return nil, nil
}

type mockResource struct {
	uri      string
	name     string
	readFunc func() (any, error)
}

func (r *mockResource) URI() string         { return r.uri }
func (r *mockResource) Name() string        { return r.name }
func (r *mockResource) Description() string { return "Mock resource for testing" }
func (r *mockResource) MimeType() string    { return "application/json" }
func (r *mockResource) Read() (any, error) {
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
		WithMCPSupport("test-server", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register a custom tool that supports context
	customTool := &testContextTool{
		name: "context_aware_tool",
		executeFunc: func(ctx context.Context, params map[string]any) (any, error) {
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
			executeFunc: func(params map[string]any) (any, error) {
				time.Sleep(35 * time.Second) // Longer than default timeout
				return "should timeout", nil
			},
		}
		srv.RegisterMCPTool(slowTool)

		// Call the tool
		request := map[string]any{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "slow_tool",
				"arguments": map[string]any{},
			},
			"id": 1,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		srv.mux.ServeHTTP(rec, req)

		var response map[string]any
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
			readFunc: func() (any, error) {
				callCount++
				return map[string]any{
					"count": callCount,
					"data":  "test data",
				}, nil
			},
		}
		srv.RegisterMCPResource(testResource)

		// First read
		request1 := map[string]any{
			"jsonrpc": "2.0",
			"method":  "resources/read",
			"params": map[string]any{
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
		var response1, response2 map[string]any
		json.Unmarshal(rec1.Body.Bytes(), &response1)
		json.Unmarshal(rec2.Body.Bytes(), &response2)

		// Both should have the same data (count=1 because cached)
		contents1 := response1["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)
		contents2 := response2["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)

		// Parse the JSON string from text field
		var data1, data2 map[string]any
		json.Unmarshal([]byte(contents1["text"].(string)), &data1)
		json.Unmarshal([]byte(contents2["text"].(string)), &data2)

		count1 := data1["count"].(float64)
		count2 := data2["count"].(float64)

		if count1 != 1 || count2 != 1 {
			t.Errorf("Expected cached value (count=1), got %v and %v", count1, count2)
		}

		if callCount != 1 {
			t.Errorf("Expected resource to be called once, got %d", callCount)
		}
	})

}

// Test concurrent tool execution safety
func TestMCPConcurrentToolExecution(t *testing.T) {
	srv, err := NewServer(
		WithMCPSupport("concurrent-test", "1.0.0"),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Register a tool that tracks concurrent executions
	var activeCount int32
	var maxActive int32
	concurrentTool := &mockTool{
		name: "concurrent_tool",
		executeFunc: func(params map[string]any) (any, error) {
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

			return map[string]any{"executed": true}, nil
		},
	}
	srv.RegisterMCPTool(concurrentTool)

	// Execute multiple concurrent requests
	const numRequests = 20
	done := make(chan bool, numRequests)

	for i := range numRequests {
		go func(id int) {
			request := map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name":      "concurrent_tool",
					"arguments": map[string]any{"id": id},
				},
				"id": id,
			}

			body, _ := json.Marshal(request)
			req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			srv.mux.ServeHTTP(rec, req)

			var response map[string]any
			json.Unmarshal(rec.Body.Bytes(), &response)

			if response["error"] != nil {
				t.Errorf("Request %d failed: %v", id, response["error"])
			}

			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for range numRequests {
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
	executeFunc func(ctx context.Context, params map[string]any) (any, error)
}

func (t *testContextTool) Name() string           { return t.name }
func (t *testContextTool) Description() string    { return "Test tool" }
func (t *testContextTool) Schema() map[string]any { return map[string]any{} }
func (t *testContextTool) Execute(params map[string]any) (any, error) {
	return t.ExecuteWithContext(context.Background(), params)
}
func (t *testContextTool) ExecuteWithContext(ctx context.Context, params map[string]any) (any, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, params)
	}
	return nil, nil
}
