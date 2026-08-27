package hyperserve

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSlowlorisProtection tests the ReadHeaderTimeout protection against Slowloris attacks
func TestSlowlorisProtection(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping: unable to reserve a loopback address (%v)", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}

	srv, err := New(WithAddr(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const headerTimeout = 50 * time.Millisecond
	srv.options.ReadHeaderTimeout = headerTimeout

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.Run(ctx)
	}()

	startupDeadline := time.After(time.Second)
	for !srv.isRunning.Load() {
		select {
		case err := <-runErr:
			t.Fatalf("server exited during startup: %v", err)
		case <-startupDeadline:
			t.Fatal("server did not start")
		case <-time.After(time.Millisecond):
		}
	}
	defer func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("Run shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	}()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("connect to server: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + addr + "\r\nX-Slow: unfinished")); err != nil {
		t.Fatalf("write partial request: %v", err)
	}

	started := time.Now()
	_, readErr := io.ReadAll(conn)
	if timeoutErr, ok := readErr.(net.Error); ok && timeoutErr.Timeout() {
		t.Fatalf("connection remained open past the test deadline: %v", readErr)
	}
	if elapsed := time.Since(started); elapsed < headerTimeout/2 {
		t.Fatalf("connection closed after %v, before header timeout %v", elapsed, headerTimeout)
	}
}

// TestHealthServerTimeoutConfiguration tests that health server has proper timeout configuration
func TestHealthServerTimeoutConfiguration(t *testing.T) {
	srv, err := New(
		WithAddr(":0"),
		WithHealthServer(),
		WithTimeouts(10*time.Second, 15*time.Second, 30*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Bind health server to an ephemeral loopback port to avoid sandbox restrictions
	srv.options.HealthAddr = "127.0.0.1:0"

	// Start the server
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Run(context.Background())
	}()

	// Wait for server initialization or failure
	timeout := time.After(5 * time.Second)
waiting:
	for {
		select {
		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed {
				if strings.Contains(err.Error(), "operation not permitted") {
					t.Skipf("skipping: unable to bind in restricted environment (%v)", err)
				}
				t.Fatalf("server failed to start: %v", err)
			}
			break waiting
		case <-timeout:
			t.Fatal("timeout waiting for server to start")
		case <-time.After(5 * time.Millisecond):
			if srv.isRunning.Load() {
				break waiting
			}
		}
	}

	// If server isn't running at this point, skip (likely sandbox restrictions)
	if !srv.isRunning.Load() {
		if err := srv.Shutdown(context.Background()); err != nil && err != http.ErrServerClosed {
			t.Logf("cleanup stop error: %v", err)
		}
		t.Skip("server could not start in this environment")
	}

	// Verify main server timeouts
	if srv.httpServer.ReadTimeout != 10*time.Second {
		t.Errorf("expected ReadTimeout to be 10s, got %v", srv.httpServer.ReadTimeout)
	}
	if srv.httpServer.WriteTimeout != 15*time.Second {
		t.Errorf("expected WriteTimeout to be 15s, got %v", srv.httpServer.WriteTimeout)
	}
	if srv.httpServer.IdleTimeout != 30*time.Second {
		t.Errorf("expected IdleTimeout to be 30s, got %v", srv.httpServer.IdleTimeout)
	}
	if srv.httpServer.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("expected ReadHeaderTimeout to be 10s, got %v", srv.httpServer.ReadHeaderTimeout)
	}

	// Verify health server timeouts
	if srv.healthServer != nil {
		if srv.healthServer.ReadTimeout != 10*time.Second {
			t.Errorf("health server: expected ReadTimeout to be 10s, got %v", srv.healthServer.ReadTimeout)
		}
		if srv.healthServer.WriteTimeout != 15*time.Second {
			t.Errorf("health server: expected WriteTimeout to be 15s, got %v", srv.healthServer.WriteTimeout)
		}
		if srv.healthServer.IdleTimeout != 30*time.Second {
			t.Errorf("health server: expected IdleTimeout to be 30s, got %v", srv.healthServer.IdleTimeout)
		}
		if srv.healthServer.ReadHeaderTimeout != 10*time.Second {
			t.Errorf("health server: expected ReadHeaderTimeout to be 10s, got %v", srv.healthServer.ReadHeaderTimeout)
		}
	}

	if err := srv.Shutdown(context.Background()); err != nil && err != http.ErrServerClosed {
		t.Errorf("failed to stop server: %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected server shutdown error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server shutdown")
	}
}

// TestTLSConfiguration tests that TLS is properly configured with secure defaults
func TestTLSConfiguration(t *testing.T) {
	certFile, keyFile, certificate := writeTestCertificate(t)
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping: unable to reserve a loopback address (%v)", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}

	srv, err := New(WithTLS(certFile, keyFile))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.options.TLSAddr = addr
	srv.HandleFunc("/secure", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- srv.Run(ctx)
	}()

	startupDeadline := time.After(time.Second)
	for !srv.isRunning.Load() {
		select {
		case err := <-runErr:
			t.Fatalf("TLS server exited during startup: %v", err)
		case <-startupDeadline:
			t.Fatal("TLS server did not start")
		case <-time.After(time.Millisecond):
		}
	}
	defer func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("Run shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("TLS server did not stop")
		}
	}()

	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		}},
		Timeout: time.Second,
	}
	resp, err := client.Get("https://" + addr + "/secure")
	if err != nil {
		t.Fatalf("HTTPS request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Fatalf("TLS state = %#v, want TLS 1.2 or newer", resp.TLS)
	}
}

func writeTestCertificate(t *testing.T) (certFile, keyFile string, certificate *x509.Certificate) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "HyperServe test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certificate, err = x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	tempDir := t.TempDir()
	certFile = filepath.Join(tempDir, "server.crt")
	keyFile = filepath.Join(tempDir, "server.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return certFile, keyFile, certificate
}
