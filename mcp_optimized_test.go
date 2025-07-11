package hyperserve

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestMCPToolWithContext tests the context support for tools
func TestMCPToolWithContext(t *testing.T) {
	t.Run("normal execution completes", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			executeFunc: func(params map[string]interface{}) (interface{}, error) {
				time.Sleep(10 * time.Millisecond)
				return "success", nil
			},
		}
		
		wrapped := wrapToolWithContext(tool)
		ctx := context.Background()
		
		result, err := wrapped.ExecuteWithContext(ctx, map[string]interface{}{})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if result != "success" {
			t.Errorf("expected 'success', got %v", result)
		}
	})
	
	t.Run("context cancellation stops execution", func(t *testing.T) {
		tool := &mockTool{
			name: "slow_tool",
			executeFunc: func(params map[string]interface{}) (interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				return "should not reach here", nil
			},
		}
		
		wrapped := wrapToolWithContext(tool)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		
		_, err := wrapped.ExecuteWithContext(ctx, map[string]interface{}{})
		if err == nil {
			t.Error("expected context cancellation error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})
	
	t.Run("tool already implements context", func(t *testing.T) {
		tool := &mockToolWithContext{
			mockTool: mockTool{name: "ctx_tool"},
			executeWithContextFunc: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
				return "context aware", nil
			},
		}
		
		wrapped := wrapToolWithContext(tool)
		
		// Should return the same instance
		if _, ok := wrapped.(*mockToolWithContext); !ok {
			t.Error("expected tool to be returned as-is when it already supports context")
		}
	})
}

// TestResourceCache tests the resource caching functionality
func TestResourceCache(t *testing.T) {
	t.Run("cache hit returns cached value", func(t *testing.T) {
		cache := newResourceCache(10)
		cache.set("key1", "value1", 1*time.Hour)
		
		value, hit := cache.get("key1")
		if !hit {
			t.Error("expected cache hit")
		}
		if value != "value1" {
			t.Errorf("expected 'value1', got %v", value)
		}
	})
	
	t.Run("expired entry returns cache miss", func(t *testing.T) {
		cache := newResourceCache(10)
		cache.set("key1", "value1", 1*time.Millisecond)
		
		time.Sleep(2 * time.Millisecond)
		
		_, hit := cache.get("key1")
		if hit {
			t.Error("expected cache miss for expired entry")
		}
	})
	
	t.Run("LRU eviction when cache is full", func(t *testing.T) {
		cache := newResourceCache(2)
		
		cache.set("key1", "value1", 1*time.Hour)
		time.Sleep(1 * time.Millisecond)
		cache.set("key2", "value2", 1*time.Hour)
		time.Sleep(1 * time.Millisecond)
		cache.set("key3", "value3", 1*time.Hour) // Should evict key1
		
		_, hit := cache.get("key1")
		if hit {
			t.Error("expected key1 to be evicted")
		}
		
		if _, hit := cache.get("key2"); !hit {
			t.Error("expected key2 to still be in cache")
		}
		if _, hit := cache.get("key3"); !hit {
			t.Error("expected key3 to be in cache")
		}
	})
	
	t.Run("clear removes all entries", func(t *testing.T) {
		cache := newResourceCache(10)
		cache.set("key1", "value1", 1*time.Hour)
		cache.set("key2", "value2", 1*time.Hour)
		
		cache.clear()
		
		_, hit := cache.get("key1")
		if hit {
			t.Error("expected cache to be empty after clear")
		}
	})
}

// TestMCPMetrics tests the metrics collection
func TestMCPMetrics(t *testing.T) {
	t.Run("records request metrics", func(t *testing.T) {
		metrics := newMCPMetrics()
		
		// Record some requests
		metrics.recordRequest("method1", 10*time.Millisecond, nil)
		metrics.recordRequest("method1", 20*time.Millisecond, nil)
		metrics.recordRequest("method2", 5*time.Millisecond, errors.New("error"))
		
		summary := metrics.GetMetricsSummary()
		
		if metrics.totalRequests != 3 {
			t.Errorf("expected 3 total requests, got %d", metrics.totalRequests)
		}
		if metrics.totalErrors != 1 {
			t.Errorf("expected 1 total error, got %d", metrics.totalErrors)
		}
		
		methodStats := summary["methods"].(map[string]interface{})
		method1Stats := methodStats["method1"].(map[string]interface{})
		if method1Stats["count"].(int64) != 2 {
			t.Errorf("expected 2 calls to method1, got %v", method1Stats["count"])
		}
	})
	
	t.Run("records tool execution metrics", func(t *testing.T) {
		metrics := newMCPMetrics()
		
		metrics.recordToolExecution("tool1", 15*time.Millisecond, nil)
		metrics.recordToolExecution("tool1", 25*time.Millisecond, errors.New("failed"))
		
		summary := metrics.GetMetricsSummary()
		toolStats := summary["tools"].(map[string]interface{})
		tool1Stats := toolStats["tool1"].(map[string]interface{})
		
		if tool1Stats["count"].(int64) != 2 {
			t.Errorf("expected 2 executions, got %v", tool1Stats["count"])
		}
		if tool1Stats["errors"].(int64) != 1 {
			t.Errorf("expected 1 error, got %v", tool1Stats["errors"])
		}
		if tool1Stats["error_rate"].(float64) != 0.5 {
			t.Errorf("expected error rate 0.5, got %v", tool1Stats["error_rate"])
		}
	})
	
	t.Run("records cache metrics", func(t *testing.T) {
		metrics := newMCPMetrics()
		
		metrics.recordResourceRead("resource1", 10*time.Millisecond, nil, true) // cache hit
		metrics.recordResourceRead("resource1", 20*time.Millisecond, nil, false) // cache miss
		metrics.recordResourceRead("resource2", 15*time.Millisecond, nil, true) // cache hit
		
		summary := metrics.GetMetricsSummary()
		cacheStats := summary["cache"].(map[string]interface{})
		
		if cacheStats["hits"].(int64) != 2 {
			t.Errorf("expected 2 cache hits, got %v", cacheStats["hits"])
		}
		if cacheStats["misses"].(int64) != 1 {
			t.Errorf("expected 1 cache miss, got %v", cacheStats["misses"])
		}
		if cacheStats["hit_rate"].(float64) < 0.66 || cacheStats["hit_rate"].(float64) > 0.67 {
			t.Errorf("expected hit rate ~0.667, got %v", cacheStats["hit_rate"])
		}
	})
}

// TestMCPResourceWithCache tests the cached resource wrapper
func TestMCPResourceWithCache(t *testing.T) {
	t.Run("caches resource reads", func(t *testing.T) {
		callCount := 0
		resource := &mockResource{
			uri: "test://resource",
			readFunc: func() (interface{}, error) {
				callCount++
				return map[string]interface{}{"data": "value"}, nil
			},
		}
		
		cached := EnableResourceCaching(resource, 1*time.Hour, 10)
		
		// First read - should call the resource
		result1, err := cached.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		// Second read - should use cache
		result2, err := cached.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		if callCount != 1 {
			t.Errorf("expected resource to be called only once, got %d", callCount)
		}
		
		// Results should be the same
		map1 := result1.(map[string]interface{})
		map2 := result2.(map[string]interface{})
		if map1["data"] != map2["data"] {
			t.Error("cached results don't match")
		}
	})
	
	t.Run("handles string data", func(t *testing.T) {
		resource := &mockResource{
			uri: "test://text",
			readFunc: func() (interface{}, error) {
				return "plain text", nil
			},
		}
		
		cached := EnableResourceCaching(resource, 1*time.Hour, 10)
		
		result, err := cached.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		if result != "plain text" {
			t.Errorf("expected 'plain text', got %v", result)
		}
	})
}

// TestConcurrentAccess tests concurrent access to cache and metrics
func TestConcurrentAccess(t *testing.T) {
	t.Run("cache concurrent access", func(t *testing.T) {
		cache := newResourceCache(100)
		var wg sync.WaitGroup
		
		// Concurrent writes
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					key := string(rune('a' + n))
					cache.set(key, n*100+j, 1*time.Hour)
				}
			}(i)
		}
		
		// Concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					key := string(rune('a' + n))
					cache.get(key)
				}
			}(i)
		}
		
		wg.Wait()
		// Test passes if no race conditions or panics occur
	})
	
	t.Run("metrics concurrent access", func(t *testing.T) {
		metrics := newMCPMetrics()
		var wg sync.WaitGroup
		
		// Concurrent metric recording
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				method := string(rune('a' + n))
				for j := 0; j < 100; j++ {
					metrics.recordRequest(method, time.Duration(j)*time.Millisecond, nil)
					metrics.recordToolExecution("tool"+method, time.Duration(j)*time.Millisecond, nil)
				}
			}(i)
		}
		
		// Concurrent reads
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					metrics.GetMetricsSummary()
					time.Sleep(1 * time.Millisecond)
				}
			}()
		}
		
		wg.Wait()
		// Test passes if no race conditions or panics occur
	})
}

