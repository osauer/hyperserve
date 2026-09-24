package websocket

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rejectHijackWriter struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (w *rejectHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return nil, nil, errors.New("unexpected hijack")
}

func headerHandshakeRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://localhost/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	r.Header.Set("Sec-WebSocket-Version", "13")
	return r
}

func TestHandshakeRejectsInvalidHeadersBeforeHijack(t *testing.T) {
	for _, tt := range []struct{ name, key, value string }{
		{"CRLF injection", "X-Trace", "ok\r\nX-Injected: yes"},
		{"carriage return", "X-Trace", "a\rb"},
		{"line feed", "X-Trace", "a\nb"},
		{"NUL", "X-Trace", "a\x00b"},
		{"control", "X-Trace", "a\x1fb"},
		{"DEL", "X-Trace", "a\x7fb"},
		{"empty name", "", "ok"},
		{"name injection", "X-Trace\r\nX-Injected", "ok"},
		{"name space", "X Trace", "ok"},
		{"name colon", "X:Trace", "ok"},
		{"name non ASCII", "X-Tracé", "ok"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := &rejectHijackWriter{ResponseRecorder: httptest.NewRecorder()}
			_, _, err := PerformHandshake(w, headerHandshakeRequest(), &HandshakeOptions{
				ResponseHeader: http.Header{tt.key: {tt.value}},
			})
			if err == nil || w.hijacked {
				t.Fatalf("invalid header reached hijack: error=%v hijacked=%v", err, w.hijacked)
			}
			// The caller can still send an ordinary HTTP error response.
			http.Error(w, "invalid response header", http.StatusInternalServerError)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("response already committed: %d", w.Code)
			}
		})
	}
}

func TestHandshakePreservesValidResponseHeaders(t *testing.T) {
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	if err := peer.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	wire := make(chan []byte, 1)
	go func() { data, _ := io.ReadAll(peer); wire <- data }()
	_, _, err := PerformHandshake(handshakePipeWriter{local, make(http.Header)}, headerHandshakeRequest(), &HandshakeOptions{
		ResponseHeader: http.Header{"X-Trace": {"first", "second\tvalue"}, "X-Text": {"café"}},
	})
	local.Close()
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(strings.NewReader(string(<-wire))), headerHandshakeRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if got := response.Header.Values("X-Trace"); len(got) != 2 || got[0] != "first" || got[1] != "second\tvalue" {
		t.Fatalf("header values changed: %q", got)
	}
	if got := response.Header.Get("X-Text"); got != "café" {
		t.Fatalf("non ASCII value changed: %q", got)
	}
}
