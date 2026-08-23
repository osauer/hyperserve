package websocket

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDialHTTPClientDirectCustomTransport(t *testing.T) {
	t.Parallel()

	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Custom-Transport"); got != "used" {
			serverErr <- fmt.Errorf("X-Custom-Transport = %q", got)
			return
		}
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		messageType, payload, err := conn.ReadMessage()
		if err == nil {
			err = conn.WriteMessage(messageType, payload)
		}
		serverErr <- err
	}))
	defer server.Close()

	base := http.DefaultTransport.(*http.Transport).Clone()
	defer base.CloseIdleConnections()
	transport := &headerTransport{base: base}
	httpClient := &http.Client{Transport: transport, Timeout: 30 * time.Millisecond}
	conn, _, err := Dial(context.Background(), websocketURL(server.URL), &DialOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	if httpClient.Transport != transport || httpClient.Timeout != 30*time.Millisecond {
		t.Fatal("Dial mutated the supplied HTTP client")
	}
	if transport.calls.Load() != 1 {
		t.Fatalf("transport calls = %d, want 1", transport.calls.Load())
	}
	if conn.LocalAddr() == nil || conn.RemoteAddr() == nil {
		t.Fatal("standard HTTP transport did not expose connection addresses")
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	// HTTPClient.Timeout applies to the handshake, not the upgraded stream.
	time.Sleep(2 * httpClient.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, TextMessage, []byte("through transport")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	_, payload, err := conn.Read(ctx)
	if err != nil || string(payload) != "through transport" {
		t.Fatalf("Read() = %q, %v", payload, err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestDialHTTPClientUsesHTTPProxy(t *testing.T) {
	t.Parallel()

	targetErr := make(chan error, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			targetErr <- err
			return
		}
		defer conn.Close()
		messageType, payload, err := conn.ReadMessage()
		if err == nil {
			err = conn.WriteMessage(messageType, payload)
		}
		targetErr <- err
	}))
	defer target.Close()

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxySeen := make(chan error, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Proxy-Authorization"); got == "" {
			proxySeen <- errors.New("missing Proxy-Authorization")
		} else {
			proxySeen <- nil
		}
		reverseProxy.ServeHTTP(w, r)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword("relay-proxy", "secret")
	var proxyCalls atomic.Int32
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(*http.Request) (*url.URL, error) {
		proxyCalls.Add(1)
		return proxyURL, nil
	}
	defer transport.CloseIdleConnections()

	conn, _, err := Dial(context.Background(), websocketURL(target.URL), &DialOptions{
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if proxyCalls.Load() == 0 {
		t.Fatal("configured proxy was not consulted")
	}
	if err := <-proxySeen; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, BinaryMessage, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	_, payload, err := conn.Read(ctx)
	if err != nil || string(payload) != "\x01\x02\x03" {
		t.Fatalf("Read() = %q, %v", payload, err)
	}
	if err := <-targetErr; err != nil {
		t.Fatalf("target: %v", err)
	}
}

func TestDialHTTPClientDefaultTransportUsesProxyFromEnvironment(t *testing.T) {
	if os.Getenv("HYPERSERVE_PROXY_CHILD") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := Dial(ctx, "ws://relay.invalid/socket", &DialOptions{HTTPClient: &http.Client{}})
		if err != nil {
			t.Fatalf("child Dial() error = %v", err)
		}
		_ = conn.Close()
		return
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer target.Close()
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxySeen := make(chan struct{}, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case proxySeen <- struct{}{}:
		default:
		}
		reverseProxy.ServeHTTP(w, r)
	}))
	defer proxy.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDialHTTPClientDefaultTransportUsesProxyFromEnvironment$")
	cmd.Env = append(os.Environ(),
		"HYPERSERVE_PROXY_CHILD=1",
		"HTTP_PROXY="+proxy.URL,
		"HTTPS_PROXY=",
		"ALL_PROXY=",
		"NO_PROXY=",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("proxy child failed: %v\n%s", err, output)
	}
	select {
	case <-proxySeen:
	case <-time.After(time.Second):
		t.Fatal("http.DefaultTransport did not use HTTP_PROXY")
	}
}

func TestDialHTTPClientUsesHTTPSProxyTunnel(t *testing.T) {
	t.Parallel()

	targetErr := make(chan error, 1)
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			targetErr <- err
			return
		}
		defer conn.Close()
		messageType, payload, err := conn.ReadMessage()
		if err == nil {
			err = conn.WriteMessage(messageType, payload)
		}
		targetErr <- err
	}))
	defer target.Close()

	connectSeen := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		connectSeen <- r.Host
		upstream, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		downstream, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer downstream.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		copyDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, downstream)
			_ = upstream.Close()
			close(copyDone)
		}()
		_, _ = io.Copy(downstream, upstream)
		<-copyDone
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(target.Certificate())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	defer transport.CloseIdleConnections()

	conn, _, err := Dial(context.Background(), strings.Replace(target.URL, "https://", "wss://", 1), &DialOptions{
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if got := <-connectSeen; got != target.Listener.Addr().String() {
		t.Fatalf("CONNECT target = %q, want %q", got, target.Listener.Addr())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, TextMessage, []byte("tunneled")); err != nil {
		t.Fatal(err)
	}
	_, payload, err := conn.Read(ctx)
	if err != nil || string(payload) != "tunneled" {
		t.Fatalf("Read() = %q, %v", payload, err)
	}
	if err := <-targetErr; err != nil {
		t.Fatalf("target: %v", err)
	}
}

func TestDialHTTPClientPreservesJarAndRedirectCallback(t *testing.T) {
	t.Parallel()

	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.SetCookie(w, &http.Cookie{Name: "relay_session", Value: "ready", Path: "/"})
			http.Redirect(w, r, "/socket", http.StatusTemporaryRedirect)
			return
		}
		cookie, err := r.Cookie("relay_session")
		if err != nil || cookie.Value != "ready" {
			serverErr <- fmt.Errorf("redirect cookie = %#v, %v", cookie, err)
			return
		}
		conn, err := (&Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		serverErr <- err
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var redirects atomic.Int32
	httpClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirects.Add(1)
			return nil
		},
	}
	conn, _, err := Dial(context.Background(), websocketURL(server.URL)+"/start", &DialOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	if redirects.Load() != 1 {
		t.Fatalf("redirect callbacks = %d, want 1", redirects.Load())
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestDialHTTPClientRedirectSecurity(t *testing.T) {
	t.Parallel()

	t.Run("cross-origin credentials stripped after callback", func(t *testing.T) {
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
			http.Redirect(w, r, final.URL, http.StatusTemporaryRedirect)
		}))
		defer redirect.Close()

		httpClient := &http.Client{CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			req.Header.Set("Authorization", "Bearer callback-readded")
			return nil
		}}
		conn, _, err := Dial(context.Background(), websocketURL(redirect.URL), &DialOptions{
			HTTPClient: httpClient,
			HTTPHeader: http.Header{"Authorization": {"Bearer initial"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if got := <-authorization; got != "" {
			t.Fatalf("cross-origin Authorization = %q", got)
		}
	})

	t.Run("secure downgrade refused", func(t *testing.T) {
		var calls atomic.Int32
		httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return redirectResponse(req, "ws://plain.example/socket"), nil
		})}
		_, _, err := Dial(context.Background(), "wss://secure.example/socket", &DialOptions{HTTPClient: httpClient})
		if !errors.Is(err, ErrBadHandshake) || !strings.Contains(err.Error(), "refusing wss to ws") {
			t.Fatalf("Dial() error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("transport calls = %d, want 1", calls.Load())
		}
	})

	t.Run("callback cannot introduce secure downgrade", func(t *testing.T) {
		var calls atomic.Int32
		httpClient := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return redirectResponse(req, "wss://other-secure.example/socket"), nil
			}),
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				req.URL.Scheme = "http"
				return nil
			},
		}
		_, _, err := Dial(context.Background(), "wss://secure.example/socket", &DialOptions{HTTPClient: httpClient})
		if !errors.Is(err, ErrBadHandshake) || !strings.Contains(err.Error(), "refusing wss to ws") {
			t.Fatalf("Dial() error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("transport calls = %d, want 1", calls.Load())
		}
	})

	t.Run("redirect limit", func(t *testing.T) {
		var calls atomic.Int32
		httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return redirectResponse(req, "/again"), nil
		})}
		_, _, err := Dial(context.Background(), "ws://relay.example/start", &DialOptions{HTTPClient: httpClient})
		if !errors.Is(err, ErrBadHandshake) || !strings.Contains(err.Error(), "too many redirects") {
			t.Fatalf("Dial() error = %v", err)
		}
		if calls.Load() != maxRedirects {
			t.Fatalf("transport calls = %d, want %d", calls.Load(), maxRedirects)
		}
	})
}

