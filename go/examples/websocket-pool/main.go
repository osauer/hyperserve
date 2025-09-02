package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/osauer/hyperserve"
)

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// ConnectionManager manages WebSocket connections with pooling
type ConnectionManager struct {
	pool      *hyperserve.WebSocketPool
	upgrader  *hyperserve.Upgrader
	broadcast chan Message
	clients   sync.Map // map[*hyperserve.Conn]bool
}

func NewConnectionManager() *ConnectionManager {
	// Configure the pool
	poolConfig := hyperserve.PoolConfig{
		MaxConnectionsPerEndpoint: 100,
		MaxIdleConnections:        20,
		IdleTimeout:              30 * time.Second,
		HealthCheckInterval:      10 * time.Second,
		ConnectionTimeout:        5 * time.Second,
		EnableCompression:        true,
		OnConnectionCreated: func(endpoint string, conn *hyperserve.Conn) {
			log.Printf("New connection created for endpoint: %s from %s", endpoint, conn.RemoteAddr())
		},
		OnConnectionClosed: func(endpoint string, conn *hyperserve.Conn, reason error) {
			log.Printf("Connection closed for endpoint: %s, reason: %v", endpoint, reason)
		},
	}

	return &ConnectionManager{
		pool: hyperserve.NewWebSocketPool(poolConfig),
		upgrader: &hyperserve.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// In production, implement proper origin checking
				return true
			},
			MaxMessageSize:  1024 * 1024, // 1MB
			WriteBufferSize: 1024,
			ReadBufferSize:  1024,
		},
		broadcast: make(chan Message, 100),
	}
}

func (cm *ConnectionManager) Start() {
	// Start broadcast handler
	go cm.handleBroadcast()
	
	// Periodically send stats
	go cm.sendStats()
}

func (cm *ConnectionManager) handleBroadcast() {
	for msg := range cm.broadcast {
		cm.clients.Range(func(key, value interface{}) bool {
			conn := key.(*hyperserve.Conn)
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Error broadcasting to client: %v", err)
				cm.clients.Delete(conn)
				cm.pool.Close(conn, err)
			}
			return true
		})
	}
}

func (cm *ConnectionManager) sendStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		stats := cm.pool.GetStats()
		msg := Message{
			Type:      "stats",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"total_connections":    stats.TotalConnections.Load(),
				"active_connections":   stats.ActiveConnections.Load(),
				"idle_connections":     stats.IdleConnections.Load(),
				"connections_created":  stats.ConnectionsCreated.Load(),
				"connections_reused":   stats.ConnectionsReused.Load(),
				"health_checks_failed": stats.HealthChecksFailed.Load(),
			},
		}
		
		select {
		case cm.broadcast <- msg:
		default:
			// Channel full, skip
		}
	}
}

func (cm *ConnectionManager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get connection from pool or create new one
	endpoint := r.URL.Path
	conn, err := cm.pool.Get(r.Context(), endpoint, cm.upgrader, w, r)
	if err != nil {
		log.Printf("Failed to get WebSocket connection: %v", err)
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	
	// Register client
	cm.clients.Store(conn, true)
	
	// Send welcome message
	welcomeMsg := Message{
		Type:      "welcome",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"message": "Connected to WebSocket pool demo",
			"endpoint": endpoint,
		},
	}
	
	if err := conn.WriteJSON(welcomeMsg); err != nil {
		log.Printf("Failed to send welcome message: %v", err)
		cm.clients.Delete(conn)
		cm.pool.Close(conn, err)
		return
	}
	
	// Handle incoming messages
	go cm.handleConnection(conn)
}

func (cm *ConnectionManager) handleConnection(conn *hyperserve.Conn) {
	defer func() {
		cm.clients.Delete(conn)
		// Return connection to pool instead of closing
		if err := cm.pool.Put(conn); err != nil {
			log.Printf("Failed to return connection to pool: %v", err)
		}
	}()
	
	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	
	// Set pong handler to update read deadline
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			if hyperserve.IsUnexpectedCloseError(err, hyperserve.CloseGoingAway, hyperserve.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		
		// Update read deadline on successful read
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		
		// Handle different message types
		switch msg.Type {
		case "ping":
			// Respond with pong
			response := Message{
				Type:      "pong",
				Timestamp: time.Now(),
				Data:      msg.Data,
			}
			if err := conn.WriteJSON(response); err != nil {
				log.Printf("Failed to send pong: %v", err)
				break
			}
			
		case "broadcast":
			// Broadcast to all clients
			msg.Timestamp = time.Now()
			select {
			case cm.broadcast <- msg:
			default:
				log.Printf("Broadcast channel full")
			}
			
		case "echo":
			// Echo back to sender
			msg.Type = "echo_response"
			msg.Timestamp = time.Now()
			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("Failed to echo: %v", err)
				break
			}
			
		default:
			log.Printf("Unknown message type: %s", msg.Type)
		}
	}
}

func main() {
	// Create server on port 8085
	srv, err := hyperserve.NewServer()
	if err != nil {
		log.Fatal(err)
	}
	srv.Options.Addr = ":8085"
	
	// Create connection manager
	manager := NewConnectionManager()
	manager.Start()
	
	// Serve static files
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(http.Dir("./static")).ServeHTTP(w, r)
	})
	
	// WebSocket endpoints with pooling
	srv.HandleFunc("/ws/chat", manager.HandleWebSocket)
	srv.HandleFunc("/ws/notifications", manager.HandleWebSocket)
	srv.HandleFunc("/ws/updates", manager.HandleWebSocket)
	
	// API endpoint to get pool stats
	srv.HandleFunc("/api/pool-stats", func(w http.ResponseWriter, r *http.Request) {
		stats := manager.pool.GetStats()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_connections":    stats.TotalConnections.Load(),
			"active_connections":   stats.ActiveConnections.Load(),
			"idle_connections":     stats.IdleConnections.Load(),
			"failed_connections":   stats.FailedConnections.Load(),
			"connections_created":  stats.ConnectionsCreated.Load(),
			"connections_reused":   stats.ConnectionsReused.Load(),
			"health_checks_failed": stats.HealthChecksFailed.Load(),
		})
	})
	
	// Start server in background
	go func() {
		log.Printf("WebSocket Pool Demo server starting on http://localhost:8085")
		if err := srv.Run(); err != nil {
			log.Fatal(err)
		}
	}()
	
	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	
	// Graceful shutdown
	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := manager.pool.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down pool: %v", err)
	}
	
	log.Println("Server stopped")
}