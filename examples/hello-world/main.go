package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/osauer/hyperserve/v2"
)

func main() {
	// This executable owns Ctrl+C. HyperServe follows ctx and cleans up the
	// server resources it started.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app, err := hyperserve.New()
	if err != nil {
		log.Fatal(err)
	}

	app.HandleFunc("/", hello)

	log.Println("Starting server on http://localhost:8080")
	log.Println("Press Ctrl+C to stop")

	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func hello(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "Hello, World from HyperServe!")
}
