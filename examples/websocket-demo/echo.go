package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"os/signal"

	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

//go:embed demo.html
var demoHTML []byte

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := serverpkg.NewServer(
		serverpkg.WithAddr(":8080"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// The server-owned upgrader keeps the package's same-origin default and
	// records successful upgrades in HyperServe's WebSocket telemetry.
	upgrader := srv.WebSocketUpgrader()
	upgrader.MaxMessageSize = 512 << 10

	// WebSocket echo handler
	srv.GET("/ws/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		log.Println("WebSocket connection established")

		// Context-aware I/O stops promptly when the request is cancelled.
		for {
			messageType, payload, err := conn.Read(r.Context())
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}

			log.Printf("Received: %s", string(payload))

			if err := conn.Write(r.Context(), messageType, payload); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
		}
	})

	// Embedding the page keeps the example runnable from any working directory.
	srv.GET("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(demoHTML); err != nil {
			log.Printf("Write demo page: %v", err)
		}
	})

	log.Printf("Starting WebSocket echo server on :8080")
	log.Printf("Open http://localhost:8080 in your browser")
	log.Fatal(srv.Run(ctx))
}
