package websocket

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDialReadWriteAndClose(t *testing.T) {
	t.Parallel()

	serverErr := make(chan error, 1)
	serverProtocol := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer relay-token" {
			serverErr <- fmt.Errorf("Authorization = %q", got)
			return
		}
		upgrader := Upgrader{
			CheckOrigin:     func(*http.Request) bool { return true },
			Subprotocols:    []string{"relay.v1"},
			RequireProtocol: true,
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		serverProtocol <- conn.Subprotocol()

		for range 2 {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				serverErr <- err
				return
			}
			if err := conn.WriteMessage(messageType, payload); err != nil {
				serverErr <- err
				return
			}
		}
		_, _, err = conn.ReadMessage()
		if !IsCloseError(err, CloseGoingAway) {
			serverErr <- fmt.Errorf("close error = %v", err)
			return
		}
		serverErr <- nil
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, resp, err := Dial(ctx, websocketURL(server.URL), &DialOptions{
		HTTPHeader:   http.Header{"Authorization": {"Bearer relay-token"}},
		Subprotocols: []string{"relay.v1"},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := conn.Subprotocol(); got != "relay.v1" {
		t.Fatalf("Subprotocol() = %q", got)
	}
	if got := <-serverProtocol; got != "relay.v1" {
		t.Fatalf("server Subprotocol() = %q", got)
	}

	messages := []struct {
		messageType int
		payload     string
	}{
		{TextMessage, "relay-online"},
		{BinaryMessage, "\x00\x01\x02"},
	}
	for _, message := range messages {
		if err := conn.Write(ctx, message.messageType, []byte(message.payload)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		gotType, got, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if gotType != message.messageType || string(got) != message.payload {
			t.Fatalf("Read() = (%d, %q), want (%d, %q)", gotType, got, message.messageType, message.payload)
		}
	}
	if err := conn.CloseWithStatus(CloseGoingAway, "relay shutdown"); err != nil {
		t.Fatalf("CloseWithStatus() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestDialFollowsRedirectAndStripsCredentials(t *testing.T) {
	t.Parallel()

	authorization := make(chan string, 1)
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/relay", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	conn, _, err := Dial(context.Background(), websocketURL(redirect.URL), &DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer secret"}},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if got := <-authorization; got != "" {
		t.Fatalf("cross-origin Authorization = %q, want empty", got)
	}
}

func TestDialWSS(t *testing.T) {
	t.Parallel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	conn, _, err := Dial(context.Background(), strings.Replace(server.URL, "https://", "wss://", 1), &DialOptions{
		TLSConfig: &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestDialHandshakeFailureReturnsResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "token rejected", http.StatusUnauthorized)
	}))
	defer server.Close()

	conn, resp, err := Dial(context.Background(), websocketURL(server.URL), nil)
	if conn != nil || err == nil {
		t.Fatalf("Dial() = (%v, %v), want nil connection and error", conn, err)
	}
	if !errors.Is(err, ErrBadHandshake) {
		t.Fatalf("error = %v, want ErrBadHandshake", err)
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %#v", resp)
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil || !strings.Contains(string(body), "token rejected") {
		t.Fatalf("body = %q, err = %v", body, readErr)
	}
}

func TestDialRejectsInvalidHandshakeResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		responseHeaders func(key string) http.Header
		protocols       []string
	}{
		{
			name: "bad accept",
			responseHeaders: func(string) http.Header {
				return http.Header{"Sec-WebSocket-Accept": {"wrong"}}
			},
		},
		{
			name: "unsolicited extension",
			responseHeaders: func(key string) http.Header {
				return http.Header{
					"Sec-WebSocket-Accept":     {generateAcceptKey(key)},
					"Sec-WebSocket-Extensions": {"permessage-deflate"},
				}
			},
		},
		{
			name:      "unrequested subprotocol",
			protocols: []string{"relay.v1"},
			responseHeaders: func(key string) http.Header {
				return http.Header{
					"Sec-WebSocket-Accept":   {generateAcceptKey(key)},
					"Sec-WebSocket-Protocol": {"admin.v1"},
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := rawWebSocketServer(t, func(r *http.Request, rw *bufio.ReadWriter) {
				headers := test.responseHeaders(r.Header.Get("Sec-WebSocket-Key"))
				writeSwitchingProtocols(t, rw, headers)
			})
			defer server.Close()

			conn, resp, err := Dial(context.Background(), websocketURL(server.URL), &DialOptions{Subprotocols: test.protocols})
			if conn != nil || resp == nil || !errors.Is(err, ErrBadHandshake) {
				t.Fatalf("Dial() = (%v, %#v, %v)", conn, resp, err)
			}
		})
	}
}

