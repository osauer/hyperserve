package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/osauer/hyperserve"
)

func main() {
	fmt.Println("=== HyperServe CORS Configuration Example ===")
	fmt.Println("This example demonstrates CORS origin configuration")
	fmt.Println()

	// Method 1: Using functional options
	fmt.Println("--- Method 1: Functional Options ---")
	server1, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithCORSOrigins("https://example.com", "https://app.example.com"),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	fmt.Printf("Configured CORS origins: %v\n", server1.Options.CORSOrigins)

	// Method 2: Using environment variables
	fmt.Println("\n--- Method 2: Environment Variables ---")
	fmt.Println("Set HS_CORS_ORIGINS=https://env1.com,https://env2.com")
	fmt.Println("Then create server with default options")

	// Method 3: Wildcard (default behavior)
	fmt.Println("\n--- Method 3: Wildcard (Default) ---")
	server3, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8082"),
		// No CORS origins specified - will use wildcard "*"
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	fmt.Printf("Default CORS origins (empty = wildcard): %v\n", server3.Options.CORSOrigins)

	// Setup handlers
	server1.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "CORS headers configured!", "method": "` + r.Method + `"}`))
	})

	fmt.Println("\n--- Example Usage ---")
	fmt.Println("1. Start the server: go run main.go")
	fmt.Println("2. Test CORS headers:")
	fmt.Println("   curl -H 'Origin: https://example.com' http://localhost:8080/api/data")
	fmt.Println("3. Check Access-Control-Allow-Origin header in response")
	fmt.Println()
	fmt.Println("Server configured with CORS origins:", server1.Options.CORSOrigins)
	fmt.Println("Starting server on :8080...")

	if err := server1.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}