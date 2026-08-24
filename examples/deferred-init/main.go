// Deferred-init example.
//
// Demonstrates serving /healthz immediately while long-running bootstrap work
// (warmCaches) runs in the background. Application routes return 503 until
// bootstrap and the OnReady hook both succeed, then the server flips to ready.
//
// Run it:
//
//	go run examples/deferred-init/main.go
//
// Then in another shell:
//
//	curl -i http://localhost:8080/healthz  # 200 immediately
//	curl -i http://localhost:8080/api/users  # 503 until ready (~3s), then 200
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	server "github.com/osauer/hyperserve/v2/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv, err := server.NewServer(
		server.WithAddr(":8080"),
		server.WithHealthServer(),

		// Long-running bootstrap. Listener is up while this runs; /healthz
		// returns 200 but application routes return 503.
		server.WithDeferredInit(func(ctx context.Context, _ *server.Server) error {
			log.Println("[bootstrap] warming caches...")
			return warmCaches(ctx)
		}),

		// Hook runs after deferred init succeeds, before the server flips to
		// ready. Routes registered here only become reachable once ready.
		server.WithOnReady(func(_ context.Context, app *server.Server) error {
			app.HandleFunc("/api/users", usersHandler)
			log.Println("[ready] /api/users registered")
			return nil
		}),

		// Optional: keep the listener up even if bootstrap fails, so an
		// operator can introspect /healthz and call CompleteDeferredInit
		// manually after fixing the underlying issue.
		server.WithDeferredInitStopOnFailure(false),
	)
	if err != nil {
		log.Fatalf("NewServer: %v", err)
	}

	log.Println("starting on :8080 — /healthz live, /api/users 503 until ready")
	if err := srv.Run(ctx); err != nil {
		log.Fatalf("Run: %v", err)
	}
}

func warmCaches(ctx context.Context) error {
	select {
	case <-time.After(3 * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func usersHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"users": []string{"alice", "bob"},
	})
}
