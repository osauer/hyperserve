package hyperserve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMCPHandler_ConcurrentRequests tests that the MCP handler can handle multiple concurrent requests safely
func TestMCPHandler_ConcurrentRequests(t *testing.T) {
	// Create temporary directory for file tools
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("hyperserve_concurrency_test_%d_%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	for i := 0; i < 10; i++ {
		testFile := filepath.Join(tempDir, fmt.Sprintf("test_%d.txt", i))
		content := fmt.Sprintf("Test content for file %d", i)
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %d: %v", i, err)
		}
	}

	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("concurrency-test-server", "1.0.0"),
		WithMCPBuiltinTools(true),      // Enable built-in tools for tests
		WithMCPBuiltinResources(true),  // Enable built-in resources for tests
		WithMCPFileToolRoot(tempDir),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test different types of concurrent requests
	testCases := []struct {
		name     string
		requests []map[string]interface{}
	}{
		{
			name: "ConcurrentPing",
			requests: []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"method":  "ping",
					"id":      1,
				},
				{
					"jsonrpc": "2.0",
					"method":  "ping",
					"id":      2,
				},
				{
					"jsonrpc": "2.0",
					"method":  "ping",
					"id":      3,
				},
			},
		},
		{
			name: "ConcurrentToolsList",
			requests: []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      1,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      2,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      3,
				},
			},
		},
		{
			name: "ConcurrentCalculator",
			requests: []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "calculator",
						"arguments": map[string]interface{}{
							"operation": "add",
							"a":         10.0,
							"b":         20.0,
						},
					},
					"id": 1,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "calculator",
						"arguments": map[string]interface{}{
							"operation": "multiply",
							"a":         5.0,
							"b":         6.0,
						},
					},
					"id": 2,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "calculator",
						"arguments": map[string]interface{}{
							"operation": "divide",
							"a":         100.0,
							"b":         4.0,
						},
					},
					"id": 3,
				},
			},
		},
		{
			name: "ConcurrentFileRead",
			requests: []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "read_file",
						"arguments": map[string]interface{}{
							"path": "test_0.txt",
						},
					},
					"id": 1,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "read_file",
						"arguments": map[string]interface{}{
							"path": "test_1.txt",
						},
					},
					"id": 2,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "read_file",
						"arguments": map[string]interface{}{
							"path": "test_2.txt",
						},
					},
					"id": 3,
				},
			},
		},
		{
			name: "MixedConcurrentRequests",
			requests: []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"method":  "ping",
					"id":      1,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      2,
				},
				{
					"jsonrpc": "2.0",
					"method":  "resources/list",
					"id":      3,
				},
				{
					"jsonrpc": "2.0",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "calculator",
						"arguments": map[string]interface{}{
							"operation": "add",
							"a":         1.0,
							"b":         2.0,
						},
					},
					"id": 4,
				},
				{
					"jsonrpc": "2.0",
					"method":  "resources/read",
					"params": map[string]interface{}{
						"uri": "system://runtime/info",
					},
					"id": 5,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			var errors []error
			var mu sync.Mutex
			
			// Track successful responses
			var successCount int64

			for i, request := range tc.requests {
				wg.Add(1)
				go func(i int, req map[string]interface{}) {
					defer wg.Done()

					requestData, err := json.Marshal(req)
					if err != nil {
						mu.Lock()
						errors = append(errors, fmt.Errorf("request %d marshal error: %v", i, err))
						mu.Unlock()
						return
					}

					httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
					httpReq.Header.Set("Content-Type", "application/json")

					w := httptest.NewRecorder()
					srv.mcpHandler.ServeHTTP(w, httpReq)

					if w.Code != http.StatusOK {
						mu.Lock()
						errors = append(errors, fmt.Errorf("request %d returned status %d", i, w.Code))
						mu.Unlock()
						return
					}

					var response JSONRPCResponse
					if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
						mu.Lock()
						errors = append(errors, fmt.Errorf("request %d response unmarshal error: %v", i, err))
						mu.Unlock()
						return
					}

					if response.Error != nil {
						mu.Lock()
						errors = append(errors, fmt.Errorf("request %d returned JSON-RPC error: %v", i, response.Error))
						mu.Unlock()
						return
					}

					atomic.AddInt64(&successCount, 1)
				}(i, request)
			}

			wg.Wait()

			// Check for errors
			if len(errors) > 0 {
				for _, err := range errors {
					t.Errorf("Concurrent request error: %v", err)
				}
			}

			// Verify all requests completed successfully
			expectedCount := int64(len(tc.requests))
			if atomic.LoadInt64(&successCount) != expectedCount {
				t.Errorf("Expected %d successful responses, got %d", expectedCount, atomic.LoadInt64(&successCount))
			}
		})
	}
}