func TestDialHTTPClientCustomWritableBodyCancellation(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	closed := make(chan struct{})
	body := &rwcOnly{ReadWriteCloser: clientSide, closed: closed}
	httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return switchingProtocolsResponse(req, body), nil
	})}
	conn, _, err := Dial(context.Background(), "ws://custom.transport/socket", &DialOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}

	if conn.LocalAddr() != nil || conn.RemoteAddr() != nil {
		t.Fatal("custom transport unexpectedly exposed network addresses")
	}
	if err := conn.SetReadDeadline(time.Now()); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	written := make(chan error, 1)
	go func() {
		_, err := serverSide.Write([]byte{0x81})
		written <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := conn.Read(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Read() error = %v", err)
	}
	if err := <-written; err != nil {
		t.Fatalf("partial frame write: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("canceled read did not close custom upgraded body")
	}
}

func TestDialHTTPClientRejectsUnsafeConfigurations(t *testing.T) {
	t.Parallel()

	t.Run("non-writable body", func(t *testing.T) {
		body := &trackingReadCloser{Reader: strings.NewReader("")}
		httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return switchingProtocolsResponse(req, body), nil
		})}
		conn, resp, err := Dial(context.Background(), "ws://custom.transport/socket", &DialOptions{HTTPClient: httpClient})
		if conn != nil || resp == nil || !errors.Is(err, ErrBadHandshake) || !strings.Contains(err.Error(), "non-writable") {
			t.Fatalf("Dial() = (%v, %#v, %v)", conn, resp, err)
		}
		if !body.closed.Load() {
			t.Fatal("non-writable response body was not closed")
		}
	})

	t.Run("invalid 101 closes writable body without reading", func(t *testing.T) {
		clientSide, serverSide := net.Pipe()
		defer serverSide.Close()
		closed := make(chan struct{})
		body := &rwcOnly{ReadWriteCloser: clientSide, closed: closed}
		httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			resp := switchingProtocolsResponse(req, body)
			resp.Header.Set("Sec-WebSocket-Accept", "invalid")
			return resp, nil
		})}
		_, _, err := Dial(context.Background(), "ws://custom.transport/socket", &DialOptions{HTTPClient: httpClient})
		if !errors.Is(err, ErrBadHandshake) {
			t.Fatalf("Dial() error = %v", err)
		}
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("invalid 101 response body was not closed")
		}
	})

	for _, test := range []struct {
		name string
		opts *DialOptions
	}{
		{"NetDialer", &DialOptions{HTTPClient: &http.Client{}, NetDialer: &net.Dialer{}}},
		{"TLSConfig", &DialOptions{HTTPClient: &http.Client{}, TLSConfig: &tls.Config{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := Dial(context.Background(), "ws://127.0.0.1:1", test.opts)
			if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("Dial() error = %v", err)
			}
		})
	}
}

