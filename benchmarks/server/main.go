// Command server is the maintained loopback fixture for HyperServe load tests.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/osauer/hyperserve/v2/pkg/auth"
	"github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "loopback address to listen on")
	flag.Parse()

	srv, err := server.NewServer(
		server.WithAddr(*addr),
		server.WithLogLevel("ERROR"),
	)
	if err != nil {
		log.Fatal(err)
	}

	srv.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv.HandleFunc("/minimal", okHandler)
	srv.HandleFunc("/middleware", okHandler)
	authenticator := auth.Bearer(benchmarkVerifier{})
	srv.UsePrefix("/middleware",
		server.HeadersMiddleware(srv.Options()),
		auth.Require(authenticator),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil {
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
