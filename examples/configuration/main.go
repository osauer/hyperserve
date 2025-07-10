package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/osauer/hyperserve"
)

func main() {
	fmt.Println("=== HyperServe Configuration Example ===")
	fmt.Println("This example demonstrates the three ways to configure HyperServe:")
	fmt.Println("1. Environment variables (highest priority)")
	fmt.Println("2. JSON configuration file")
	fmt.Println("3. Programmatic options (lowest priority)")
	fmt.Println()

	// Show current environment variables
	fmt.Println("Current environment variables:")
	showEnvVars()
	fmt.Println()

	// Method 1: Programmatic configuration (lowest priority)
	fmt.Println("--- Method 1: Programmatic Configuration ---")
	runProgrammaticConfig()

	// Method 2: JSON configuration file
	fmt.Println("\n--- Method 2: JSON Configuration File ---")
	runJSONConfig()

	// Method 3: Environment variables (highest priority)
	fmt.Println("\n--- Method 3: Environment Variables ---")
	runEnvConfig()

	// Method 4: Combined configuration showing precedence
	fmt.Println("\n--- Method 4: Combined Configuration (Precedence Demo) ---")
	runCombinedConfig()
}

// Show relevant environment variables
func showEnvVars() {
	envVars := []string{
		"HS_PORT",
		"HS_ADDR",
		"HS_RATE_LIMIT",
		"HS_BURST_LIMIT",
		"HS_LOG_LEVEL",
		"HS_CONFIG_PATH",
	}

	hasAny := false
	for _, env := range envVars {
		if val := os.Getenv(env); val != "" {
			fmt.Printf("  %s=%s\n", env, val)
			hasAny = true
		}
	}
	if !hasAny {
		fmt.Println("  (none set)")
	}
}

// Method 1: Programmatic configuration
func runProgrammaticConfig() {
	server, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8081"), // Different port
		hyperserve.WithRateLimit(50, 100),
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	setupHandlers(server, "Programmatic")

	fmt.Println("Configuration:")
	fmt.Printf("  Port: %s\n", server.Options.Addr)
	fmt.Printf("  Rate Limit: %.0f req/s\n", float64(server.Options.RateLimit))
	fmt.Printf("  Burst Limit: %d\n", server.Options.Burst)

	fmt.Println("\nPress Enter to continue to next method...")
	fmt.Scanln()
}

// Method 2: JSON configuration file
func runJSONConfig() {
	// Create a sample config file
	config := map[string]interface{}{
		"addr":        ":8082",
		"rate_limit":  25,
		"burst":       50,
	}

	configPath := "server-config.json"
	file, err := os.Create(configPath)
	if err != nil {
		log.Fatalf("Failed to create config file: %v", err)
	}
	json.NewEncoder(file).Encode(config)
	file.Close()

	fmt.Printf("Created config file: %s\n", configPath)
	fmt.Println("Contents:")
	data, _ := os.ReadFile(configPath)
	fmt.Printf("%s\n", data)

	// Set config path
	os.Setenv("HS_CONFIG_PATH", configPath)

	// Create server (will read from JSON)
	server, err := hyperserve.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	setupHandlers(server, "JSON Config")

	fmt.Println("Configuration loaded from JSON:")
	fmt.Printf("  Port: %s\n", server.Options.Addr)
	fmt.Printf("  Rate Limit: %.0f req/s\n", float64(server.Options.RateLimit))
	fmt.Printf("  Burst Limit: %d\n", server.Options.Burst)

	// Cleanup
	os.Remove(configPath)
	os.Unsetenv("HS_CONFIG_PATH")

	fmt.Println("\nPress Enter to continue to next method...")
	fmt.Scanln()
}

// Method 3: Environment variables
func runEnvConfig() {
	// Set environment variables
	os.Setenv("HS_PORT", "8083")
	os.Setenv("HS_RATE_LIMIT", "200")
	os.Setenv("HS_BURST_LIMIT", "400")

	fmt.Println("Set environment variables:")
	showEnvVars()
	fmt.Println()

	// Create server (will read from env)
	server, err := hyperserve.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	setupHandlers(server, "Environment Variables")

	fmt.Println("Configuration loaded from environment:")
	fmt.Printf("  Port: %s\n", server.Options.Addr)
	fmt.Printf("  Rate Limit: %.0f req/s\n", float64(server.Options.RateLimit))
	fmt.Printf("  Burst Limit: %d\n", server.Options.Burst)

	// Cleanup
	os.Unsetenv("HS_PORT")
	os.Unsetenv("HS_RATE_LIMIT")
	os.Unsetenv("HS_BURST_LIMIT")

	fmt.Println("\nPress Enter to continue to precedence demo...")
	fmt.Scanln()
}

// Method 4: Combined configuration showing precedence
func runCombinedConfig() {
	fmt.Println("This demonstrates configuration precedence:")
	fmt.Println("Environment Variables > JSON File > Programmatic")
	fmt.Println()

	// 1. Create JSON config
	config := map[string]interface{}{
		"addr":        ":8084",
		"rate_limit":  75,
		"burst":       150,
	}

	configPath := "precedence-config.json"
	file, err := os.Create(configPath)
	if err != nil {
		log.Fatalf("Failed to create config file: %v", err)
	}
	json.NewEncoder(file).Encode(config)
	file.Close()

	// 2. Set some environment variables (these will override JSON)
	os.Setenv("HS_CONFIG_PATH", configPath)
	os.Setenv("HS_PORT", "8085")     // Override port only

	// 3. Create server with programmatic options (lowest priority)
	server, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8086"),      // Will be overridden
		hyperserve.WithRateLimit(10, 20),  // Will be overridden
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	setupHandlers(server, "Precedence Demo")

	fmt.Println("Sources:")
	fmt.Println("  Programmatic: port=:8086, rate=10, burst=20")
	fmt.Println("  JSON File:    port=:8084, rate=75, burst=150")
	fmt.Println("  Environment:  port=:8085")
	fmt.Println()
	fmt.Println("Final configuration (after precedence):")
	fmt.Printf("  Port: %s (from environment)\n", server.Options.Addr)
	fmt.Printf("  Rate Limit: %d req/s (from JSON)\n", server.Options.RateLimit)
	fmt.Printf("  Burst Limit: %d (from JSON)\n", server.Options.Burst)

	// Cleanup
	os.Remove(configPath)
	os.Unsetenv("HS_CONFIG_PATH")
	os.Unsetenv("HS_PORT")

	fmt.Println("\nConfiguration example complete!")
}

// Setup common handlers
func setupHandlers(server *hyperserve.Server, configType string) {
	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"message":      fmt.Sprintf("Server configured via: %s", configType),
			"port":         server.Options.Addr,
			"rate_limit":   server.Options.RateLimit,
			"burst_limit":  server.Options.Burst,
		}
		json.NewEncoder(w).Encode(response)
	})
}