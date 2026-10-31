package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/ratelimit"
)

func main() {
	restore := preserveEnvironment("HS_PORT")
	defer restore()

	configFile, err := os.CreateTemp("", "hyperserve-options-*.json")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(configFile.Name())

	// The file supplies a deployable baseline.
	fileConfig := map[string]any{
		"addr": ":8084",
	}
	if err := json.NewEncoder(configFile).Encode(fileConfig); err != nil {
		log.Fatal(err)
	}
	if err := configFile.Close(); err != nil {
		log.Fatal(err)
	}

	mustSetenv("HS_PORT", "8085") // Environment overrides only the file's address.

	loaded, err := hyperserve.New(
		hyperserve.WithConfigFile(configFile.Name()),
		hyperserve.WithEnvironment(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = loaded.Shutdown(context.Background()) }()
	fmt.Println("After defaults, file, and environment:")
	printAddress(loaded.Options())

	// Server options apply left to right, so the final address enforces an
	// application invariant.
	app, err := hyperserve.New(
		hyperserve.WithConfigFile(configFile.Name()),
		hyperserve.WithEnvironment(),
		hyperserve.WithAddr(":8086"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := app.Shutdown(context.Background()); err != nil {
			log.Printf("stop server: %v", err)
		}
	}()

	// Rate limiting is separate application policy. Create a middleware gate,
	// then place that gate in front of the path it protects.
	apiPolicy := ratelimit.Config{
		RequestsPerSecond: 10,
		Burst:             20,
	}
	apiGate, err := ratelimit.New(apiPolicy)
	if err != nil {
		log.Fatal(err)
	}
	app.UsePrefix("/api", apiGate)

	fmt.Println("\nAfter the application-owned address option:")
	printAddress(app.Options())
	fmt.Printf("  /api gate: %.0f requests/second, burst %d\n", apiPolicy.RequestsPerSecond, apiPolicy.Burst)
}

func printAddress(options hyperserve.Options) {
	fmt.Printf("  address: %s\n", options.Addr)
}

func mustSetenv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		log.Fatal(err)
	}
}

func preserveEnvironment(keys ...string) func() {
	type value struct {
		text string
		set  bool
	}

	previous := make(map[string]value, len(keys))
	for _, key := range keys {
		text, set := os.LookupEnv(key)
		previous[key] = value{text: text, set: set}
	}

	return func() {
		for key, old := range previous {
			if old.set {
				_ = os.Setenv(key, old.text)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
}