// TestMCPHandler_HighConcurrency tests the handler under high concurrent load
func TestMCPHandler_HighConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrency test in short mode")
	}

	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("test-server", "1.0.0"),
		WithMCPBuiltinTools(true),      // Enable built-in tools for tests
		WithMCPBuiltinResources(true),  // Enable built-in resources for tests
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	const numGoroutines = 100
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex
	var successCount int64

	// Create the request template
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      1,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
				httpReq.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				srv.mcpHandler.ServeHTTP(w, httpReq)

				if w.Code != http.StatusOK {
					mu.Lock()
					errors = append(errors, fmt.Errorf("goroutine %d request %d returned status %d", goroutineID, j, w.Code))
					mu.Unlock()
					continue
				}

				var response JSONRPCResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("goroutine %d request %d unmarshal error: %v", goroutineID, j, err))
					mu.Unlock()
					continue
				}

				if response.Error != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("goroutine %d request %d JSON-RPC error: %v", goroutineID, j, response.Error))
					mu.Unlock()
					continue
				}

				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Report results
	totalRequests := int64(numGoroutines * requestsPerGoroutine)
	successfulRequests := atomic.LoadInt64(&successCount)
	
	t.Logf("High concurrency test completed:")
	t.Logf("  - Duration: %v", duration)
	t.Logf("  - Total requests: %d", totalRequests)
	t.Logf("  - Successful requests: %d", successfulRequests)
	t.Logf("  - Success rate: %.2f%%", float64(successfulRequests)/float64(totalRequests)*100)
	t.Logf("  - Requests per second: %.2f", float64(totalRequests)/duration.Seconds())

	// Check for errors (allow up to 1% failure rate for high load scenarios)
	maxErrors := totalRequests / 100 // 1% tolerance
	if int64(len(errors)) > maxErrors {
		t.Errorf("Too many errors (%d > %d): first few errors:", len(errors), maxErrors)
		for i, err := range errors {
			if i >= 5 { // Only show first 5 errors
				break
			}
			t.Errorf("  - %v", err)
		}
	}

	// Verify success rate is reasonable (at least 95%)
	if successfulRequests < totalRequests*95/100 {
		t.Errorf("Success rate too low: %d/%d (%.2f%%)", successfulRequests, totalRequests, 
			float64(successfulRequests)/float64(totalRequests)*100)
	}
}

// TestMCPResources_ThreadSafety tests that resource access is thread-safe
func TestMCPResources_ThreadSafety(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("thread-safety-test", "1.0.0"),
		WithMCPBuiltinTools(true),      // Enable built-in tools for tests
		WithMCPBuiltinResources(true),  // Enable built-in resources for tests
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Simulate some server activity to populate metrics
	srv.totalRequests.Store(100)
	srv.totalResponseTime.Store(1000000000) // 1 second in nanoseconds

	resources := []string{
		"config://server/options",
		"metrics://server/stats", 
		"system://runtime/info",
		"logs://server/recent",
	}

	const numGoroutines = 50
	const accessesPerGoroutine = 20

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex
	var successCount int64

	for _, resourceURI := range resources {
		t.Run("Resource_"+resourceURI, func(t *testing.T) {
			errors = errors[:0] // Reset errors for each resource
			atomic.StoreInt64(&successCount, 0)

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func(goroutineID int, uri string) {
					defer wg.Done()

					for j := 0; j < accessesPerGoroutine; j++ {
						request := map[string]interface{}{
							"jsonrpc": "2.0",
							"method":  "resources/read",
							"params": map[string]interface{}{
								"uri": uri,
							},
							"id": goroutineID*accessesPerGoroutine + j,
						}

						requestData, err := json.Marshal(request)
						if err != nil {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d marshal error: %v", goroutineID, j, err))
							mu.Unlock()
							continue
						}

						httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
						httpReq.Header.Set("Content-Type", "application/json")

						w := httptest.NewRecorder()
						srv.mcpHandler.ServeHTTP(w, httpReq)

						if w.Code != http.StatusOK {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d returned status %d", goroutineID, j, w.Code))
							mu.Unlock()
							continue
						}

						var response JSONRPCResponse
						if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d unmarshal error: %v", goroutineID, j, err))
							mu.Unlock()
							continue
						}

						if response.Error != nil {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d JSON-RPC error: %v", goroutineID, j, response.Error))
							mu.Unlock()
							continue
						}

						// Validate that the response contains expected structure
						result, ok := response.Result.(map[string]interface{})
						if !ok {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d: result is not a map", goroutineID, j))
							mu.Unlock()
							continue
						}

						contents, ok := result["contents"].([]interface{})
						if !ok {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d: contents is not an array", goroutineID, j))
							mu.Unlock()
							continue
						}

						if len(contents) == 0 {
							mu.Lock()
							errors = append(errors, fmt.Errorf("goroutine %d access %d: contents array is empty", goroutineID, j))
							mu.Unlock()
							continue
						}

						atomic.AddInt64(&successCount, 1)
					}
				}(i, resourceURI)
			}

			wg.Wait()

			// Check results for this resource
			totalAccesses := int64(numGoroutines * accessesPerGoroutine)
			successful := atomic.LoadInt64(&successCount)

			if len(errors) > 0 {
				t.Errorf("Errors occurred during concurrent access to %s:", resourceURI)
				for i, err := range errors {
					if i >= 3 { // Limit to first 3 errors
						t.Errorf("  ... and %d more errors", len(errors)-3)
						break
					}
					t.Errorf("  - %v", err)
				}
			}

			if successful != totalAccesses {
				t.Errorf("Resource %s: expected %d successful accesses, got %d", resourceURI, totalAccesses, successful)
			}
		})
	}
}

