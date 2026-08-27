package hyperserve

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/mcp"
)

func TestRunCancellationGracefullyStopsServer(t *testing.T) {
	t.Parallel()

	var shutdownCalls atomic.Int32
	srv, err := New(
		WithAddr("127.0.0.1:0"),
		WithOnShutdown(func(context.Context) error {
			shutdownCalls.Add(1)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.Run(ctx)
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
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if srv.isRunning.Load() || srv.isReady.Load() {
		t.Fatalf("server state after cancellation: running=%v ready=%v", srv.isRunning.Load(), srv.isReady.Load())
	}
	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown hook calls = %d, want 1", got)
	}
}

func TestRunStartupFailureCleansPartiallyStartedServer(t *testing.T) {
	t.Parallel()

	mainListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve main address: %v", err)
	}
	defer mainListener.Close()

	healthProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve health address: %v", err)
	}
	healthAddr := healthProbe.Addr().String()
	if err := healthProbe.Close(); err != nil {
		t.Fatalf("release health address: %v", err)
	}

	var shutdownCalls atomic.Int32
	srv, err := New(
		WithAddr(mainListener.Addr().String()),
		WithHealthServer(),
		WithHealthAddr(healthAddr),
		WithOnShutdown(func(context.Context) error {
			shutdownCalls.Add(1)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = srv.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to listen") {
		t.Fatalf("Run startup error = %v, want listen failure", err)
	}
	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("shutdown hook calls = %d, want 1", got)
	}
	rebound, err := net.Listen("tcp", healthAddr)
	if err != nil {
		t.Fatalf("health listener remained open after main startup failure: %v", err)
	}
	_ = rebound.Close()
}

func TestRunAlreadyCanceledSkipsStartupAndCleansUp(t *testing.T) {
	t.Parallel()

	srv, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := srv.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if srv.httpServer != nil || srv.isRunning.Load() {
		t.Fatalf("pre-canceled context started server: httpServer=%v running=%v", srv.httpServer != nil, srv.isRunning.Load())
	}
}

func TestRunRejectsNilAndStdio(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		srv, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		//lint:ignore SA1012 Passing nil is intentional: this exercises the public guard.
		if err := srv.Run(nil); err == nil || !strings.Contains(err.Error(), "nil context") {
			t.Fatalf("Run(nil) error=%v, want nil-context error", err)
		}
	})

	t.Run("stdio", func(t *testing.T) {
		srv, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		srv.options.MCPEnabled = true
		srv.options.MCPTransport = mcp.StdioTransport

		if err := srv.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "does not support MCP stdio") {
			t.Fatalf("Run(stdio) error=%v, want unsupported-transport error", err)
		}
	})
}
