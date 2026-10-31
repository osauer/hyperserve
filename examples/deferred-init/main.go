// Deferred-init example.
//
// Demonstrates keeping the health listener live while long-running bootstrap
// work runs in the background. Application routes return 503 until bootstrap
// and the OnReady hook both succeed, then the server flips to ready.
//
// Run it:
//
//	go run ./examples/deferred-init
//
// Then in another shell:
//
//	curl -i http://localhost:9080/healthz/   # 200 immediately
//	curl -i http://localhost:9080/readyz/    # 503, then 200
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

	"github.com/osauer/hyperserve/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app, err := hyperserve.New(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithHealthServer(),
		hyperserve.WithHealthAddr(":9080"),

		// The listeners are up while this runs. Health stays 200, readiness and
		// application traffic stay 503.
		hyperserve.WithDeferredInit(func(ctx context.Context, _ *hyperserve.Server) error {
			log.Println("[bootstrap] warming caches...")
			return warmCaches(ctx)
		}),

		// Hook runs after deferred init succeeds, before the server flips to
		// ready. Routes registered here only become reachable once ready.
		hyperserve.WithOnReady(func(_ context.Context, app *hyperserve.Server) error {
			app.HandleFunc("/api/users", usersHandler)
			log.Println("[ready] /api/users registered")
			return nil
		}),

		// Optional: keep the listener up even if bootstrap fails, so an
		// operator can inspect health and call CompleteDeferredInit
		// manually after fixing the underlying issue.
		hyperserve.WithDeferredInitStopOnFailure(false),
	)
	if err != nil {
		log.Fatalf("hyperserve.New: %v", err)
	}

	log.Println("application on :8080; health and readiness on :9080")
	if err := app.Run(ctx); err != nil {
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
