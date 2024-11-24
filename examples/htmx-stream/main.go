package main

import (
	"fmt"
	"log"
	"math/rand"

	// "math/rand"
	"net/http"
	"time"

	hs "github.com/osauer/hyperserve"
)

func numbersStreamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	log.Println("SSE connection opened")

	ticker := time.NewTicker(1 * time.Second) // Slower for testing
	defer ticker.Stop()

	sseMessage := hs.NewSEEventMessage("hello world")
	sseMessage.Event = "sse:numbers"
	if _, err := fmt.Fprint(w, sseMessage); err != nil {
		log.Println("Error creating SSE message:", err)
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			log.Println("SSE connection closed", r.Context().Err())
			return
		case <-ticker.C:
			number := rand.Intn(100)
			log.Println("Sending number:", number)
			// Send HTMX-compatible SSE data
			sseMessage.Data = number
			fmt.Fprint(w, sseMessage)
			flusher.Flush()
			log.Println("Sent number:", number)
		}
	}
}

func main() {
	// Initialize hs
	srv, err := hs.NewServer(
		hs.WithLoglevel(hs.LevelDebug))
	if err != nil {
		panic(err)
	}

	srv.Options.ReadTimeout = 0
	srv.Options.WriteTimeout = 0
	srv.Options.IdleTimeout = 0

	// Configure template and static directories
	srv.Options.TemplateDir = "examples/htmx-stream/templates"
	srv.Options.StaticDir = "examples/htmx-dynamic/static"
	srv.HandleStatic("/static/")

	// Handle random number streaming
	srv.HandleFunc("/numbers/stream", numbersStreamHandler)

	// Serve the main template
	srv.HandleTemplate("/", "index.html", nil)

	// Run the srv
	err = srv.Run()
	if err != nil {
		fmt.Printf("Error running srv: %v", err)
	}
}
