package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOptionsReturnsIndependentSnapshot(t *testing.T) {
	shutdown := func(context.Context) error { return nil }
	srv, err := NewServer(
		WithCORS(&CORSOptions{AllowedOrigins: []string{"https://example.test"}}),
		WithOnShutdown(shutdown),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()

	first := srv.Options()
	first.CORS.AllowedOrigins[0] = "https://changed.test"
	first.OnShutdownHooks[0] = nil

	second := srv.Options()
	if got := second.CORS.AllowedOrigins[0]; got != "https://example.test" {
		t.Fatalf("CORS origin = %q; Options leaked mutable state", got)
	}
	if second.OnShutdownHooks[0] == nil {
		t.Fatal("shutdown hook snapshot leaked mutable state")
	}
}

func TestWithLoggerIsScopedToOneServer(t *testing.T) {
	processDefault := slog.Default()
	var output bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&output, nil))

	srv, err := NewServer(WithLogger(custom))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()
	if srv.logger != custom {
		t.Fatal("server did not retain the supplied logger")
	}
	if slog.Default() != processDefault {
		t.Fatal("WithLogger changed slog's process-wide default")
	}

	srv.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !bytes.Contains(output.Bytes(), []byte("Request completed")) {
		t.Fatalf("custom logger output = %q", output.String())
	}
}

func TestWithLoggerRejectsNil(t *testing.T) {
	if _, err := NewServer(WithLogger(nil)); err == nil {
		t.Fatal("NewServer accepted a nil logger")
	}
}

func TestWithTimeoutsPreservesExplicitZero(t *testing.T) {
	srv, err := NewServer(WithTimeouts(0, 0, 0))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()

	options := srv.Options()
	if options.ReadTimeout != 0 || options.WriteTimeout != 0 || options.IdleTimeout != 0 {
		t.Fatalf("timeouts = (%s, %s, %s), want explicit zero values",
			options.ReadTimeout, options.WriteTimeout, options.IdleTimeout)
	}
}

func TestUsePrefixHonorsPathBoundary(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()

	srv.UsePrefix("/api", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	})
	srv.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "/api", want: true},
		{path: "/api/users", want: true},
		{path: "/apiv2", want: false},
	} {
		recorder := httptest.NewRecorder()
		srv.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		got := recorder.Header().Get("X-API-Middleware") == "true"
		if got != test.want {
			t.Errorf("path %q: middleware applied = %v, want %v", test.path, got, test.want)
		}
	}
}

func TestMiddlewareRoutesReturnsIndependentStacks(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()

	middleware := func(next http.Handler) http.Handler { return next }
	srv.UsePrefix("/api", middleware)
	first := srv.MiddlewareRoutes()
	first["/api"][0] = nil
	second := srv.MiddlewareRoutes()
	if second["/api"][0] == nil {
		t.Fatal("MiddlewareRoutes leaked a mutable stack")
	}
}

func TestShutdownRejectsNilContext(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()

	//lint:ignore SA1012 Passing nil is intentional: this exercises the public guard.
	if err := srv.Shutdown(nil); err == nil {
		t.Fatal("Shutdown accepted a nil context")
	}
}

func TestLogLevelDoesNotChangeProcessDefault(t *testing.T) {
	processDefault := slog.Default()
	srv, err := NewServer(WithLogLevel("ERROR"))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.stopCleanup()
	if slog.Default() != processDefault {
		t.Fatal("WithLogLevel changed slog's process-wide default")
	}
	if srv.Options().LogLevel != "ERROR" {
		t.Fatalf("LogLevel = %q, want ERROR", srv.Options().LogLevel)
	}
}
