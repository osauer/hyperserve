package websocket

import (
	"context"
	"testing"
	"time"
)

func TestWebSocketPoolStats(t *testing.T) {
	pool := NewWebSocketPool(PoolConfig{
		MaxConnectionsPerEndpoint: 5,
		MaxIdleConnections:        2,
		IdleTimeout:               2 * time.Second,
		HealthCheckInterval:       time.Second,
		ConnectionTimeout:         time.Second,
	})
	t.Cleanup(func() { pool.Shutdown(context.Background()) })

	stats := pool.GetStats()
	if stats.TotalConnections != 0 {
		t.Fatalf("expected no connections initially, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 0 {
		t.Fatalf("expected no active connections initially, got %d", stats.ActiveConnections)
	}
}
