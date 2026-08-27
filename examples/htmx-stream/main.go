// This example demonstrates a minimal HTMX setup with Server-Sent Events (SSE) in Go.
// It includes a Go server that streams random numbers to the client every 100ms.
// The client-side HTML uses HTMX to connect to the SSE endpoint and update the content in real-time.
// Key learning points:
// - Setting up a basic Go server with SSE support
// - Using HTMX for real-time updates in the browser
// - Configuring server and client-side code for SSE

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/osauer/hyperserve/v2"
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
	sseMessage := hyperserve.NewSSEMessage("")
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
			data := map[string]any{
				"value":     rand.Intn(100),
				"timestamp": time.Now().Format("15:04:05"),
			}

			// Use the improved SSE message formatting
			sseMessage := hyperserve.NewSSEMessage(data)
			if _, err := fmt.Fprint(w, sseMessage); err != nil {
				log.Println("Error sending SSE message:", err)
				return
			}
			flusher.Flush() // Ensure the message is sent immediately
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Initialize the server
	app, err := hyperserve.New(
		hyperserve.WithTimeouts(0, 0, 0),
		hyperserve.WithTemplateDir("./templates"),
		hyperserve.WithStaticDir("./static"),
	)
	if err != nil {
		panic(err)
	}
	if err := app.HandleStatic("/static/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	// Handler for streaming
	app.HandleFunc("/numbers/stream", numbersStreamHandler)

	// Serve the main template
	app.HandleTemplate("/", "index.html", nil)

	// Run the app
	err = app.Run(ctx)
	if err != nil {
		fmt.Printf("Error running app: %v", err)
	}
}
