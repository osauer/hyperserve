package scaffold

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGeneratedServiceDrainsOnSIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX process signals")
	}
	dest := filepath.Join(t.TempDir(), "service")
	if _, err := Generate(Options{Module: "example.com/drain", OutputDir: dest, LocalReplace: repoRoot(t)}); err != nil {
		t.Fatal(err)
	}
	routes := filepath.Join(dest, "internal/app/routes.go")
	source, err := os.ReadFile(routes)
	if err != nil {
		t.Fatal(err)
	}
	fixture := `
 app.HandleFunc("/review-drain", func(w http.ResponseWriter, r *http.Request) {
   fmt.Fprintln(w, "begin")
   w.(http.Flusher).Flush()
   time.Sleep(300*time.Millisecond)
   fmt.Fprintln(w, "done")
 })
`
	data := strings.Replace(string(source), "func RegisterRoutes(app *hyperserve.Server, cfg Config) {", "func RegisterRoutes(app *hyperserve.Server, cfg Config) {"+fixture, 1)
	if err := os.WriteFile(routes, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	binary := filepath.Join(dest, "server")
	build := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", binary, "./cmd/server")
	build.Dir = dest
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = dest
	cmd.Env = append(os.Environ(), "SERVER_ADDR="+addr, "HEALTH_ADDR=127.0.0.1:0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	client := &http.Client{Timeout: 3 * time.Second}
	var response *http.Response
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); {
		response, err = client.Get("http://" + addr + "/review-drain")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "begin\n" {
		t.Fatalf("begin = %q, %v", line, err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil || string(body) != "done\n" {
		t.Fatalf("drain = %q, %v", body, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit after drain: %v", err)
	}
}