func TestDialAndIOCancellation(t *testing.T) {
	t.Parallel()

	t.Run("dial", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			time.Sleep(time.Second)
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, _, err := Dial(ctx, websocketURL(server.URL), nil)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Dial() error = %v", err)
		}
	})

	for _, test := range []struct {
		name    string
		partial []byte
	}{
		{"partial header", []byte{0x81}},
		{"partial payload", []byte{0x81, 0x05, 'h', 'e'}},
	} {
		t.Run(test.name, func(t *testing.T) {
			written := make(chan struct{})
			server := rawWebSocketServer(t, func(r *http.Request, rw *bufio.ReadWriter) {
				writeSwitchingProtocols(t, rw, http.Header{
					"Sec-WebSocket-Accept": {generateAcceptKey(r.Header.Get("Sec-WebSocket-Key"))},
				})
				_, _ = rw.Write(test.partial)
				_ = rw.Flush()
				close(written)
				_, _ = rw.ReadByte()
			})
			defer server.Close()

			conn, _, err := Dial(context.Background(), websocketURL(server.URL), nil)
			if err != nil {
				t.Fatal(err)
			}
			<-written
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if _, _, err := conn.Read(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Read() error = %v", err)
			}
			if _, _, err := conn.Read(context.Background()); err == nil {
				t.Fatal("second Read() succeeded on canceled connection")
			}
		})
	}

	t.Run("partial write", func(t *testing.T) {
		clientSide, serverSide := net.Pipe()
		defer serverSide.Close()
		conn := newConn(newLowConn(clientSide, bufio.NewReadWriter(bufio.NewReader(clientSide), bufio.NewWriter(clientSide)), false, defaultMaxMessageSize))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := conn.Write(ctx, BinaryMessage, make([]byte, 1<<20))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Write() error = %v", err)
		}
		if err := conn.Write(context.Background(), BinaryMessage, []byte("again")); err == nil {
			t.Fatal("second Write() succeeded on canceled connection")
		}
	})
}

func TestClientAutomaticControlFramesAreMasked(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		serverFrame   []byte
		wantOpcode    int
		wantPayload   string
		wantCloseCode int
	}{
		{"pong", []byte{0x89, 0x02, 'h', 'i'}, OpcodePong, "hi", 0},
		{"close echo", []byte{0x88, 0x02, 0x03, 0xe8}, OpcodeClose, "\x03\xe8", CloseNormalClosure},
	} {
		t.Run(test.name, func(t *testing.T) {
			wireResult := make(chan error, 1)
			server := rawWebSocketServer(t, func(r *http.Request, rw *bufio.ReadWriter) {
				writeSwitchingProtocols(t, rw, http.Header{
					"Sec-WebSocket-Accept": {generateAcceptKey(r.Header.Get("Sec-WebSocket-Key"))},
				})
				_, _ = rw.Write(test.serverFrame)
				_ = rw.Flush()
				frame, err := NewFrameReader(rw.Reader, defaultMaxMessageSize).ReadFrame()
				if err != nil {
					wireResult <- err
					return
				}
				if !frame.Masked || frame.Opcode != test.wantOpcode || string(frame.Payload) != test.wantPayload {
					wireResult <- fmt.Errorf("frame = masked:%v opcode:%d payload:%q", frame.Masked, frame.Opcode, frame.Payload)
					return
				}
				if test.wantOpcode == OpcodePong {
					_, _ = rw.Write([]byte{0x81, 0x02, 'o', 'k'})
					_ = rw.Flush()
				}
				wireResult <- nil
			})
			defer server.Close()

			conn, _, err := Dial(context.Background(), websocketURL(server.URL), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, payload, readErr := conn.Read(context.Background())
			if test.wantCloseCode != 0 {
				if !IsCloseError(readErr, test.wantCloseCode) {
					t.Fatalf("Read() close error = %v", readErr)
				}
			} else if readErr != nil || string(payload) != "ok" {
				t.Fatalf("Read() = %q, %v", payload, readErr)
			}
			if err := <-wireResult; err != nil {
				t.Fatalf("wire: %v", err)
			}
		})
	}
}

func TestEmptyInitialFragment(t *testing.T) {
	t.Parallel()

	for _, messageType := range []int{TextMessage, BinaryMessage} {
		t.Run(fmt.Sprintf("type-%d", messageType), func(t *testing.T) {
			clientSide, serverSide := net.Pipe()
			defer clientSide.Close()
			defer serverSide.Close()
			conn := newConn(newLowConn(serverSide, bufio.NewReadWriter(bufio.NewReader(serverSide), bufio.NewWriter(serverSide)), true, defaultMaxMessageSize))
			go func() {
				writer := NewFrameWriter(bufio.NewWriter(clientSide), false)
				_ = writer.WriteFrame(&Frame{Opcode: messageType, Masked: true})
				_ = writer.WriteFrame(&Frame{Fin: true, Opcode: OpcodeContinuation, Masked: true, Payload: []byte("ok")})
			}()
			gotType, payload, err := conn.ReadMessage()
			if err != nil || gotType != messageType || string(payload) != "ok" {
				t.Fatalf("ReadMessage() = (%d, %q, %v)", gotType, payload, err)
			}
		})
	}
}

func TestWriteRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()
	conn := newConn(newLowConn(clientSide, bufio.NewReadWriter(bufio.NewReader(clientSide), bufio.NewWriter(clientSide)), false, defaultMaxMessageSize))
	if err := conn.WriteMessage(TextMessage, []byte{0xff}); !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("WriteMessage() error = %v", err)
	}
}

func TestDialRejectsInvalidSubprotocolBeforeConnecting(t *testing.T) {
	t.Parallel()
	_, _, err := Dial(context.Background(), "ws://127.0.0.1:1", &DialOptions{Subprotocols: []string{"relay,admin"}})
	if err == nil || !strings.Contains(err.Error(), "invalid subprotocol") {
		t.Fatalf("Dial() error = %v", err)
	}
}

func TestDialOwnsHandshakeHeaders(t *testing.T) {
	t.Parallel()

	seen := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !headerHasToken(r.Header, "Connection", "upgrade") ||
			r.Header.Get("Sec-WebSocket-Version") != websocketVersion ||
			r.Header.Get("Sec-WebSocket-Extensions") != "" {
			seen <- fmt.Errorf("handshake headers = %#v", r.Header)
			return
		}
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		seen <- err
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	conn, _, err := Dial(context.Background(), websocketURL(server.URL), &DialOptions{
		HTTPHeader: http.Header{
			"connection":                  {"close"},
			"sec-websocket-version":       {"12"},
			"sec-websocket-extensions":    {"permessage-deflate"},
			"Sec-WebSocket-Key":           {"attacker-controlled"},
			"Sec-WebSocket-Protocol":      {"attacker-controlled"},
			"Sec-WebSocket-Authorization": {"preserved"},
		},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if err := <-seen; err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsMaskedServerFrame(t *testing.T) {
	t.Parallel()

	server := rawWebSocketServer(t, func(r *http.Request, rw *bufio.ReadWriter) {
		writeSwitchingProtocols(t, rw, http.Header{
			"Sec-WebSocket-Accept": {generateAcceptKey(r.Header.Get("Sec-WebSocket-Key"))},
		})
		frame := []byte{0x81, 0x82, 1, 2, 3, 4, 'o' ^ 1, 'k' ^ 2}
		if _, err := rw.Write(frame); err != nil {
			t.Errorf("write frame: %v", err)
		}
		_ = rw.Flush()
	})
	defer server.Close()

	conn, _, err := Dial(context.Background(), websocketURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, _, err := conn.Read(context.Background()); !errors.Is(err, ErrMaskingViolation) {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestCloseWithStatusValidation(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := netPipe(t)
	defer serverSide.Close()
	conn := newConn(newLowConn(clientSide, bufio.NewReadWriter(bufio.NewReader(clientSide), bufio.NewWriter(clientSide)), false, defaultMaxMessageSize))

	if err := conn.CloseWithStatus(CloseNoStatusReceived, ""); !errors.Is(err, ErrInvalidCloseCode) {
		t.Fatalf("reserved code error = %v", err)
	}
	if err := conn.CloseWithStatus(CloseNormalClosure, strings.Repeat("x", 124)); !errors.Is(err, ErrControlFrameTooBig) {
		t.Fatalf("long reason error = %v", err)
	}
	_ = clientSide.Close()
}

func rawWebSocketServer(t *testing.T, serve func(*http.Request, *bufio.ReadWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement Hijacker")
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer conn.Close()
		serve(r, rw)
	}))
}

func writeSwitchingProtocols(t *testing.T, rw *bufio.ReadWriter, headers http.Header) {
	t.Helper()
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	_, _ = rw.WriteString("Connection: Upgrade\r\nUpgrade: websocket\r\n")
	for name, values := range headers {
		for _, value := range values {
			_, _ = fmt.Fprintf(rw, "%s: %s\r\n", name, value)
		}
	}
	_, _ = rw.WriteString("\r\n")
	if err := rw.Flush(); err != nil {
		t.Errorf("flush handshake: %v", err)
	}
}

func websocketURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// netPipe is wrapped so test cleanup remains uniform and future deadline
// tests can share the helper.
func netPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	return client, server
}
