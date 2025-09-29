package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	server "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	var port int
	flag.IntVar(&port, "port", 8080, "Port to listen on")
	flag.Parse()

	// Create server
	srv, err := server.NewServer(
		server.WithAddr(fmt.Sprintf(":%d", port)),
		server.WithRateLimit(100, 200),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Basic routes for benchmarking
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello from HyperServe Go!"))
	})

	// Echo endpoint for POST benchmarks
	srv.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
	})

	// JSON endpoint
	srv.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"message":   "Hello from HyperServe Go",
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	// Health check
	srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("Starting server on port %d", port)
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
