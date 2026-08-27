// Command enterprise demonstrates HyperServe's restricted TLS handshake policy.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	serverpkg "github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := serverpkg.NewServer(
		serverpkg.WithTLS("cert.pem", "key.pem"),
		serverpkg.WithFIPSMode(),
		serverpkg.WithHealthServer(),
		serverpkg.WithHealthAddr(":9080"),
	)
	if err != nil {
		log.Fatal(err)
	}

	srv.Use(serverpkg.HeadersMiddleware(srv.Options()))
	srv.GET("/", describePolicy)

	log.Println("TLS example listening on https://localhost:8443")
	log.Println("health checks listening on http://localhost:9080/healthz/")
	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func describePolicy(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "TLS uses the cipher suites and curves selected by WithFIPSMode.")
	fmt.Fprintln(w, "This setting alone is not FIPS 140-3 compliance.")
}
