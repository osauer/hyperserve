package hyperserve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// WebSocketPool manages a pool of WebSocket connections
type WebSocketPool struct {
	// Configuration
	config PoolConfig
	
	// Connection storage
	pools    map[string]*endpointPool // Key: endpoint URL
	poolsMu  sync.RWMutex
	
	// Metrics
	stats    PoolStats
	
	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// PoolConfig configures the WebSocket connection pool
type PoolConfig struct {
	// MaxConnectionsPerEndpoint is the maximum number of connections per endpoint
	MaxConnectionsPerEndpoint int
	
	// MaxIdleConnections is the maximum number of idle connections per endpoint
	MaxIdleConnections int
	
	// IdleTimeout is how long a connection can be idle before being closed
	IdleTimeout time.Duration
	
	// HealthCheckInterval is how often to ping connections to check health
	HealthCheckInterval time.Duration
	
	// ConnectionTimeout is the timeout for establishing new connections
	ConnectionTimeout time.Duration
	
	// EnableCompression enables WebSocket compression
	EnableCompression bool
	
	// OnConnectionCreated is called when a new connection is created
	OnConnectionCreated func(endpoint string, conn *Conn)
	
	// OnConnectionClosed is called when a connection is closed
	OnConnectionClosed func(endpoint string, conn *Conn, reason error)
}

// PoolStats tracks pool statistics
type PoolStats struct {
	TotalConnections   atomic.Int64
	ActiveConnections  atomic.Int64
	IdleConnections    atomic.Int64
	FailedConnections  atomic.Int64
	ConnectionsCreated atomic.Int64
	ConnectionsReused  atomic.Int64
	HealthChecksFailed atomic.Int64
}

// pooledConn wraps a WebSocket connection with pool metadata
type pooledConn struct {
	conn        *Conn
	endpoint    string
	inUse       atomic.Bool
	lastUsed    time.Time
	created     time.Time
	healthCheck time.Time
	mu          sync.Mutex
}

// endpointPool manages connections for a specific endpoint
type endpointPool struct {
	endpoint    string
	connections []*pooledConn
	mu          sync.Mutex
	upgrader    *Upgrader
	config      *PoolConfig
}

// DefaultPoolConfig returns a default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnectionsPerEndpoint: 10,
		MaxIdleConnections:        5,
		IdleTimeout:              30 * time.Second,
		HealthCheckInterval:      10 * time.Second,
		ConnectionTimeout:        10 * time.Second,
		EnableCompression:        false,
	}
}

// NewWebSocketPool creates a new WebSocket connection pool
func NewWebSocketPool(config PoolConfig) *WebSocketPool {
	// Apply defaults
	if config.MaxConnectionsPerEndpoint <= 0 {
		config.MaxConnectionsPerEndpoint = 10
	}
	if config.MaxIdleConnections <= 0 {
		config.MaxIdleConnections = 5
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 30 * time.Second
	}
	if config.HealthCheckInterval <= 0 {
		config.HealthCheckInterval = 10 * time.Second
	}
	if config.ConnectionTimeout <= 0 {
		config.ConnectionTimeout = 10 * time.Second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	pool := &WebSocketPool{
		config: config,
		pools:  make(map[string]*endpointPool),
		ctx:    ctx,
		cancel: cancel,
	}
	
	// Start maintenance goroutine
	pool.wg.Add(1)
	go pool.maintainPools()
	
	return pool
}

// Get retrieves a connection from the pool or creates a new one
func (p *WebSocketPool) Get(ctx context.Context, endpoint string, upgrader *Upgrader, w http.ResponseWriter, r *http.Request) (*Conn, error) {
	p.poolsMu.RLock()
	ep, exists := p.pools[endpoint]
	p.poolsMu.RUnlock()
	
	if !exists {
		// Create new endpoint pool
		p.poolsMu.Lock()
		ep, exists = p.pools[endpoint]
		if !exists {
			ep = &endpointPool{
				endpoint:    endpoint,
				connections: make([]*pooledConn, 0),
				upgrader:    upgrader,
				config:      &p.config,
			}
			p.pools[endpoint] = ep
		}
		p.poolsMu.Unlock()
	}
	
	// Try to get an existing connection
	conn := ep.getIdleConnection()
	if conn != nil {
		p.stats.ConnectionsReused.Add(1)
		p.stats.IdleConnections.Add(-1)
		p.stats.ActiveConnections.Add(1)
		return conn.conn, nil
	}
	
	// Check if we can create a new connection
	if !ep.canCreateConnection() {
		return nil, errors.New("connection pool limit reached")
	}
	
	// Create new connection
	newConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		p.stats.FailedConnections.Add(1)
		return nil, fmt.Errorf("failed to upgrade connection: %w", err)
	}
	
	// Wrap in pooled connection
	pc := &pooledConn{
		conn:        newConn,
		endpoint:    endpoint,
		created:     time.Now(),
		lastUsed:    time.Now(),
		healthCheck: time.Now(),
	}
	pc.inUse.Store(true)
	
	// Add to pool
	ep.addConnection(pc)
	
	// Update stats
	p.stats.TotalConnections.Add(1)
	p.stats.ActiveConnections.Add(1)
	p.stats.ConnectionsCreated.Add(1)
	
	// Callback
	if p.config.OnConnectionCreated != nil {
		p.config.OnConnectionCreated(endpoint, newConn)
	}
	
	return newConn, nil
}

