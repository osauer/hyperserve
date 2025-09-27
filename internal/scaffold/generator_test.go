package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateCreatesProject(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "sample-service")

	repoRoot := repoRoot(t)

	path, err := Generate(Options{
		Module:       "github.com/example/sample-service",
		OutputDir:    dest,
		WithMCP:      true,
		LocalReplace: repoRoot,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if path != dest {
		t.Fatalf("expected output path %q, got %q", dest, path)
	}

	assertExists(t, dest, "go.mod")
	assertExists(t, dest, "cmd/server/main.go")
	assertExists(t, dest, "internal/app/server.go")
	assertExists(t, dest, "configs/default.json")

	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	content := string(gomod)
	if !strings.Contains(content, "module github.com/example/sample-service") {
		t.Fatalf("go.mod missing module declaration: %s", content)
	}
	if !strings.Contains(content, "replace github.com/osauer/hyperserve =>") {
		t.Fatalf("go.mod missing replace directive: %s", content)
	}

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test failed: %v\n%s", err, output)
	}
}

func TestGenerateFailsOnNonEmptyDir(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "occupied")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if _, err := Generate(Options{
		Module:    "github.com/example/occupied",
		OutputDir: dest,
		WithMCP:   true,
	}); err == nil {
		t.Fatalf("expected error for non-empty directory")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate caller")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

func assertExists(t *testing.T, base string, relative string) {
	t.Helper()
	path := filepath.Join(base, relative)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
