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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Filesystem roots are deliberately explicit: an embedding application's
	// working directory must never become web content by convention alone.
	app, err := hyperserve.New(hyperserve.WithStaticDir("./static"))
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	app.Use(hyperserve.HeadersMiddleware(app.Options()))

	if err := app.HandleStatic("/"); err != nil {
		log.Fatalf("Static files unavailable: %v", err)
	}

	app.GET("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	log.Println("static example listening on http://localhost:8080")

	if err := app.Run(ctx); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
