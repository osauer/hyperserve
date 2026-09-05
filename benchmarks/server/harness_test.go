package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHarnessRequiresOwnListener(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires bash")
	}
	for _, occupied := range []bool{true, false} {
		var addr string
		if occupied {
			unrelated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			defer unrelated.Close()
			addr = unrelated.Listener.Addr().String()
		} else {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr = listener.Addr().String()
			listener.Close()
		}
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			t.Fatal(err)
		}
		results := t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "run_benchmarks.sh")
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "BENCH_PORT="+port, "BENCH_DURATION=100ms", "BENCH_CONNECTIONS=1", "BENCH_THREADS=1", "BENCH_RESULTS_DIR="+results)
		output, err := cmd.CombinedOutput()
		if (err != nil) != occupied {
			t.Fatalf("occupied=%t error=%v\n%s", occupied, err, output)
		}
		for _, profile := range []string{"minimal", "middleware"} {
			_, statErr := os.Stat(filepath.Join(results, profile+".txt"))
			if occupied && !os.IsNotExist(statErr) {
				t.Fatal("failed fixture published a success result")
			}
			if !occupied && statErr != nil {
				t.Fatal(statErr)
			}
		}
		if !occupied && !strings.Contains(string(output), "Benchmark results:") {
			t.Fatalf("missing completion: %s", output)
		}
		if !occupied {
			before := map[string]string{}
			for _, name := range []string{"metadata.txt", "server.log", "minimal.txt", "middleware.txt"} {
				data, err := os.ReadFile(filepath.Join(results, name))
				if err != nil {
					t.Fatal(err)
				}
				before[name] = string(data)
			}
			rerun := exec.CommandContext(ctx, "bash", "run_benchmarks.sh")
			rerun.Dir = ".."
			rerun.Env = cmd.Env
			if out, err := rerun.CombinedOutput(); err == nil || !strings.Contains(string(out), "must be empty") {
				t.Fatalf("rerun = %s, %v", out, err)
			}
			for name, want := range before {
				got, err := os.ReadFile(filepath.Join(results, name))
				if err != nil || string(got) != want {
					t.Fatalf("rerun changed %s", name)
				}
			}
		}
	}
}