// Put returns a connection to the pool
func (p *WebSocketPool) Put(conn *Conn) error {
	p.poolsMu.RLock()
	defer p.poolsMu.RUnlock()
	
	// Find the connection in all pools
	for _, ep := range p.pools {
		if ep.returnConnection(conn) {
			p.stats.ActiveConnections.Add(-1)
			p.stats.IdleConnections.Add(1)
			return nil
		}
	}
	
	// Connection not found in pool, close it
	return conn.Close()
}

// Close closes a connection and removes it from the pool
func (p *WebSocketPool) Close(conn *Conn, reason error) error {
	p.poolsMu.RLock()
	defer p.poolsMu.RUnlock()
	
	// Find and remove the connection
	for endpoint, ep := range p.pools {
		if ep.removeConnection(conn) {
			p.stats.TotalConnections.Add(-1)
			p.stats.ActiveConnections.Add(-1)
			
			// Callback
			if p.config.OnConnectionClosed != nil {
				p.config.OnConnectionClosed(endpoint, conn, reason)
			}
			
			return conn.Close()
		}
	}
	
	// Connection not in pool, just close it
	return conn.Close()
}

// GetStats returns current pool statistics
func (p *WebSocketPool) GetStats() PoolStats {
	return PoolStats{
		TotalConnections:   atomic.Int64{},
		ActiveConnections:  atomic.Int64{},
		IdleConnections:    atomic.Int64{},
		FailedConnections:  atomic.Int64{},
		ConnectionsCreated: atomic.Int64{},
		ConnectionsReused:  atomic.Int64{},
		HealthChecksFailed: atomic.Int64{},
	}
}

// Shutdown gracefully shuts down the pool
func (p *WebSocketPool) Shutdown(ctx context.Context) error {
	p.cancel()
	
	// Wait for maintenance to stop or context to expire
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		// Maintenance stopped
	case <-ctx.Done():
		return ctx.Err()
	}
	
	// Close all connections
	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()
	
	for _, ep := range p.pools {
		ep.closeAll()
	}
	
	return nil
}

// maintainPools performs periodic maintenance on the connection pools
func (p *WebSocketPool) maintainPools() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performMaintenance()
		}
	}
}

// performMaintenance checks health and removes idle connections
func (p *WebSocketPool) performMaintenance() {
	p.poolsMu.RLock()
	pools := make([]*endpointPool, 0, len(p.pools))
	for _, ep := range p.pools {
		pools = append(pools, ep)
	}
	p.poolsMu.RUnlock()
	
	now := time.Now()
	
	for _, ep := range pools {
		ep.mu.Lock()
		
		// Check each connection
		for i := len(ep.connections) - 1; i >= 0; i-- {
			pc := ep.connections[i]
			
			// Skip connections in use
			if pc.inUse.Load() {
				continue
			}
			
			pc.mu.Lock()
			
			// Remove idle connections
			if now.Sub(pc.lastUsed) > p.config.IdleTimeout {
				pc.conn.Close()
				ep.connections = append(ep.connections[:i], ep.connections[i+1:]...)
				p.stats.TotalConnections.Add(-1)
				p.stats.IdleConnections.Add(-1)
				pc.mu.Unlock()
				continue
			}
			
			// Health check via ping
			if now.Sub(pc.healthCheck) > p.config.HealthCheckInterval {
				pc.healthCheck = now
				pc.mu.Unlock()
				
				// Send ping with timeout
				pingData := []byte(fmt.Sprintf("ping-%d", time.Now().Unix()))
				err := pc.conn.WriteControl(PingMessage, pingData, time.Now().Add(5*time.Second))
				
				if err != nil {
					// Connection unhealthy, remove it
					pc.conn.Close()
					ep.connections = append(ep.connections[:i], ep.connections[i+1:]...)
					p.stats.TotalConnections.Add(-1)
					p.stats.IdleConnections.Add(-1)
					p.stats.HealthChecksFailed.Add(1)
				}
			} else {
				pc.mu.Unlock()
			}
		}
		
		ep.mu.Unlock()
	}
}

// endpointPool methods

func (ep *endpointPool) getIdleConnection() *pooledConn {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	for _, pc := range ep.connections {
		if !pc.inUse.Load() {
			if pc.inUse.CompareAndSwap(false, true) {
				pc.mu.Lock()
				pc.lastUsed = time.Now()
				pc.mu.Unlock()
				return pc
			}
		}
	}
	
	return nil
}

func (ep *endpointPool) canCreateConnection() bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	return len(ep.connections) < ep.config.MaxConnectionsPerEndpoint
}

func (ep *endpointPool) addConnection(pc *pooledConn) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	ep.connections = append(ep.connections, pc)
}

func (ep *endpointPool) returnConnection(conn *Conn) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	for _, pc := range ep.connections {
		if pc.conn == conn {
			pc.inUse.Store(false)
			pc.mu.Lock()
			pc.lastUsed = time.Now()
			pc.mu.Unlock()
			return true
		}
	}
	
	return false
}

func (ep *endpointPool) removeConnection(conn *Conn) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	for i, pc := range ep.connections {
		if pc.conn == conn {
			ep.connections = append(ep.connections[:i], ep.connections[i+1:]...)
			return true
		}
	}
	
	return false
}

func (ep *endpointPool) closeAll() {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	
	for _, pc := range ep.connections {
		pc.conn.Close()
	}
	
	ep.connections = nil
}