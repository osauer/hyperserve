package main

import (
	"log"
	"net/http"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
	"github.com/osauer/hyperserve/pkg/websocket"
)

func main() {
	srv, err := serverpkg.NewServer(
		serverpkg.WithAddr(":8080"),
	)
	if err != nil {
		log.Fatal(err)
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for demo
		},
	}

	// WebSocket echo handler
	srv.HandleFunc("/ws/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		log.Println("WebSocket connection established")

		// Echo loop
		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				break
			}

			log.Printf("Received: %s", string(p))

			if err := conn.WriteMessage(messageType, p); err != nil {
				log.Printf("Write error: %v", err)
				break
			}
		}
	})

	// Serve static demo page
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "demo.html")
	})

	log.Printf("Starting WebSocket echo server on :8080")
	log.Printf("Open http://localhost:8080 in your browser")
	log.Fatal(srv.Run())
}
