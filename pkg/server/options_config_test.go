package server

import (
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
	t.Setenv(paramConfigPath, path)

	opts := NewServerOptions()
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

	opts := NewServerOptions()
	if opts.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", opts.Addr)
	}
	if opts.RateLimit != 25.5 {
		t.Fatalf("RateLimit = %v, want 25.5", opts.RateLimit)
	}
	if opts.Burst != 50 {
		t.Fatalf("Burst = %d, want 50", opts.Burst)
	}
}

func TestFunctionalOptionsOverrideEnvironmentAndConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{
		"addr": ":9091",
		"rate_limit": 10,
		"burst": 20
	}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(paramConfigPath, path)
	t.Setenv(paramServerPort, "9092")
	t.Setenv(paramRateLimit, "30")
	t.Setenv(paramBurstLimit, "40")

	srv, err := NewServer(
		WithAddr(":9093"),
		WithRateLimit(50, 60),
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() {
		if err := srv.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	if srv.Options.Addr != ":9093" {
		t.Fatalf("Addr = %q, want functional option :9093", srv.Options.Addr)
	}
	if srv.Options.RateLimit != 50 {
		t.Fatalf("RateLimit = %v, want functional option 50", srv.Options.RateLimit)
	}
	if srv.Options.Burst != 60 {
		t.Fatalf("Burst = %d, want functional option 60", srv.Options.Burst)
	}
}
