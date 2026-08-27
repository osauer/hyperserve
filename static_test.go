package hyperserve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHandleStaticFailsClosedWhenRootOpenFails(t *testing.T) {
	srv, err := New(WithStaticDir(filepath.Join(t.TempDir(), "missing")))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	if err := srv.HandleStatic("/static/"); err == nil {
		t.Fatal("HandleStatic succeeded with a missing root")
	}
	assertStaticRouteClosed(t, srv, "/static/", "/static/secret.txt")
}

func TestWithStaticDirServesExplicitRoot(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "asset.txt"), []byte("explicit root"), 0o600); err != nil {
		t.Fatalf("write static asset: %v", err)
	}

	srv, err := New(WithStaticDir(staticDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	if err := srv.HandleStatic("/static/"); err != nil {
		t.Fatalf("HandleStatic: %v", err)
	}

	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/static/asset.txt", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "explicit root" {
		t.Fatalf("static response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandleStaticDoesNotServeWorkingDirectory(t *testing.T) {
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	if err := os.WriteFile(filepath.Join(workingDir, "secret.txt"), []byte("must not escape"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	srv, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	srv.options.StaticDir = ""

	if err := srv.HandleStatic("/static/"); err == nil {
		t.Fatal("HandleStatic succeeded without a configured root")
	}
	assertStaticRouteClosed(t, srv, "/static/", "/static/secret.txt")
}

func assertStaticRouteClosed(t *testing.T, srv *Server, pattern, requestPath string) {
	t.Helper()
	if slices.Contains(srv.RegisteredRoutes(), pattern) {
		t.Fatalf("failed static route %q was registered", pattern)
	}
	recorder := httptest.NewRecorder()
	srv.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("failed static route status = %d, want 404", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "must not escape") {
		t.Fatal("failed static route served a file from the working directory")
	}
}
