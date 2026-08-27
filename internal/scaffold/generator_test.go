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
	assertExists(t, dest, "go.sum")
	assertExists(t, dest, "cmd/server/main.go")
	assertExists(t, dest, "internal/app/server.go")
	assertExists(t, dest, "internal/app/server_test.go")
	assertExists(t, dest, "internal/app/config_test.go")
	assertExists(t, dest, "configs/default.json")
	assertExists(t, dest, "Dockerfile")

	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	content := string(gomod)
	if !strings.Contains(content, "module github.com/example/sample-service") {
		t.Fatalf("go.mod missing module declaration: %s", content)
	}
	if !strings.Contains(content, "github.com/osauer/hyperserve/v2 "+latestStableVersion) {
		t.Fatalf("go.mod missing hyperserve requirement: %s", content)
	}
	if !strings.Contains(content, "replace github.com/osauer/hyperserve/v2 =>") {
		t.Fatalf("go.mod missing replace directive: %s", content)
	}
	if !strings.Contains(content, "go 1.27") {
		t.Fatalf("go.mod missing Go 1.27 floor: %s", content)
	}
	if !strings.Contains(content, "golang.org/x/time v0.15.0 // indirect") {
		t.Fatalf("go.mod does not record HyperServe's limiter dependency as indirect: %s", content)
	}

	goSum, err := os.ReadFile(filepath.Join(dest, "go.sum"))
	if err != nil {
		t.Fatalf("read go.sum: %v", err)
	}
	if !strings.Contains(string(goSum), "golang.org/x/time v0.15.0") {
		t.Fatalf("go.sum missing runtime dependency checksum: %s", goSum)
	}

	serverSource, err := os.ReadFile(filepath.Join(dest, "internal/app/server.go"))
	if err != nil {
		t.Fatalf("read generated server: %v", err)
	}
	serverContent := string(serverSource)
	if strings.Contains(serverContent, "RequestLoggerMiddleware") {
		t.Fatal("generated server registers HyperServe's default request logger twice")
	}
	if !strings.Contains(serverContent, "app.Use(hyperserve.HeadersMiddleware(app.Options()))") {
		t.Fatal("generated server does not apply headers to the global middleware scope")
	}
	for _, required := range []string{
		`"github.com/osauer/hyperserve/v2"`,
		`"github.com/osauer/hyperserve/v2/ratelimit"`,
		"app, err := hyperserve.New(opts...)",
		"gate, err := ratelimit.New(ratelimit.Config{",
		"RequestsPerSecond: float64(cfg.RateLimit)",
		"Burst:             cfg.RateBurst",
		`app.UsePrefix("/api", gate)`,
	} {
		if !strings.Contains(serverContent, required) {
			t.Errorf("generated server missing %q", required)
		}
	}
	for _, retired := range []string{
		"github.com/osauer/hyperserve/v2/pkg/",
		"golang.org/x/time/rate",
		"server.NewServer",
		"WithRateLimit",
		"RateLimitMiddleware",
	} {
		if strings.Contains(serverContent, retired) {
			t.Errorf("generated server contains retired surface %q", retired)
		}
	}
	if strings.Contains(serverContent, "WithMCPBuiltin") {
		t.Fatal("generated server enables MCP capabilities without an authorization policy")
	}

	configSource, err := os.ReadFile(filepath.Join(dest, "internal/app/config.go"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if !strings.Contains(string(configSource), `os.Getenv("HS_RATE_BURST")`) {
		t.Fatal("generated config does not read canonical HS_RATE_BURST")
	}
	if !strings.Contains(string(configSource), `os.LookupEnv("HS_BURST_LIMIT")`) {
		t.Fatal("generated config does not reject presence of retired HS_BURST_LIMIT")
	}
	if strings.Contains(string(configSource), `os.Getenv("HS_BURST_LIMIT")`) {
		t.Fatal("generated config checks retired HS_BURST_LIMIT by value instead of presence")
	}
	for _, required := range []string{"HS_BURST_LIMIT is retired", "use HS_RATE_BURST"} {
		if !strings.Contains(string(configSource), required) {
			t.Fatalf("generated config's retired-variable error does not contain %q", required)
		}
	}

	readme, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil {
		t.Fatalf("read generated README: %v", err)
	}
	if !strings.Contains(string(readme), "`HS_RATE_BURST=200`") {
		t.Fatal("generated README does not document canonical HS_RATE_BURST")
	}
	if !strings.Contains(string(readme), "`HS_BURST_LIMIT` is retired") {
		t.Fatal("generated README does not explain that retired HS_BURST_LIMIT fails startup")
	}

	dockerfile, err := os.ReadFile(filepath.Join(dest, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(dockerfile), "FROM golang:1.27 AS builder") {
		t.Fatalf("Dockerfile missing Go 1.27 builder: %s", dockerfile)
	}

	cmd := exec.Command("go", "test", "-mod=readonly", "./...")
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
