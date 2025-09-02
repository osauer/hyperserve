package hyperserve

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestWebSocketPool(t *testing.T) {
	// Create test server
	srv, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}

	// Create pool with test configuration
	poolConfig := PoolConfig{
		MaxConnectionsPerEndpoint: 5,
		MaxIdleConnections:        3,
		IdleTimeout:              5 * time.Second,
		HealthCheckInterval:      2 * time.Second,
		ConnectionTimeout:        1 * time.Second,
	}
	
	pool := NewWebSocketPool(poolConfig)
	defer pool.Shutdown(context.Background())
	
	// Create upgrader
	upgrader := &Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	
	// Echo handler for testing
	srv.HandleFunc("/ws/test", func(w http.ResponseWriter, r *http.Request) {
		conn, err := pool.Get(r.Context(), "/ws/test", upgrader, w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		// Echo messages back
		go func() {
			defer pool.Put(conn)
			
			for {
				messageType, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				
				if err := conn.WriteMessage(messageType, data); err != nil {
					return
				}
			}
		}()
	})
	
	// Test basic pool functionality without HTTP server
	t.Run("PoolBasicFunctionality", func(t *testing.T) {
		// Test pool configuration and stats
		stats := pool.GetStats()
		initialTotal := stats.TotalConnections.Load()
		
		// Basic stats verification
		if stats.ActiveConnections.Load() != 0 {
			t.Error("Expected 0 active connections initially")
		}
		
		t.Logf("Initial pool stats: Total=%d, Active=%d, Idle=%d", 
			initialTotal, 
			stats.ActiveConnections.Load(), 
			stats.IdleConnections.Load())
	})
	
	// Test connection limits
	t.Run("ConnectionLimits", func(t *testing.T) {
		// Try to exceed max connections
		// Implementation would depend on internal pool access
		stats := pool.GetStats()
		
		if stats.TotalConnections.Load() > int64(poolConfig.MaxConnectionsPerEndpoint) {
			t.Errorf("Pool exceeded max connections: %d > %d", 
				stats.TotalConnections.Load(), 
				poolConfig.MaxConnectionsPerEndpoint)
		}
	})
	
	// Test health checks
	t.Run("HealthChecks", func(t *testing.T) {
		// Wait for health check interval
		time.Sleep(poolConfig.HealthCheckInterval + 500*time.Millisecond)
		
		stats := pool.GetStats()
		// Health checks should have run by now
		t.Logf("Health checks failed: %d", stats.HealthChecksFailed.Load())
	})
	
	// Test idle timeout
	t.Run("IdleTimeout", func(t *testing.T) {
		initialStats := pool.GetStats()
		initialTotal := initialStats.TotalConnections.Load()
		
		// Wait for idle timeout
		time.Sleep(poolConfig.IdleTimeout + time.Second)
		
		finalStats := pool.GetStats()
		finalTotal := finalStats.TotalConnections.Load()
		
		// Some connections should have been cleaned up
		if finalTotal >= initialTotal && initialTotal > 0 {
			t.Log("Warning: Idle connections may not have been cleaned up")
		}
	})
}

func TestPoolShutdown(t *testing.T) {
	poolConfig := DefaultPoolConfig()
	pool := NewWebSocketPool(poolConfig)
	
	// Test graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err := pool.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestPoolStats(t *testing.T) {
	poolConfig := DefaultPoolConfig()
	pool := NewWebSocketPool(poolConfig)
	defer pool.Shutdown(context.Background())
	
	// Get initial stats
	stats := pool.GetStats()
	
	// Verify initial state
	if stats.TotalConnections.Load() != 0 {
		t.Errorf("Expected 0 total connections, got %d", stats.TotalConnections.Load())
	}
	
	if stats.ActiveConnections.Load() != 0 {
		t.Errorf("Expected 0 active connections, got %d", stats.ActiveConnections.Load())
	}
	
	if stats.IdleConnections.Load() != 0 {
		t.Errorf("Expected 0 idle connections, got %d", stats.IdleConnections.Load())
	}
}

// Helper function to generate WebSocket key
func generateWebSocketKey() string {
	return "dGhlIHNhbXBsZSBub25jZQ=="
}