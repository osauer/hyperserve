package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/pkg/mcp"
)

func TestRunContextCancellationGracefullyStopsServer(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(
		WithAddr("127.0.0.1:0"),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.RunContext(ctx)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !srv.isRunning.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !srv.isRunning.Load() {
		cancel()
		t.Fatal("server did not start")
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("RunContext after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunContext did not return after cancellation")
	}

	if srv.isRunning.Load() || srv.isReady.Load() {
		t.Fatalf("server state after cancellation: running=%v ready=%v", srv.isRunning.Load(), srv.isReady.Load())
	}
}

func TestRunContextAlreadyCanceledSkipsStartupAndCleansUp(t *testing.T) {
	t.Parallel()

	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := srv.RunContext(ctx); err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	if srv.httpServer != nil || srv.isRunning.Load() {
		t.Fatalf("pre-canceled context started server: httpServer=%v running=%v", srv.httpServer != nil, srv.isRunning.Load())
	}
	select {
	case <-srv.rateLimiters.cleanupDone:
	default:
		t.Fatal("pre-canceled RunContext did not release server cleanup resources")
	}
}

func TestRunContextRejectsNilAndStdio(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		srv, err := NewServer()
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.stopCleanup()

		//lint:ignore SA1012 Passing nil is intentional: this exercises the public guard.
		if err := srv.RunContext(nil); err == nil || !strings.Contains(err.Error(), "nil context") {
			t.Fatalf("RunContext(nil) error=%v, want nil-context error", err)
		}
	})

	t.Run("stdio", func(t *testing.T) {
		srv, err := NewServer()
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		defer srv.stopCleanup()
		srv.Options.MCPEnabled = true
		srv.Options.MCPTransport = mcp.StdioTransport

		if err := srv.RunContext(context.Background()); err == nil || !strings.Contains(err.Error(), "does not support MCP stdio") {
			t.Fatalf("RunContext(stdio) error=%v, want unsupported-transport error", err)
		}
	})
}