func TestDialHTTPClientTimeoutBoundsHandshake(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	_, _, err := Dial(context.Background(), websocketURL(server.URL), &DialOptions{
		HTTPClient: &http.Client{Timeout: 20 * time.Millisecond},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Dial() error = %v", err)
	}
}

func TestDialHTTPClientTimeoutDoesNotCancelSuccessfulCustomTransport(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	closed := make(chan struct{})
	body := &rwcOnly{ReadWriteCloser: clientSide, closed: closed}
	stopWatching := make(chan struct{})
	defer close(stopWatching)
	httpClient := &http.Client{
		Timeout: 20 * time.Millisecond,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			go func() {
				select {
				case <-req.Context().Done():
					_ = body.Close()
				case <-stopWatching:
				}
			}()
			return switchingProtocolsResponse(req, body), nil
		}),
	}

	conn, _, err := Dial(context.Background(), "ws://custom.transport/socket", &DialOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * httpClient.Timeout)
	select {
	case <-closed:
		t.Fatal("successful handshake context canceled the upgraded body")
	default:
	}
	conn.conn.abort()
}

type headerTransport struct {
	base  http.RoundTripper
	calls atomic.Int32
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	req.Header.Set("X-Custom-Transport", "used")
	return t.base.RoundTrip(req)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type rwcOnly struct {
	io.ReadWriteCloser
	closed chan struct{}
}

func (c *rwcOnly) Close() error {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return c.ReadWriteCloser.Close()
}

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (c *trackingReadCloser) Close() error {
	c.closed.Store(true)
	return nil
}

func switchingProtocolsResponse(req *http.Request, body io.ReadCloser) *http.Response {
	header := make(http.Header)
	header.Set("Connection", "Upgrade")
	header.Set("Upgrade", "websocket")
	header.Set("Sec-WebSocket-Accept", generateAcceptKey(req.Header.Get("Sec-WebSocket-Key")))
	return &http.Response{
		Status:     "101 Switching Protocols",
		StatusCode: http.StatusSwitchingProtocols,
		Header:     header,
		Body:       body,
		Request:    req,
	}
}

func redirectResponse(req *http.Request, location string) *http.Response {
	return &http.Response{
		Status:     "307 Temporary Redirect",
		StatusCode: http.StatusTemporaryRedirect,
		Header:     http.Header{"Location": {location}},
		Body:       io.NopCloser(strings.NewReader("redirect")),
		Request:    req,
	}
}
