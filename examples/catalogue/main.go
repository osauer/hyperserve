// The catalogue exposes one application through HTML, JSON, MCP and SSE.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/osauer/hyperserve"
)

//go:embed policy.json
var policyJSON []byte

func newApp() (*hyperserve.Server, error) {
	var policy Policy
	if err := json.Unmarshal(policyJSON, &policy); err != nil {
		return nil, err
	}
	app, _, err := newCatalogue(policy,
		hyperserve.WithAddr("127.0.0.1:8080"),
		hyperserve.WithEnvironment(),
		hyperserve.WithHealthServer(),
		hyperserve.WithHealthAddr("127.0.0.1:9080"),
		hyperserve.WithTimeouts(10*time.Second, 0, 30*time.Second),
		hyperserve.WithMCPSupport("catalogue", "1"),
	)
	if err != nil {
		return nil, err
	}
	// This public-token fixture must never bind a public interface, even when
	// deployment environment overrides the baseline address.
	host, _, err := net.SplitHostPort(app.Options().Addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		_ = app.Shutdown(context.Background())
		return nil, fmt.Errorf("catalogue requires a loopback IP address, got %q", app.Options().Addr)
	}
	return app, nil
}

func main() {
	app, err := newApp()
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
