package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileCanOverrideDefaultsWithZeroValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"burst": 0,
		"stop_on_deferred_init_failure": false,
		"health_addr": ":9091"
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	srv, err := NewServer(WithConfigFile(path))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	opts := srv.Options
	if opts.Burst != 0 {
		t.Fatalf("Burst = %d, want explicit zero from config", opts.Burst)
	}
	if opts.StopOnDeferredInitFailure {
		t.Fatal("StopOnDeferredInitFailure = true, want explicit false from config")
	}
	if opts.HealthAddr != ":9091" {
		t.Fatalf("HealthAddr = %q, want :9091", opts.HealthAddr)
	}
}

func TestEnvironmentPortAndRateLimitAliases(t *testing.T) {
	t.Setenv(paramServerPort, "9090")
	t.Setenv(paramRateLimit, "25.5")
	t.Setenv(paramBurstLimit, "50")
	t.Setenv(paramServerHeader, "environment-service")
	t.Setenv(paramStartupBanner, "true")

	srv, err := NewServer(WithEnvironment())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	opts := srv.Options
	if opts.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", opts.Addr)
	}
	if opts.RateLimit != 25.5 {
		t.Fatalf("RateLimit = %v, want 25.5", opts.RateLimit)
	}
	if opts.Burst != 50 {
		t.Fatalf("Burst = %d, want 50", opts.Burst)
	}
	if opts.ServerHeader != "environment-service" {
		t.Fatalf("ServerHeader = %q, want environment-service", opts.ServerHeader)
	}
	if opts.SuppressBanner {
		t.Fatal("HS_STARTUP_BANNER did not enable the startup banner")
	}
}

func TestConfigurationPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":9091",
		"rate_limit": 10,
		"burst": 20
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tests := []struct {
		name      string
		envPort   string
		envRate   string
		envBurst  string
		options   []ServerOptionFunc
		wantAddr  string
		wantRate  RateLimit
		wantBurst int
	}{
		{
			name:      "defaults",
			wantAddr:  ":8080",
			wantRate:  1,
			wantBurst: 10,
		},
		{
			name:      "file overrides defaults",
			options:   []ServerOptionFunc{WithConfigFile(path)},
			wantAddr:  ":9091",
			wantRate:  10,
			wantBurst: 20,
		},
		{
			name:      "unset environment preserves file",
			options:   []ServerOptionFunc{WithConfigFile(path), WithEnvironment()},
			wantAddr:  ":9091",
			wantRate:  10,
			wantBurst: 20,
		},
		{
			name:      "environment overrides file",
			envPort:   "9092",
			envRate:   "30",
			envBurst:  "40",
			options:   []ServerOptionFunc{WithConfigFile(path), WithEnvironment()},
			wantAddr:  ":9092",
			wantRate:  30,
			wantBurst: 40,
		},
		{
			name:      "explicit environment zero overrides file",
			envPort:   "9092",
			envRate:   "0",
			envBurst:  "0",
			options:   []ServerOptionFunc{WithConfigFile(path), WithEnvironment()},
			wantAddr:  ":9092",
			wantRate:  0,
			wantBurst: 0,
		},
		{
			name:     "functional options override environment and file",
			envPort:  "9092",
			envRate:  "30",
			envBurst: "40",
			options: []ServerOptionFunc{
				WithConfigFile(path),
				WithEnvironment(),
				WithAddr(":9093"),
				WithRateLimit(50, 60),
			},
			wantAddr:  ":9093",
			wantRate:  50,
			wantBurst: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(paramServerPort, tt.envPort)
			t.Setenv(paramRateLimit, tt.envRate)
			t.Setenv(paramBurstLimit, tt.envBurst)

			srv, err := NewServer(tt.options...)
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}
			t.Cleanup(func() {
				if err := srv.Stop(); err != nil {
					t.Errorf("Stop: %v", err)
				}
			})

			if srv.Options.Addr != tt.wantAddr {
				t.Errorf("Addr = %q, want %q", srv.Options.Addr, tt.wantAddr)
			}
			if srv.Options.RateLimit != tt.wantRate {
				t.Errorf("RateLimit = %v, want %v", srv.Options.RateLimit, tt.wantRate)
			}
			if srv.Options.Burst != tt.wantBurst {
				t.Errorf("Burst = %d, want %d", srv.Options.Burst, tt.wantBurst)
			}
		})
	}
}

func TestNewServerIgnoresAmbientConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":9091",
		"mcp_enabled": true,
		"mcp_dev": true,
		"server_header": "ambient-file",
		"suppress_banner": false
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(paramConfigPath, path)
	t.Setenv(paramServerPort, "9092")
	t.Setenv(paramMCPEnabled, "true")
	t.Setenv(paramMCPDev, "true")
	t.Setenv(paramServerHeader, "ambient-environment")
	t.Setenv(paramStartupBanner, "true")

	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	if srv.Options.Addr != ":8080" {
		t.Fatalf("Addr = %q, want deterministic default :8080", srv.Options.Addr)
	}
	if srv.Options.MCPEnabled || srv.MCPEnabled() {
		t.Fatal("bare NewServer enabled MCP from ambient configuration")
	}
	if srv.Options.ServerHeader != "" {
		t.Fatalf("ServerHeader = %q, want omitted by default", srv.Options.ServerHeader)
	}
	if !srv.Options.SuppressBanner {
		t.Fatal("startup banner enabled by ambient configuration")
	}
}

func TestDefaultServerOptionsDisableFilesystemRoots(t *testing.T) {
	options := DefaultServerOptions()
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

	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	if srv.Options.StaticDir != "" || srv.Options.TemplateDir != "" {
		t.Fatalf("server filesystem roots: static=%q template=%q, want both empty", srv.Options.StaticDir, srv.Options.TemplateDir)
	}
	if srv.staticRoot != nil || srv.templateRoot != nil {
		t.Fatalf("bare NewServer opened ambient roots: static=%v template=%v", srv.staticRoot != nil, srv.templateRoot != nil)
	}
}

func TestWithOptionsDefensivelyClones(t *testing.T) {
	options := DefaultServerOptions()
	options.CORS = &CORSOptions{AllowedOrigins: []string{"https://before.example"}}
	options.OnShutdownHooks = []func(context.Context) error{func(context.Context) error { return nil }}

	srv, err := NewServer(WithOptions(options))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	options.CORS.AllowedOrigins[0] = "https://after.example"
	options.OnShutdownHooks[0] = nil
	if got := srv.Options.CORS.AllowedOrigins[0]; got != "https://before.example" {
		t.Fatalf("CORS option aliased caller memory: %q", got)
	}
	if srv.Options.OnShutdownHooks[0] == nil {
		t.Fatal("shutdown hook slice aliased caller memory")
	}
}

func TestWithConfigFileRequiresReadableJSON(t *testing.T) {
	if _, err := NewServer(WithConfigFile(filepath.Join(t.TempDir(), "missing.json"))); err == nil {
		t.Fatal("missing explicit config file succeeded")
	}
}
