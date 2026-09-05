package websocket

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedirectCredentialsStayStripped(t *testing.T) {
	for _, target := range []string{"http://example.test:9002/redirect", "http://child.example.test:9001/redirect", "http://other.test/redirect"} {
		t.Run(target, func(t *testing.T) {
			var requests []*http.Request
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.Clone(r.Context()))
				if len(requests) == 1 {
					return redirectResponse(r, target), nil
				}
				if len(requests) == 2 {
					return redirectResponse(r, "/target"), nil
				}
				return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
			}), CheckRedirect: func(r *http.Request, _ []*http.Request) error {
				// Callback mutations must not undo origin-boundary protection.
				r.Header.Set("Authorization", "Bearer callback")
				return nil
			}}
			_, response, err := Dial(t.Context(), "ws://example.test:9001/start", &DialOptions{HTTPClient: client,
				HTTPHeader: http.Header{"Authorization": {"Bearer test"}, "Cookie": {"session=test"}, "Proxy-Authorization": {"Basic test"}},
			})
			if response != nil {
				response.Body.Close()
			}
			if err == nil || len(requests) != 3 {
				t.Fatalf("requests=%d error=%v", len(requests), err)
			}
			for _, request := range requests[1:] {
				for _, header := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
					if got := request.Header.Get(header); got != "" {
						t.Errorf("%s leaked %s", request.URL, header)
					}
				}
			}
		})
	}
}

func TestReadCancellationInterruptsControlReply(t *testing.T) {
	for _, frame := range [][]byte{{0x89, 0x80, 0, 0, 0, 0}, {0x88, 0x82, 0, 0, 0, 0, 0x03, 0xe8}} {
		for _, deadline := range []bool{false, true} {
			local, peer := net.Pipe()
			conn := newConn(newLowConn(local, bufio.NewReadWriter(bufio.NewReader(local), bufio.NewWriter(local)), true, 1024))
			ctx, cancel := context.WithCancel(t.Context())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
			}
			done := make(chan error, 1)
			go func() { _, _, err := conn.Read(ctx); done <- err }()
			if _, err := peer.Write(frame); err != nil {
				t.Fatal(err)
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("Read: %v", err)
				}
			case <-time.After(time.Second):
				t.Error("Read remained blocked on a control reply")
			}
			cancel()
			local.Close()
			peer.Close()
		}
	}
}

type handshakePipeWriter struct {
	conn   net.Conn
	header http.Header
}

func (w handshakePipeWriter) Header() http.Header         { return w.header }
func (w handshakePipeWriter) Write(p []byte) (int, error) { return w.conn.Write(p) }
func (w handshakePipeWriter) WriteHeader(int)             {}
func (w handshakePipeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

func TestUpgradeTimeoutBoundsResponseWrite(t *testing.T) {
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	r := httptest.NewRequest("GET", "http://localhost/ws", nil)
	r.Header = http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="}, "Sec-Websocket-Version": {"13"}}
	r.Header.Set("Origin", "http://localhost")
	done := make(chan error, 1)
	go func() {
		_, err := (&Upgrader{HandshakeTimeout: 20 * time.Millisecond}).Upgrade(handshakePipeWriter{local, make(http.Header)}, r, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if timeout, ok := errors.AsType[net.Error](err); !ok || !timeout.Timeout() {
			t.Fatalf("expected a write timeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handshake timeout did not interrupt response write")
	}
}

func TestControlRepliesAndCloseAreBounded(t *testing.T) {
	for _, mode := range []string{"pong", "close echo", "explicit close"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			local, peer := net.Pipe()
			defer local.Close()
			defer peer.Close()
			conn := newConn(newLowConn(local, bufio.NewReadWriter(bufio.NewReader(local), bufio.NewWriter(local)), true, 1024))
			done := make(chan struct{})
			if mode == "explicit close" {
				go func() { _ = conn.Close(); close(done) }()
			} else {
				go func() { _, _, _ = conn.ReadMessage(); close(done) }()
				frame := []byte{0x89, 0x80, 0, 0, 0, 0}
				if mode == "close echo" {
					frame[0] = 0x88
				}
				if _, err := peer.Write(frame); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
			case <-time.After(7 * time.Second):
				t.Fatal("blocked operation did not close the connection")
			}
		})
	}
}
