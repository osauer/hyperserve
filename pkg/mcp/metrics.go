package mcp

import (
	"sync"
	"time"
)

// Metrics tracks performance metrics for MCP operations.
type Metrics struct {
	mu              sync.RWMutex
	totalRequests   int64
	totalErrors     int64
	methodDurations map[string]*durationStats
	toolExecutions  map[string]*executionStats
	resourceReads   map[string]*executionStats
	cacheHits       int64
	cacheMisses     int64
}

type durationStats struct {
	count   int64
	totalMs int64
	minMs   int64
	maxMs   int64
}

type executionStats struct {
	count   int64
	errors  int64
	totalMs int64
}

func newMetrics() *Metrics {
	return &Metrics{
		methodDurations: make(map[string]*durationStats),
		toolExecutions:  make(map[string]*executionStats),
		resourceReads:   make(map[string]*executionStats),
	}
}

func (m *Metrics) recordRequest(method string, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	if err != nil {
		m.totalErrors++
	}

	durationMs := duration.Milliseconds()
	stats, exists := m.methodDurations[method]
	if !exists {
		stats = &durationStats{minMs: durationMs, maxMs: durationMs}
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

func (m *Metrics) recordToolExecution(toolName string, duration time.Duration, err error) {
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

func (m *Metrics) recordResourceRead(uri string, duration time.Duration, err error, cacheHit bool) {
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

// GetMetricsSummary returns a summary of collected metrics.
func (m *Metrics) GetMetricsSummary() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	methodStats := make(map[string]any)
	for method, stats := range m.methodDurations {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		methodStats[method] = map[string]any{
			"count":  stats.count,
			"avg_ms": avgMs,
			"min_ms": stats.minMs,
			"max_ms": stats.maxMs,
		}
	}

	toolStats := make(map[string]any)
	for tool, stats := range m.toolExecutions {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		toolStats[tool] = map[string]any{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}

	resourceStats := make(map[string]any)
	for uri, stats := range m.resourceReads {
		avgMs := float64(0)
		if stats.count > 0 {
			avgMs = float64(stats.totalMs) / float64(stats.count)
		}
		resourceStats[uri] = map[string]any{
			"count":      stats.count,
			"errors":     stats.errors,
			"avg_ms":     avgMs,
			"error_rate": float64(stats.errors) / float64(stats.count),
		}
	}

	totalCacheRequests := m.cacheHits + m.cacheMisses
	cacheHitRate := float64(0)
	if totalCacheRequests > 0 {
		cacheHitRate = float64(m.cacheHits) / float64(totalCacheRequests)
	}

	return map[string]any{
		"total_requests": m.totalRequests,
		"total_errors":   m.totalErrors,
		"error_rate":     float64(m.totalErrors) / float64(m.totalRequests),
		"methods":        methodStats,
		"tools":          toolStats,
		"resources":      resourceStats,
		"cache": map[string]any{
			"hits":     m.cacheHits,
			"misses":   m.cacheMisses,
			"hit_rate": cacheHitRate,
		},
	}
}
