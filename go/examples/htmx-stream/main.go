// This example demonstrates a minimal HTMX setup with Server-Sent Events (SSE) in Go.
// It includes a Go server that streams random numbers to the client every 100ms.
// The client-side HTML uses HTMX to connect to the SSE endpoint and update the content in real-time.
// Key learning points:
// - Setting up a basic Go server with SSE support
// - Using HTMX for real-time updates in the browser
// - Configuring server and client-side code for SSE

package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	hs "github.com/osauer/hyperserve/go"
)

func numbersStreamHandler(w http.ResponseWriter, r *http.Request) {
	// set headers for server-sent events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Flusher to send buffered data to the client. Make sure the http.ResponseWriter supports flushing in case
	// you use a custom one (must implement http.Flusher interface).
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send a random number every 100ms
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Create a new SSE message, with empty data. The data will be updated in the loop.
	sseMessage := hs.NewSSEMessage("")
	if _, err := fmt.Fprint(w, sseMessage); err != nil {
		log.Println("Error creating SSE message:", err)
	}

	// Loop until the client closes the connection
	for {
		select {
		case <-r.Context().Done():
			log.Println("SSE connection closed", r.Context().Err())
			return
		case <-ticker.C:
			// Create dynamic data
			data := map[string]interface{}{
				"value":     rand.Intn(100),
				"timestamp": time.Now().Format("15:04:05"),
			}

			// Use the improved SSE message formatting
			sseMessage := hs.NewSSEMessage(data)
			if _, err := fmt.Fprint(w, sseMessage); err != nil {
				log.Println("Error sending SSE message:", err)
				return
			}
			flusher.Flush() // Ensure the message is sent immediately
		}
	}
}

func main() {
	// Initialize the server
	srv, err := hs.NewServer()
	if err != nil {
		panic(err)
	}
	// proper timeout settings for streaming
	srv.Options.ReadTimeout = 0
	srv.Options.WriteTimeout = 0
	srv.Options.IdleTimeout = 0

	// Configure template and static directories
	srv.Options.TemplateDir = "./templates"
	srv.Options.StaticDir = "./static"
	srv.HandleStatic("/static/")

	// Handler for streaming
	srv.HandleFunc("/numbers/stream", numbersStreamHandler)

	// Serve the main template
	srv.HandleTemplate("/", "index.html", nil)

	// Run the srv
	err = srv.Run()
	if err != nil {
		fmt.Printf("Error running srv: %v", err)
	}
}
