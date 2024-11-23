package main

import (
	"fmt"
	"math/rand"
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

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			number := rand.Intn(100)
			fmt.Fprintf(w, "data: %d\n\n", number)
			flusher.Flush()
		}
	}
}

func main() {
	// Initialize hs
	srv, err := hs.NewServer(hs.WithLoglevel(hs.LevelDebug))
	if err != nil {
		panic(err)
	}

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