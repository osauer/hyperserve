package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBootstrapReadinessHandlerFallback(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	handler := srv.bootstrapReadinessHandler(srv.Handler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for non-health route, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "service initializing") {
		t.Errorf("expected initialization message, got %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz during bootstrap, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "ok" {
		t.Errorf("expected body 'ok', got %q", body)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /readyz during bootstrap, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "initializing" {
		t.Errorf("expected body 'initializing', got %q", body)
	}
}

func TestBootstrapReadinessHandlerCustomHealthOverride(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		if _, err := w.Write([]byte("custom")); err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
	})

	handler := srv.bootstrapReadinessHandler(srv.Handler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected custom handler to run, got status %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "custom" {
		t.Errorf("expected body 'custom', got %q", body)
	}
}

func TestDeferredInitLifecycle(t *testing.T) {
	release := make(chan struct{})

	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return nil
			}
		}),
		WithOnReady(func(ctx context.Context, app *Server) error {
			app.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte("ready")); err != nil {
					t.Fatalf("unexpected write error: %v", err)
				}
			})
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	srv.lifecycleCtx, srv.lifecycleCancel = context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	srv.startDeferredInit(errChan)

	handler := srv.bootstrapReadinessHandler(srv.Handler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for application route before readiness, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for healthz during bootstrap, got %d", rec.Code)
	}

	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for !srv.isReady.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.isReady.Load() {
		t.Fatal("server never transitioned to ready state")
	}

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("unexpected deferred init error: %v", err)
		}
	default:
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for application route after readiness, got %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "ready" {
		t.Errorf("expected body 'ready', got %q", body)
	}
}

func TestDeferredInitFailureStopsServer(t *testing.T) {
	srv, err := NewServer(
		WithAddr(":0"),
		WithSuppressBanner(true),
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return errors.New("init boom")
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run()
	}()

	select {
	case err := <-serverErr:
		if err == nil {
			t.Fatal("expected deferred init error, got nil")
		}
		if !strings.Contains(err.Error(), "Deferred initialization failed") {
			t.Fatalf("expected deferred init failure message, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not return after deferred init failure")
	}

	if srv.isRunning.Load() {
		t.Error("server should not be running after deferred init failure")
	}
}

func TestDeferredInitFailureWithoutShutdown(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return errors.New("init boom")
		}),
		WithDeferredInitStopOnFailure(false),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	srv.lifecycleCtx, srv.lifecycleCancel = context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	srv.startDeferredInit(errChan)

	time.Sleep(50 * time.Millisecond)

	select {
	case err := <-errChan:
		t.Fatalf("did not expect deferred init to request shutdown, got %v", err)
	default:
	}

	if srv.isReady.Load() {
		t.Error("server should not be ready after deferred init failure")
	}

	if err := srv.getDeferredInitError(); err == nil {
		t.Fatal("expected deferred init error to be recorded")
	}
}

func TestBootstrapHealthDoesNotMatchPrefix(t *testing.T) {
	t.Parallel()

	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			<-ctx.Done()
			return ctx.Err()
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	h := srv.bootstrapReadinessHandler(srv.Handler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz-backup", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for prefixed health path, got %d", rec.Code)
	}
}

func TestCompleteDeferredInitAllowsManualRecovery(t *testing.T) {
	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return errors.New("boom")
		}),
		WithDeferredInitStopOnFailure(false),
		WithOnReady(func(ctx context.Context, app *Server) error {
			app.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ready"))
			})
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	srv.lifecycleCtx, srv.lifecycleCancel = context.WithCancel(context.Background())
	srv.startDeferredInit(nil)
	time.Sleep(50 * time.Millisecond)

	rec := httptest.NewRecorder()
	srv.bootstrapReadinessHandler(srv.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before manual completion, got %d", rec.Code)
	}

	if err := srv.CompleteDeferredInit(context.Background(), nil); err != nil {
		t.Fatalf("manual completion failed: %v", err)
	}

	rec = httptest.NewRecorder()
	srv.bootstrapReadinessHandler(srv.Handler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after manual completion, got %d", rec.Code)
	}
}

func TestCompleteDeferredInitRecordsError(t *testing.T) {
	srv, err := NewServer(
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	manualErr := errors.New("still broken")
	if err := srv.CompleteDeferredInit(context.Background(), manualErr); err == nil {
		t.Fatalf("expected manual completion to report error")
	}

	if err := srv.getDeferredInitError(); err == nil || !strings.Contains(err.Error(), "still broken") {
		t.Fatalf("expected stored error, got %v", err)
	}

	if srv.isReady.Load() {
		t.Fatal("server should remain unready when manual completion fails")
	}
}

func TestRunHonorsDeferredInitHandler(t *testing.T) {
	release := make(chan struct{})

	srv, err := NewServer(
		WithAddr(":0"),
		WithSuppressBanner(true),
		WithDeferredInit(func(ctx context.Context, app *Server) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
				return nil
			}
		}),
		WithOnReady(func(ctx context.Context, app *Server) error {
			app.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ready"))
			})
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	defer srv.stopCleanup()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run()
	}()

	for !srv.isRunning.Load() {
		time.Sleep(10 * time.Millisecond)
	}

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before deferred init completion, got %d", rec.Code)
	}

	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for !srv.isReady.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !srv.isReady.Load() {
		t.Fatal("server did not transition to ready state")
	}

	rec = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after deferred init, got %d", rec.Code)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}
