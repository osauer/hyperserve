package hyperserve

import (
	"context"
	"sync"
	"time"
)

// MCPToolWithContext is an enhanced interface that supports context for cancellation and timeouts
type MCPToolWithContext interface {
	MCPTool
	ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// MCPResourceWithCache wraps a resource with caching capabilities
type MCPResourceWithCache struct {
	resource     MCPResource
	cache        *resourceCache
	cacheEnabled bool
	cacheTTL     time.Duration
}

// resourceCache provides thread-safe caching for MCP resources
type resourceCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	maxSize int
}

type cacheEntry struct {
	value     interface{}
	timestamp time.Time
	ttl       time.Duration
}

// newResourceCache creates a new resource cache
func newResourceCache(maxSize int) *resourceCache {
	return &resourceCache{
		data:    make(map[string]*cacheEntry),
		maxSize: maxSize,
	}
}

// get retrieves a value from the cache if it exists and hasn't expired
func (c *resourceCache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.data[key]
	if !exists {
		return nil, false
	}
	
	// Check if entry has expired
	if time.Since(entry.timestamp) > entry.ttl {
		return nil, false
	}
	
	return entry.value, true
}

// set stores a value in the cache with the given TTL
func (c *resourceCache) set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Implement simple LRU eviction if cache is full
	if len(c.data) >= c.maxSize && c.maxSize > 0 {
		// Find oldest entry
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.data {
			if oldestKey == "" || v.timestamp.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.timestamp
			}
		}
		delete(c.data, oldestKey)
	}
	
	c.data[key] = &cacheEntry{
		value:     value,
		timestamp: time.Now(),
		ttl:       ttl,
	}
}

// clear removes all entries from the cache
func (c *resourceCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]*cacheEntry)
}

// MCPMetrics tracks performance metrics for MCP operations
type MCPMetrics struct {
	mu               sync.RWMutex
	totalRequests    int64
	totalErrors      int64
	methodDurations  map[string]*durationStats
	toolExecutions   map[string]*executionStats
	resourceReads    map[string]*executionStats
	cacheHits        int64
	cacheMisses      int64
}

type durationStats struct {
	count    int64
	totalMs  int64
	minMs    int64
	maxMs    int64
}

type executionStats struct {
	count    int64
	errors   int64
	totalMs  int64
}

// newMCPMetrics creates a new metrics tracker
func newMCPMetrics() *MCPMetrics {
	return &MCPMetrics{
		methodDurations: make(map[string]*durationStats),
		toolExecutions:  make(map[string]*executionStats),
		resourceReads:   make(map[string]*executionStats),
	}
}

// recordRequest records a request metric
func (m *MCPMetrics) recordRequest(method string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.totalRequests++
	if err != nil {
		m.totalErrors++
	}
	
	durationMs := duration.Milliseconds()
	
	stats, exists := m.methodDurations[method]
	if !exists {
		stats = &durationStats{
			minMs: durationMs,
			maxMs: durationMs,
		}
		m.methodDurations[method] = stats
	}
	
	stats.count++
	stats.totalMs += durationMs
	if durationMs < stats.minMs {
		stats.minMs = durationMs
	}
	if durationMs > stats.maxMs {
		stats.maxMs = durationMs
	}
}

// recordToolExecution records a tool execution metric
func (m *MCPMetrics) recordToolExecution(toolName string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	stats, exists := m.toolExecutions[toolName]
	if !exists {
		stats = &executionStats{}
		m.toolExecutions[toolName] = stats
	}
	
	stats.count++
	stats.totalMs += duration.Milliseconds()
	if err != nil {
		stats.errors++
	}
}

// recordResourceRead records a resource read metric
func (m *MCPMetrics) recordResourceRead(uri string, duration time.Duration, err error, cacheHit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if cacheHit {
		m.cacheHits++
		return
	}
	
	m.cacheMisses++
	
	stats, exists := m.resourceReads[uri]
	if !exists {
		stats = &executionStats{}
		m.resourceReads[uri] = stats
	}
	
	stats.count++
	stats.totalMs += duration.Milliseconds()
	if err != nil {
		stats.errors++
	}
}

