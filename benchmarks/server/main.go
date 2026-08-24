// Command server is the maintained loopback fixture for HyperServe load tests.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/osauer/hyperserve/pkg/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "loopback address to listen on")
	flag.Parse()

	srv, err := server.NewServer(
		server.WithAddr(*addr),
		server.WithLogLevel("ERROR"),
		server.WithAuthTokenValidator(func(token string) (bool, error) {
			return token == "benchmark-token", nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	srv.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv.HandleFunc("/minimal", okHandler)
	srv.HandleFunc("/middleware", okHandler)
	srv.AddMiddlewareStack("/middleware", server.MiddlewareStack{
		server.HeadersMiddleware(srv.Options),
		server.AuthMiddleware(srv.Options),
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.RunContext(ctx); err != nil {
		log.Fatal(err)
	}
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("OK"))
}
