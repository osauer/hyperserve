package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	serverpkg "github.com/osauer/hyperserve/pkg/server"
)

func main() {
	restore := preserveEnvironment("HS_PORT", "HS_RATE_LIMIT", "HS_BURST_LIMIT")
	defer restore()

	configFile, err := os.CreateTemp("", "hyperserve-options-*.json")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(configFile.Name())

	// The file supplies a deployable baseline.
	fileConfig := map[string]any{
		"addr":       ":8084",
		"rate_limit": 75,
		"burst":      150,
	}
	if err := json.NewEncoder(configFile).Encode(fileConfig); err != nil {
		log.Fatal(err)
	}
	if err := configFile.Close(); err != nil {
		log.Fatal(err)
	}

	mustSetenv("HS_PORT", "8085") // Environment overrides only the file's address.
	mustSetenv("HS_RATE_LIMIT", "")
	mustSetenv("HS_BURST_LIMIT", "")

	loaded, err := serverpkg.NewServer(
		serverpkg.WithConfigFile(configFile.Name()),
		serverpkg.WithEnvironment(),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = loaded.Stop() }()
	fmt.Println("After defaults, file, and environment:")
	printOptions(loaded.Options)

	// Options apply left to right, so the final two calls enforce application invariants.
	srv, err := serverpkg.NewServer(
		serverpkg.WithConfigFile(configFile.Name()),
		serverpkg.WithEnvironment(),
		serverpkg.WithAddr(":8086"),
		serverpkg.WithRateLimit(10, 20),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := srv.Stop(); err != nil {
			log.Printf("stop server: %v", err)
		}
	}()

	fmt.Println("\nAfter programmatic options:")
	printOptions(srv.Options)
}

func printOptions(options *serverpkg.ServerOptions) {
	fmt.Printf("  address: %s\n", options.Addr)
	fmt.Printf("  rate:    %.0f requests/second\n", float64(options.RateLimit))
	fmt.Printf("  burst:   %d\n", options.Burst)
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