// Mock implementations for testing

type mockTool struct {
	name        string
	description string
	schema      map[string]interface{}
	executeFunc func(params map[string]interface{}) (interface{}, error)
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Schema() map[string]interface{} {
	if m.schema != nil {
		return m.schema
	}
	return map[string]interface{}{}
}
func (m *mockTool) Execute(params map[string]interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(params)
	}
	return nil, nil
}

type mockToolWithContext struct {
	mockTool
	executeWithContextFunc func(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

func (m *mockToolWithContext) ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	if m.executeWithContextFunc != nil {
		return m.executeWithContextFunc(ctx, params)
	}
	return m.Execute(params)
}

type mockResource struct {
	uri         string
	name        string
	description string
	mimeType    string
	readFunc    func() (interface{}, error)
	listFunc    func() ([]string, error)
}

func (m *mockResource) URI() string         { return m.uri }
func (m *mockResource) Name() string        { return m.name }
func (m *mockResource) Description() string { return m.description }
func (m *mockResource) MimeType() string {
	if m.mimeType != "" {
		return m.mimeType
	}
	return "application/json"
}
func (m *mockResource) Read() (interface{}, error) {
	if m.readFunc != nil {
		return m.readFunc()
	}
	return map[string]interface{}{}, nil
}
func (m *mockResource) List() ([]string, error) {
	if m.listFunc != nil {
		return m.listFunc()
	}
	return []string{m.uri}, nil
}