// GetMetricsSummary returns a summary of collected metrics
func (m *MCPMetrics) GetMetricsSummary() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Calculate method stats
	methodStats := make(map[string]interface{})
	for method, stats := range m.methodDurations {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		methodStats[method] = map[string]interface{}{
			"count":   stats.count,
			"avg_ms":  avgMs,
			"min_ms":  stats.minMs,
			"max_ms":  stats.maxMs,
		}
	}
	
	// Calculate tool stats
	toolStats := make(map[string]interface{})
	for tool, stats := range m.toolExecutions {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		toolStats[tool] = map[string]interface{}{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}
	
	// Calculate resource stats
	resourceStats := make(map[string]interface{})
	for uri, stats := range m.resourceReads {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		resourceStats[uri] = map[string]interface{}{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}
	
	// Calculate cache hit rate
	totalCacheRequests := m.cacheHits + m.cacheMisses
	cacheHitRate := float64(0)
	if totalCacheRequests > 0 {
		cacheHitRate = float64(m.cacheHits) / float64(totalCacheRequests)
	}
	
	return map[string]interface{}{
		"total_requests": m.totalRequests,
		"total_errors":   m.totalErrors,
		"error_rate":     float64(m.totalErrors) / float64(m.totalRequests),
		"methods":        methodStats,
		"tools":          toolStats,
		"resources":      resourceStats,
		"cache": map[string]interface{}{
			"hits":     m.cacheHits,
			"misses":   m.cacheMisses,
			"hit_rate": cacheHitRate,
		},
	}
}

// wrapToolWithContext wraps a regular MCPTool to support context
func wrapToolWithContext(tool MCPTool) MCPToolWithContext {
	// If it already supports context, return as-is
	if ctxTool, ok := tool.(MCPToolWithContext); ok {
		return ctxTool
	}
	
	// Otherwise, wrap it
	return &contextToolWrapper{tool: tool}
}

type contextToolWrapper struct {
	tool MCPTool
}

func (w *contextToolWrapper) Name() string {
	return w.tool.Name()
}

func (w *contextToolWrapper) Description() string {
	return w.tool.Description()
}

func (w *contextToolWrapper) Schema() map[string]interface{} {
	return w.tool.Schema()
}

func (w *contextToolWrapper) Execute(params map[string]interface{}) (interface{}, error) {
	return w.tool.Execute(params)
}

func (w *contextToolWrapper) ExecuteWithContext(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// Create a channel to receive the result
	type result struct {
		value interface{}
		err   error
	}
	
	resultChan := make(chan result, 1)
	
	// Run the tool in a goroutine
	go func() {
		value, err := w.tool.Execute(params)
		resultChan <- result{value: value, err: err}
	}()
	
	// Wait for either the result or context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.value, res.err
	}
}

// EnableResourceCaching enables caching for a resource
func EnableResourceCaching(resource MCPResource, ttl time.Duration, cacheSize int) *MCPResourceWithCache {
	return &MCPResourceWithCache{
		resource:     resource,
		cache:        newResourceCache(cacheSize),
		cacheEnabled: true,
		cacheTTL:     ttl,
	}
}

// Read implements cached read for resources
func (r *MCPResourceWithCache) Read() (interface{}, error) {
	// Check cache first
	if r.cacheEnabled {
		if value, hit := r.cache.get(r.resource.URI()); hit {
			return value, nil
		}
	}
	
	// Read from resource
	value, err := r.resource.Read()
	if err != nil {
		return nil, err
	}
	
	// Store in cache
	if r.cacheEnabled && err == nil {
		r.cache.set(r.resource.URI(), value, r.cacheTTL)
	}
	
	return value, nil
}

// URI returns the resource URI
func (r *MCPResourceWithCache) URI() string {
	return r.resource.URI()
}

// Name returns the resource name
func (r *MCPResourceWithCache) Name() string {
	return r.resource.Name()
}

// Description returns the resource description
func (r *MCPResourceWithCache) Description() string {
	return r.resource.Description()
}

// MimeType returns the resource MIME type
func (r *MCPResourceWithCache) MimeType() string {
	return r.resource.MimeType()
}

// List returns the resource list
func (r *MCPResourceWithCache) List() ([]string, error) {
	return r.resource.List()
}

// ClearCache clears the resource cache
func (r *MCPResourceWithCache) ClearCache() {
	if r.cache != nil {
		r.cache.clear()
	}
}