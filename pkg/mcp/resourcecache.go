package mcp

import (
	"sync"
	"time"
)

// resourceCache provides thread-safe caching for MCP resource reads.
type resourceCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	maxSize int
}

type cacheEntry struct {
	value     any
	timestamp time.Time
	ttl       time.Duration
}

func newResourceCache(maxSize int) *resourceCache {
	return &resourceCache{
		data:    make(map[string]*cacheEntry),
		maxSize: maxSize,
	}
}

func (c *resourceCache) get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return nil, false
	}
	if time.Since(entry.timestamp) > entry.ttl {
		return nil, false
	}
	return entry.value, true
}

func (c *resourceCache) set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction: oldest entry is dropped when full.
	if len(c.data) >= c.maxSize && c.maxSize > 0 {
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
