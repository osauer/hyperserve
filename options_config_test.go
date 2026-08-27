package hyperserve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileCanOverrideDefaultsWithZeroValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"read_timeout": 0,
		"stop_on_deferred_init_failure": false,
		"health_addr": ":9091"
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	srv, err := New(WithConfigFile(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	opts := srv.options
	if opts.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want explicit zero from config", opts.ReadTimeout)
	}
	if opts.StopOnDeferredInitFailure {
		t.Fatal("StopOnDeferredInitFailure = true, want explicit false from config")
	}
	if opts.HealthAddr != ":9091" {
		t.Fatalf("HealthAddr = %q, want :9091", opts.HealthAddr)
	}
}

func TestEnvironmentBindings(t *testing.T) {
	t.Setenv(paramServerPort, "9090")
	t.Setenv(paramServerHeader, "environment-service")
	t.Setenv(paramStartupBanner, "true")

	srv, err := New(WithEnvironment())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	opts := srv.options
	if opts.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", opts.Addr)
	}
	if opts.ServerHeader != "environment-service" {
		t.Fatalf("ServerHeader = %q, want environment-service", opts.ServerHeader)
	}
	if !opts.StartupBanner {
		t.Fatal("HS_STARTUP_BANNER did not enable the startup banner")
	}
}

func TestConfigurationPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":9091"
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tests := []struct {
		name     string
		envPort  string
		options  []Option
		wantAddr string
	}{
		{
			name:     "defaults",
			wantAddr: ":8080",
		},
		{
			name:     "file overrides defaults",
			options:  []Option{WithConfigFile(path)},
			wantAddr: ":9091",
		},
		{
			name:     "unset environment preserves file",
			options:  []Option{WithConfigFile(path), WithEnvironment()},
			wantAddr: ":9091",
		},
		{
			name:     "environment overrides file",
			envPort:  "9092",
			options:  []Option{WithConfigFile(path), WithEnvironment()},
			wantAddr: ":9092",
		},
		{
			name:    "functional options override environment and file",
			envPort: "9092",
			options: []Option{
				WithConfigFile(path),
				WithEnvironment(),
				WithAddr(":9093"),
			},
			wantAddr: ":9093",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(paramServerPort, tt.envPort)

			srv, err := New(tt.options...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() {
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Errorf("Stop: %v", err)
				}
			})

			if srv.options.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", srv.options.Addr, tt.wantAddr)
			}
		})
	}
}

func TestRetiredRateLimitConfigKeysFailVisibly(t *testing.T) {
	for _, key := range []string{"rate_limit", "burst"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server.json")
			body := []byte(`{"` + key + `": 0}`)
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := New(WithConfigFile(path))
			if err == nil {
				t.Fatalf("retired config key %q succeeded", key)
			}
			if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "ratelimit.New") {
				t.Fatalf("error = %q, want key and ratelimit.New migration direction", err)
			}
		})
	}
}

func TestRetiredRateLimitEnvironmentFailsVisiblyWhenPresent(t *testing.T) {
	for _, name := range []string{paramRateLimit, paramBurstLimit} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "")
			_, err := New(WithEnvironment())
			if err == nil {
				t.Fatalf("retired environment variable %s succeeded", name)
			}
			if !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "ratelimit.New") {
				t.Fatalf("error = %q, want variable and ratelimit.New migration direction", err)
			}
		})
	}
}

func TestNewIgnoresAmbientConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":9091",
		"mcp_enabled": true,
		"mcp_dev": true,
		"server_header": "ambient-file",
		"startup_banner": true
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HS_CONFIG_PATH", path)
	t.Setenv(paramServerPort, "9092")
	t.Setenv(paramMCPEnabled, "true")
	t.Setenv(paramMCPDev, "true")
	t.Setenv(paramServerHeader, "ambient-environment")
	t.Setenv(paramStartupBanner, "true")
	t.Setenv(paramRateLimit, "100")
	t.Setenv(paramBurstLimit, "200")

	srv, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	if srv.options.Addr != ":8080" {
		t.Fatalf("Addr = %q, want deterministic default :8080", srv.options.Addr)
	}
	if srv.options.MCPEnabled || srv.MCPEnabled() {
		t.Fatal("bare New enabled MCP from ambient configuration")
	}
	if srv.options.ServerHeader != "" {
		t.Fatalf("ServerHeader = %q, want omitted by default", srv.options.ServerHeader)
	}
	if srv.options.StartupBanner {
		t.Fatal("startup banner enabled by ambient configuration")
	}
}

func TestDefaultOptionsDisableFilesystemRoots(t *testing.T) {
	options := DefaultOptions()
	if options.StaticDir != "" || options.TemplateDir != "" {
		t.Fatalf("default filesystem roots: static=%q template=%q, want both empty", options.StaticDir, options.TemplateDir)
	}

	workingDir := t.TempDir()
	for _, dir := range []string{"static", "template"} {
		if err := os.Mkdir(filepath.Join(workingDir, dir), 0o755); err != nil {
			t.Fatalf("create ambient %s directory: %v", dir, err)
		}
	}
	t.Chdir(workingDir)

	srv, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	if srv.options.StaticDir != "" || srv.options.TemplateDir != "" {
		t.Fatalf("server filesystem roots: static=%q template=%q, want both empty", srv.options.StaticDir, srv.options.TemplateDir)
	}
	if srv.staticRoot != nil || srv.templateRoot != nil {
		t.Fatalf("bare New opened ambient roots: static=%v template=%v", srv.staticRoot != nil, srv.templateRoot != nil)
	}
}

func TestWithOptionsDefensivelyClones(t *testing.T) {
	options := DefaultOptions()
	options.CORS = &CORSOptions{AllowedOrigins: []string{"https://before.example"}}
	options.OnShutdownHooks = []func(context.Context) error{func(context.Context) error { return nil }}

	srv, err := New(WithOptions(options))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	options.CORS.AllowedOrigins[0] = "https://after.example"
	options.OnShutdownHooks[0] = nil
	if got := srv.options.CORS.AllowedOrigins[0]; got != "https://before.example" {
		t.Fatalf("CORS option aliased caller memory: %q", got)
	}
	if srv.options.OnShutdownHooks[0] == nil {
		t.Fatal("shutdown hook slice aliased caller memory")
	}
}

func TestWithConfigFileRequiresReadableJSON(t *testing.T) {
	if _, err := New(WithConfigFile(filepath.Join(t.TempDir(), "missing.json"))); err == nil {
		t.Fatal("missing explicit config file succeeded")
	}
}
