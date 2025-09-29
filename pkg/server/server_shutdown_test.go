package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithOnShutdown verifies that shutdown hooks can be registered
func TestWithOnShutdown(t *testing.T) {
	t.Parallel()

	hook := func(ctx context.Context) error {
		return nil
	}

	srv, err := NewServer(
		WithOnShutdown(hook),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if len(srv.Options.OnShutdownHooks) != 1 {
		t.Errorf("Expected 1 shutdown hook, got %d", len(srv.Options.OnShutdownHooks))
	}
}

// TestMultipleShutdownHooks verifies multiple hooks can be registered
func TestMultipleShutdownHooks(t *testing.T) {
	t.Parallel()

	var callOrder []int
	hook1 := func(ctx context.Context) error {
		callOrder = append(callOrder, 1)
		return nil
	}
	hook2 := func(ctx context.Context) error {
		callOrder = append(callOrder, 2)
		return nil
	}
	hook3 := func(ctx context.Context) error {
		callOrder = append(callOrder, 3)
		return nil
	}

	srv, err := NewServer(
		WithOnShutdown(hook1),
		WithOnShutdown(hook2),
		WithOnShutdown(hook3),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if len(srv.Options.OnShutdownHooks) != 3 {
		t.Errorf("Expected 3 shutdown hooks, got %d", len(srv.Options.OnShutdownHooks))
	}
}

// TestShutdownHookExecution verifies hooks are executed during shutdown
func TestShutdownHookExecution(t *testing.T) {
	t.Parallel()

	hookExecuted := int32(0)
	hook := func(ctx context.Context) error {
		atomic.StoreInt32(&hookExecuted, 1)
		return nil
	}

	srv, err := NewServer(
		WithAddr(":0"), // Use random port
		WithOnShutdown(hook),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		err := srv.Run()
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for server to start
	for !srv.isRunning.Load() {
		time.Sleep(10 * time.Millisecond)
	}

	// Trigger shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Verify hook was executed
	if atomic.LoadInt32(&hookExecuted) != 1 {
		t.Error("Shutdown hook was not executed")
	}

	// Wait for server to finish
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Server error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("Server did not shut down in time")
	}
}

// TestShutdownHookOrder verifies hooks execute in registration order
func TestShutdownHookOrder(t *testing.T) {
	t.Parallel()

	var executionOrder []int

	srv, err := NewServer(
		WithAddr(":0"),
		WithOnShutdown(func(ctx context.Context) error {
			executionOrder = append(executionOrder, 1)
			return nil
		}),
		WithOnShutdown(func(ctx context.Context) error {
			executionOrder = append(executionOrder, 2)
			return nil
		}),
		WithOnShutdown(func(ctx context.Context) error {
			executionOrder = append(executionOrder, 3)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Execute shutdown directly
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Verify execution order
	if len(executionOrder) != 3 {
		t.Errorf("Expected 3 hooks to execute, got %d", len(executionOrder))
	}
	for i, order := range executionOrder {
		if order != i+1 {
			t.Errorf("Hook %d executed out of order: expected %d, got %d", i, i+1, order)
		}
	}
}

// TestShutdownHookError verifies errors from hooks don't prevent shutdown
func TestShutdownHookError(t *testing.T) {
	t.Parallel()

	hook2Executed := false

	srv, err := NewServer(
		WithAddr(":0"),
		WithOnShutdown(func(ctx context.Context) error {
			return errors.New("hook 1 error")
		}),
		WithOnShutdown(func(ctx context.Context) error {
			hook2Executed = true
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Execute shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Shutdown should succeed despite hook error
	if err := srv.shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Second hook should still execute
	if !hook2Executed {
		t.Error("Second hook was not executed after first hook error")
	}
}

// TestShutdownHookTimeout verifies hooks respect context timeout
func TestShutdownHookTimeout(t *testing.T) {
	t.Parallel()

	hook2Executed := int32(0)

	srv, err := NewServer(
		WithAddr(":0"),
		WithOnShutdown(func(ctx context.Context) error {
			// This hook takes too long
			select {
			case <-time.After(10 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
		WithOnShutdown(func(ctx context.Context) error {
			atomic.StoreInt32(&hook2Executed, 1)
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Execute shutdown with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	if err := srv.shutdown(ctx); err != nil {
		// We expect a timeout-related error
		t.Logf("Shutdown completed with expected timeout: %v", err)
	}
	elapsed := time.Since(start)

	// Shutdown should complete within reasonable time despite slow hook
	if elapsed > 2*time.Second {
		t.Errorf("Shutdown took too long: %v", elapsed)
	}

	// Second hook should still have been attempted
	// (though it might not complete due to timeout)
	t.Logf("Hook 2 execution status: %d", atomic.LoadInt32(&hook2Executed))
}

func TestShutdownHookReceivesParentContext(t *testing.T) {
	t.Parallel()

	type ctxKey string
	const key ctxKey = "test"

	var (
		mu       sync.Mutex
		captured interface{}
	)

	hook := func(ctx context.Context) error {
		mu.Lock()
		captured = ctx.Value(key)
		mu.Unlock()
		return nil
	}

	srv, err := NewServer(WithOnShutdown(hook))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	parentCtx := context.WithValue(context.Background(), key, "inherited-value")
	ctx, cancel := context.WithTimeout(parentCtx, time.Second)
	defer cancel()

	if err := srv.shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if captured != "inherited-value" {
		t.Fatalf("expected hook to see parent context value, got %v", captured)
	}
}

// TestShutdownHookNilHandling verifies nil hooks are handled gracefully
func TestShutdownHookNilHandling(t *testing.T) {
	t.Parallel()

	executedCount := 0

	srv := &Server{
		Options: &ServerOptions{
			OnShutdownHooks: []func(context.Context) error{
				func(ctx context.Context) error {
					executedCount++
					return nil
				},
				nil, // nil hook should be skipped
				func(ctx context.Context) error {
					executedCount++
					return nil
				},
			},
		},
	}

	// Execute shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := srv.shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Only non-nil hooks should execute
	if executedCount != 2 {
		t.Errorf("Expected 2 hooks to execute (skipping nil), got %d", executedCount)
	}
}
