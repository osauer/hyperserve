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
	"github.com/osauer/hyperserve/v2/sse"
)

func numbersStreamHandler(w http.ResponseWriter, r *http.Request) {
	stream, err := sse.NewWriter(w, 5*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := stream.Comment("connected"); err != nil {
		log.Println("SSE connection:", err)
		return
	}

	// Send a random number every 100ms
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

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

			if err := stream.SendJSON("message", "", data); err != nil {
				log.Println("Error sending SSE message:", err)
				return
			}
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
