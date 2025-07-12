package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/osauer/hyperserve"
)

// Message represents a simple message
type Message struct {
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// SimpleChat maintains connected clients and broadcasts messages
type SimpleChat struct {
	clients map[*hyperserve.Conn]string
	mutex   sync.RWMutex
}

func NewSimpleChat() *SimpleChat {
	return &SimpleChat{
		clients: make(map[*hyperserve.Conn]string),
	}
}

func (c *SimpleChat) AddClient(conn *hyperserve.Conn, username string) {
	c.mutex.Lock()
	c.clients[conn] = username
	c.mutex.Unlock()
	log.Printf("Client %s connected. Total clients: %d", username, len(c.clients))
}

func (c *SimpleChat) RemoveClient(conn *hyperserve.Conn) {
	c.mutex.Lock()
	username := c.clients[conn]
	delete(c.clients, conn)
	c.mutex.Unlock()
	log.Printf("Client %s disconnected. Total clients: %d", username, len(c.clients))
}

func (c *SimpleChat) Broadcast(message Message) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	data, _ := json.Marshal(message)
	for conn := range c.clients {
		if err := conn.WriteMessage(hyperserve.TextMessage, data); err != nil {
			log.Printf("Error sending message to client: %v", err)
		}
	}
}

func main() {
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
	)
	if err != nil {
		log.Fatal(err)
	}

	chat := NewSimpleChat()
	
	upgrader := hyperserve.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for demo
		},
	}

	// WebSocket chat handler
	srv.HandleFunc("/ws/chat", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		username := r.URL.Query().Get("username")
		if username == "" {
			username = "Anonymous"
		}

		chat.AddClient(conn, username)
		defer chat.RemoveClient(conn)

		// Message loop
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				break
			}

			// Create and broadcast message
			message := Message{
				Username:  username,
				Message:   string(data),
				Timestamp: time.Now(),
			}
			
			chat.Broadcast(message)
		}
	})

	// Get active users endpoint
	srv.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		chat.mutex.RLock()
		usernames := make([]string, 0, len(chat.clients))
		for _, username := range chat.clients {
			usernames = append(usernames, username)
		}
		chat.mutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": usernames,
			"count": len(usernames),
		})
	})

	// Serve static demo page
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "demo.html")
	})

	log.Printf("Starting WebSocket chat server on :8080")
	log.Printf("Open http://localhost:8080 in your browser")
	log.Printf("Connect to /ws/chat?username=YourName for chat")
	log.Fatal(srv.Run())
}