package hyperserve

import (
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTLSModeNegotiation(t *testing.T) {
	app, err := New(WithFIPSMode())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.TLS = app.tlsConfig()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	for _, tt := range []struct {
		name            string
		version, cipher uint16
		success         bool
	}{
		{"TLS12 AES", tls.VersionTLS12, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, true},
		{"TLS12 ChaCha20", tls.VersionTLS12, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, false},
		{"TLS13 runtime policy", tls.VersionTLS13, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			client.MinVersion, client.MaxVersion = tt.version, tt.version
			client.CipherSuites = []uint16{tt.cipher}
			conn, err := tls.Dial("tcp", server.Listener.Addr().String(), client)
			if (err == nil) != tt.success {
				t.Fatalf("handshake: %v", err)
			}
			if err != nil {
				return
			}
			defer conn.Close()
			state := conn.ConnectionState()
			if state.Version != tt.version || (tt.cipher != 0 && state.CipherSuite != tt.cipher) {
				t.Fatalf("negotiated: %+v", state)
			}
			t.Logf("negotiated %s", tls.CipherSuiteName(state.CipherSuite))
		})
	}
}
