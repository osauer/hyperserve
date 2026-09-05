// Command server is the maintained loopback fixture for HyperServe load tests.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/auth"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "loopback address to listen on")
	runID := flag.String("run-id", "", "identity echoed by the readiness endpoint")
	flag.Parse()

	app, err := hyperserve.New(
		hyperserve.WithAddr(*addr),
		hyperserve.WithLogLevel("ERROR"),
	)
	if err != nil {
		log.Fatal(err)
	}

	app.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(*runID))
	})
	app.HandleFunc("/minimal", okHandler)
	app.HandleFunc("/middleware", okHandler)
	authenticator := auth.Bearer(benchmarkVerifier{})
	app.UsePrefix("/middleware",
		hyperserve.HeadersMiddleware(app.Options()),
		auth.Require(authenticator),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

type benchmarkVerifier struct{}

func (benchmarkVerifier) VerifyToken(_ context.Context, token string) (auth.Principal, error) {
	if token != "benchmark-token" {
		return auth.Principal{}, auth.ErrUnauthenticated
	}
	return auth.Principal{Issuer: "benchmark", Subject: "load-client"}, nil
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("OK"))
}