// TestMCPConcurrency_DataRace uses the race detector to find data race conditions
func TestMCPConcurrency_DataRace(t *testing.T) {
	// This test is specifically designed to be run with -race flag
	// go test -race -run TestMCPConcurrency_DataRace

	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("test-server", "1.0.0"),
		WithMCPBuiltinTools(true),      // Enable built-in tools for tests
		WithMCPBuiltinResources(true),  // Enable built-in resources for tests
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test concurrent access to server state that might cause data races
	var wg sync.WaitGroup
	const numGoroutines = 20

	// Concurrent ping requests to test JSON-RPC handler state
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "ping",
				"id":      id,
			}

			requestData, _ := json.Marshal(request)
			httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
			httpReq.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			srv.mcpHandler.ServeHTTP(w, httpReq)
		}(i)
	}

	// Concurrent metrics access to test server statistics updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			request := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "resources/read",
				"params": map[string]interface{}{
					"uri": "metrics://server/stats",
				},
				"id": id + numGoroutines,
			}

			requestData, _ := json.Marshal(request)
			httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
			httpReq.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			srv.mcpHandler.ServeHTTP(w, httpReq)
		}(i)
	}

	// Concurrent server statistics updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// Simulate request processing that updates server stats
			srv.totalRequests.Add(1)
			srv.totalResponseTime.Add(int64(time.Millisecond))
		}(i)
	}

	wg.Wait()

	// If we reach here without the race detector triggering, the test passes
	t.Log("No data races detected in concurrent MCP operations")
}

// TestMCPConcurrency_MemoryUsage tests that concurrent requests don't cause memory leaks
func TestMCPConcurrency_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	srv, err := NewServer(
		WithAddr(":0"),
		WithMCPSupport("test-server", "1.0.0"),
		WithMCPBuiltinTools(true),      // Enable built-in tools for tests
		WithMCPBuiltinResources(true),  // Enable built-in resources for tests
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Force garbage collection and get initial memory stats
	runtime.GC()
	runtime.GC() // Call twice to ensure cleanup
	
	var initialStats runtime.MemStats
	runtime.ReadMemStats(&initialStats)

	// Perform many concurrent requests
	const numIterations = 100
	const concurrentRequests = 50

	for iteration := 0; iteration < numIterations; iteration++ {
		var wg sync.WaitGroup

		for i := 0; i < concurrentRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				request := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      1,
				}

				requestData, _ := json.Marshal(request)
				httpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(requestData))
				httpReq.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				srv.mcpHandler.ServeHTTP(w, httpReq)
			}()
		}

		wg.Wait()

		// Periodically force garbage collection
		if iteration%10 == 0 {
			runtime.GC()
		}
	}

	// Force final garbage collection and get final memory stats
	runtime.GC()
	runtime.GC()
	
	var finalStats runtime.MemStats
	runtime.ReadMemStats(&finalStats)

	// Check memory usage growth
	initialHeap := initialStats.HeapAlloc
	finalHeap := finalStats.HeapAlloc
	growthBytes := int64(finalHeap) - int64(initialHeap)
	
	t.Logf("Memory usage:")
	t.Logf("  - Initial heap: %d bytes", initialHeap)
	t.Logf("  - Final heap: %d bytes", finalHeap)
	t.Logf("  - Growth: %d bytes", growthBytes)
	t.Logf("  - Total requests processed: %d", numIterations*concurrentRequests)

	// Allow some reasonable memory growth (e.g., 10MB for processing thousands of requests)
	const maxGrowthBytes = 10 * 1024 * 1024 // 10MB
	if growthBytes > maxGrowthBytes {
		t.Errorf("Memory usage grew by %d bytes, which exceeds the maximum allowed growth of %d bytes", 
			growthBytes, maxGrowthBytes)
	}
}