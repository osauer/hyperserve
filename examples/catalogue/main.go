// The catalogue exposes one application through HTML, JSON, MCP and SSE.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/osauer/hyperserve/v3"
)

func main() {
	policyPath := flag.String("policy", "policy.json", "Application storage policy file")
	flag.Parse()
	raw, err := os.ReadFile(*policyPath)
	if err != nil {
		log.Fatal(err)
	}
	var policy Policy
	if err := json.Unmarshal(raw, &policy); err != nil {
		log.Fatal(err)
	}
	app, _, err := newCatalogue(policy, hyperserve.WithAddr("127.0.0.1:8080"), hyperserve.WithEnvironment(), hyperserve.WithMCPSupport("catalogue", "1"))
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("Catalogue: http://%s — in-memory documents; public demonstration token", app.Options().Addr)
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
