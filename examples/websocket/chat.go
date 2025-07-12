package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/osauer/hyperserve"
)

// ChatMessage represents a chat message
type ChatMessage struct {
	Type      string    `json:"type"`
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ChatClient represents a connected chat client
type ChatClient struct {
	conn     *websocket.Conn
	send     chan ChatMessage
	username string
}

// ChatHub maintains the set of active clients and broadcasts messages
type ChatHub struct {
	clients    map[*ChatClient]bool
	broadcast  chan ChatMessage
	register   chan *ChatClient
	unregister chan *ChatClient
	mutex      sync.RWMutex
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		clients:    make(map[*ChatClient]bool),
		broadcast:  make(chan ChatMessage),
		register:   make(chan *ChatClient),
		unregister: make(chan *ChatClient),
	}
}

func (h *ChatHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mutex.Lock()
			h.clients[client] = true
			h.mutex.Unlock()
			
			log.Printf("Client %s connected. Total clients: %d", client.username, len(h.clients))
			
			// Send welcome message
			welcomeMsg := ChatMessage{
				Type:      "system",
				Username:  "System",
				Message:   client.username + " joined the chat",
				Timestamp: time.Now(),
			}
			h.broadcast <- welcomeMsg

		case client := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("Client %s disconnected. Total clients: %d", client.username, len(h.clients))
				
				// Send goodbye message
				goodbyeMsg := ChatMessage{
					Type:      "system",
					Username:  "System",
					Message:   client.username + " left the chat",
					Timestamp: time.Now(),
				}
				h.broadcast <- goodbyeMsg
			}
			h.mutex.Unlock()

		case message := <-h.broadcast:
			h.mutex.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func (c *ChatClient) writePump() {
	defer c.conn.Close()
	
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("Write error for %s: %v", c.username, err)
				return
			}
		}
	}
}

func (c *ChatClient) readPump(hub *ChatHub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	
	for {
		var msg ChatMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for %s: %v", c.username, err)
			}
			break
		}
		
		msg.Username = c.username
		msg.Timestamp = time.Now()
		msg.Type = "message"
		
		hub.broadcast <- msg
	}
}

func main() {
	srv := hyperserve.NewServer(
		hyperserve.WithPort(8080),
		hyperserve.WithDebug(true),
	)

	// Add middleware stack
	srv.AddMiddleware("*", hyperserve.DefaultMiddleware(srv))

	hub := NewChatHub()
	go hub.Run()

	upgrader := websocket.Upgrader{
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

		username := r.URL.Query().Get("username")
		if username == "" {
			username = "Anonymous"
		}

		client := &ChatClient{
			conn:     conn,
			send:     make(chan ChatMessage, 256),
			username: username,
		}

		hub.register <- client

		// Start goroutines for this client
		go client.writePump()
		go client.readPump(hub)
	})

	// Get active users endpoint
	srv.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		hub.mutex.RLock()
		usernames := make([]string, 0, len(hub.clients))
		for client := range hub.clients {
			usernames = append(usernames, client.username)
		}
		hub.mutex.RUnlock()

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

	log.Printf("Starting WebSocket chat server on port 8080")
	log.Printf("Open http://localhost:8080 in your browser")
	log.Printf("Connect to /ws/chat?username=YourName for chat")
	log.Fatal(srv.ListenAndServe())
}