package websocket

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	maxRedirects              = 10
	handshakeErrorBodyTimeout = 3 * time.Second
)

// DialOptions configures an outbound WebSocket connection.
type DialOptions struct {
	// HTTPClient performs the opening handshake. Its Transport, proxy,
	// cookie jar, redirect callback, and timeout are preserved. A successful
	// 101 response body must implement io.ReadWriteCloser; http.Transport does.
	// HTTPClient cannot be combined with NetDialer or TLSConfig.
	HTTPClient *http.Client

	// HTTPHeader is copied into the opening handshake. Dial owns the
	// WebSocket protocol headers and overwrites conflicting values.
	HTTPHeader http.Header

	// Subprotocols lists protocols in client preference order.
	Subprotocols []string

	// TLSConfig configures wss connections. It is cloned before use.
	TLSConfig *tls.Config

	// NetDialer controls the underlying TCP connection. The zero value uses
	// net.Dialer.
	NetDialer *net.Dialer

	// MaxMessageSize limits a complete message read from the peer. Values at
	// or below zero use the package default of 1 MiB.
	MaxMessageSize int64
}

// Dial opens a WebSocket connection to a ws or wss URL. The context covers
// TCP, TLS, request, response, and redirect processing. When HTTPClient is
// provided, its transport owns dialing, TLS, and proxy behavior. On an HTTP
// handshake failure, the returned response is non-nil when one was received;
// its body is buffered up to 64 KiB and remains readable after Dial returns.
func Dial(ctx context.Context, rawURL string, opts *DialOptions) (*Conn, *http.Response, error) {
	if ctx == nil {
		return nil, nil, errors.New("websocket: nil context")
	}

	u, err := parseWebSocketURL(rawURL)
	if err != nil {
		return nil, nil, err
	}
	if opts == nil {
		opts = &DialOptions{}
	}
	if opts.HTTPClient != nil && (opts.NetDialer != nil || opts.TLSConfig != nil) {
		return nil, nil, errors.New("websocket: HTTPClient cannot be combined with NetDialer or TLSConfig")
	}
	for _, protocol := range opts.Subprotocols {
		if !validSubprotocol(protocol) {
			return nil, nil, fmt.Errorf("websocket: invalid subprotocol %q", protocol)
		}
	}

	header := cloneDialHeader(opts.HTTPHeader)
	if opts.HTTPClient != nil {
		return dialHTTPClient(ctx, u, header, opts)
	}
	for redirects := 0; ; redirects++ {
		conn, resp, err := dialOnce(ctx, u, header, opts)
		if err == nil {
			return conn, resp, nil
		}
		if resp == nil || !isRedirect(resp.StatusCode) {
			return nil, resp, err
		}
		if redirects == maxRedirects {
			return nil, resp, fmt.Errorf("%w: too many redirects", ErrBadHandshake)
		}

		location, locationErr := resp.Location()
		if locationErr != nil {
			return nil, resp, fmt.Errorf("%w: redirect location: %v", ErrBadHandshake, locationErr)
		}
		next, parseErr := parseRedirectURL(u, location)
		if parseErr != nil {
			return nil, resp, parseErr
		}
		if u.Scheme == "wss" && next.Scheme != "wss" {
			return nil, resp, fmt.Errorf("%w: refusing wss to ws redirect", ErrBadHandshake)
		}
		if !sameOrigin(u, next) {
			deleteHeaderFold(header, "Authorization")
			deleteHeaderFold(header, "Cookie")
			deleteHeaderFold(header, "Proxy-Authorization")
		}
		u = next
	}
}

func dialHTTPClient(ctx context.Context, u *url.URL, header http.Header, opts *DialOptions) (*Conn, *http.Response, error) {
	client := *opts.HTTPClient

	originalCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		normalizeHTTPRedirectScheme(req.URL)
		if len(via) >= maxRedirects {
			return fmt.Errorf("%w: too many redirects", ErrBadHandshake)
		}
		previous := via[len(via)-1].URL
		if secureURLScheme(previous.Scheme) && !secureURLScheme(req.URL.Scheme) {
			return fmt.Errorf("%w: refusing wss to ws redirect", ErrBadHandshake)
		}
		if originalCheckRedirect != nil {
			if err := originalCheckRedirect(req, via); err != nil {
				return err
			}
		}
		normalizeHTTPRedirectScheme(req.URL)
		if secureURLScheme(previous.Scheme) && !secureURLScheme(req.URL.Scheme) {
			return fmt.Errorf("%w: refusing wss to ws redirect", ErrBadHandshake)
		}
		if !sameOrigin(previous, req.URL) {
			deleteHeaderFold(req.Header, "Authorization")
			deleteHeaderFold(req.Header, "Cookie")
			deleteHeaderFold(req.Header, "Proxy-Authorization")
		}
		return nil
	}

	key, err := handshakeKey()
	if err != nil {
		return nil, nil, err
	}
	httpURL := *u
	normalizeHTTPRedirectScheme(&httpURL)
	dialCtx, finishHandshake := newHandshakeContext(ctx, client.Timeout)
	client.Timeout = 0
	req, err := http.NewRequestWithContext(dialCtx, http.MethodGet, httpURL.String(), nil)
	if err != nil {
		_ = finishHandshake()
		return nil, nil, fmt.Errorf("websocket: create handshake request: %w", err)
	}
	req.Header = header.Clone()
	setHandshakeHeaders(req.Header, key, opts.Subprotocols)

	var traceMu sync.Mutex
	var tracedConn net.Conn
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		traceMu.Lock()
		tracedConn = info.Conn
		traceMu.Unlock()
	}}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	contextErr := finishHandshake()
	if err != nil {
		if contextErr != nil {
			return nil, resp, contextErr
		}
		return nil, resp, contextOrError(dialCtx, fmt.Errorf("websocket: HTTP handshake: %w", err))
	}
	if contextErr != nil {
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		return nil, resp, contextErr
	}
	if err := validateServerHandshake(resp, key, opts.Subprotocols); err != nil {
		preserveHandshakeErrorBody(resp)
		return nil, resp, err
	}

	rwc, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		bodyType := fmt.Sprintf("%T", resp.Body)
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		return nil, resp, fmt.Errorf("%w: HTTP transport returned non-writable response body %s", ErrBadHandshake, bodyType)
	}
	traceMu.Lock()
	netConn := tracedConn
	traceMu.Unlock()
	if bodyConn, ok := rwc.(net.Conn); ok {
		netConn = bodyConn
	}

	maxMessageSize := opts.MaxMessageSize
	if maxMessageSize <= 0 {
		maxMessageSize = defaultMaxMessageSize
	}
	buf := bufio.NewReadWriter(bufio.NewReader(rwc), bufio.NewWriter(rwc))
	low := newLowConnWithNetConn(rwc, netConn, buf, false, maxMessageSize)
	conn := newConn(low)
	conn.subprotocol = resp.Header.Get("Sec-WebSocket-Protocol")
	resp.Body = http.NoBody
	return conn, resp, nil
}

type handshakeContext struct {
	context.Context
	deadline    time.Time
	hasDeadline bool
}

func (c *handshakeContext) Deadline() (time.Time, bool) {
	return c.deadline, c.hasDeadline
}

// newHandshakeContext preserves the caller's values, deadline, and
// cancellation during the HTTP round trip. finish stops those signals without
// canceling the successful upgraded stream, which some custom transports keep
// tied to Request.Context after RoundTrip returns.
func newHandshakeContext(parent context.Context, timeout time.Duration) (context.Context, func() error) {
	base := context.WithoutCancel(parent)
	cancelCtx, cancel := context.WithCancelCause(base)
	deadline, hasDeadline := parent.Deadline()
	if timeout > 0 {
		timeoutDeadline := time.Now().Add(timeout)
		if !hasDeadline || timeoutDeadline.Before(deadline) {
			deadline = timeoutDeadline
			hasDeadline = true
		}
	}
	dialCtx := &handshakeContext{Context: cancelCtx, deadline: deadline, hasDeadline: hasDeadline}

	parentDone := make(chan struct{})
	stopParent := context.AfterFunc(parent, func() {
		defer close(parentDone)
		cancel(context.Cause(parent))
	})

	var timeoutTimer *time.Timer
	var timeoutDone chan struct{}
	if timeout > 0 {
		timeoutDone = make(chan struct{})
		timeoutTimer = time.AfterFunc(timeout, func() {
			defer close(timeoutDone)
			cancel(context.DeadlineExceeded)
		})
	}

	finish := func() error {
		if timeoutTimer != nil && !timeoutTimer.Stop() {
			<-timeoutDone
		}
		if !stopParent() {
			<-parentDone
		}
		if parent.Err() != nil {
			return parent.Err()
		}
		if errors.Is(context.Cause(cancelCtx), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		if cancelCtx.Err() != nil {
			return context.Canceled
		}
		return nil
	}
	return dialCtx, finish
}

func dialOnce(ctx context.Context, u *url.URL, header http.Header, opts *DialOptions) (*Conn, *http.Response, error) {
	dialer := opts.NetDialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}

	netConn, err := dialer.DialContext(ctx, "tcp", websocketAddress(u))
	if err != nil {
		return nil, nil, fmt.Errorf("websocket: dial %s: %w", u.Host, err)
	}
	closeConn := true
	defer func() {
		if closeConn {
			_ = netConn.Close()
		}
	}()

	stopCancel := context.AfterFunc(ctx, func() {
		_ = netConn.SetDeadline(time.Now())
	})
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		if err := netConn.SetDeadline(deadline); err != nil {
			return nil, nil, fmt.Errorf("websocket: set handshake deadline: %w", err)
		}
	}

	if u.Scheme == "wss" {
		var tlsConfig *tls.Config
		if opts.TLSConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = opts.TLSConfig.Clone()
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = u.Hostname()
		}
		tlsConfig.NextProtos = []string{"http/1.1"}
		tlsConn := tls.Client(netConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, nil, fmt.Errorf("websocket: TLS handshake: %w", err)
		}
		netConn = tlsConn
	}

	key, err := handshakeKey()
	if err != nil {
		return nil, nil, err
	}

	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Host:   u.Host,
		Header: header.Clone(),
	}
	req = req.WithContext(ctx)
	setHandshakeHeaders(req.Header, key, opts.Subprotocols)

	if err := req.Write(netConn); err != nil {
		return nil, nil, contextOrError(ctx, fmt.Errorf("websocket: write handshake: %w", err))
	}

	reader := bufio.NewReader(netConn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return nil, nil, contextOrError(ctx, fmt.Errorf("websocket: read handshake: %w", err))
	}
	if err := validateServerHandshake(resp, key, opts.Subprotocols); err != nil {
		preserveHandshakeErrorBody(resp)
		return nil, resp, err
	}

	if err := netConn.SetDeadline(time.Time{}); err != nil {
		return nil, resp, fmt.Errorf("websocket: clear handshake deadline: %w", err)
	}
	if !stopCancel() {
		return nil, resp, contextOrError(ctx, errors.New("websocket: handshake canceled"))
	}

	maxMessageSize := opts.MaxMessageSize
	if maxMessageSize <= 0 {
		maxMessageSize = defaultMaxMessageSize
	}
	low := newLowConn(netConn, bufio.NewReadWriter(reader, bufio.NewWriter(netConn)), false, maxMessageSize)
	conn := newConn(low)
	conn.subprotocol = resp.Header.Get("Sec-WebSocket-Protocol")
	resp.Body = http.NoBody
	closeConn = false
	return conn, resp, nil
}

func validateServerHandshake(resp *http.Response, key string, requestedProtocols []string) error {
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("%w: server returned %s", ErrBadHandshake, resp.Status)
	}
	if !headerHasToken(resp.Header, "Connection", "upgrade") ||
		!headerHasToken(resp.Header, "Upgrade", "websocket") {
		return fmt.Errorf("%w: missing upgrade response headers", ErrBadHandshake)
	}
	wantAccept := generateAcceptKey(key)
	gotAccept := resp.Header.Get("Sec-WebSocket-Accept")
	if subtle.ConstantTimeCompare([]byte(gotAccept), []byte(wantAccept)) != 1 {
		return fmt.Errorf("%w: invalid Sec-WebSocket-Accept", ErrBadHandshake)
	}
	if extensions := resp.Header.Values("Sec-WebSocket-Extensions"); len(extensions) > 0 {
		return fmt.Errorf("%w: unsupported extension negotiation", ErrBadHandshake)
	}
	selected := resp.Header.Get("Sec-WebSocket-Protocol")
	if selected != "" && !slicesContains(requestedProtocols, selected) {
		return fmt.Errorf("%w: server selected unrequested subprotocol %q", ErrBadHandshake, selected)
	}
	return nil
}

func parseWebSocketURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("websocket: parse URL: %w", err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("websocket: URL scheme must be ws or wss")
	}
	if u.Host == "" {
		return nil, errors.New("websocket: URL has no host")
	}
	if u.User != nil {
		return nil, errors.New("websocket: URL user information is not supported")
	}
	if u.Fragment != "" {
		return nil, errors.New("websocket: URL fragments are not allowed")
	}
	return u, nil
}

func parseRedirectURL(base, location *url.URL) (*url.URL, error) {
	next := base.ResolveReference(location)
	switch next.Scheme {
	case "http":
		next.Scheme = "ws"
	case "https":
		next.Scheme = "wss"
	}
	return parseWebSocketURL(next.String())
}

func normalizeHTTPRedirectScheme(u *url.URL) {
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
}

func secureURLScheme(scheme string) bool {
	return scheme == "wss" || scheme == "https"
}

func websocketAddress(u *url.URL) string {
	if u.Port() != "" {
		return u.Host
	}
	port := "80"
	if secureURLScheme(u.Scheme) {
		port = "443"
	}
	return net.JoinHostPort(u.Hostname(), port)
}

func headerHasToken(header http.Header, name, token string) bool {
	for value := range strings.SplitSeq(strings.Join(header.Values(name), ","), ",") {
		if strings.EqualFold(strings.TrimSpace(value), token) {
			return true
		}
	}
	return false
}

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(websocketAddress(a), websocketAddress(b))
}

func cloneDialHeader(source http.Header) http.Header {
	header := make(http.Header, len(source))
	for name, values := range source {
		if reservedHandshakeHeader(name) {
			continue
		}
		header[name] = append([]string(nil), values...)
	}
	return header
}

func handshakeKey() (string, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("websocket: handshake nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(keyBytes), nil
}

func setHandshakeHeaders(header http.Header, key string, subprotocols []string) {
	header.Set("Connection", "Upgrade")
	header.Set("Upgrade", "websocket")
	header.Set("Sec-WebSocket-Key", key)
	header.Set("Sec-WebSocket-Version", websocketVersion)
	if len(subprotocols) > 0 {
		header.Set("Sec-WebSocket-Protocol", strings.Join(subprotocols, ", "))
	} else {
		header.Del("Sec-WebSocket-Protocol")
	}
	header.Del("Sec-WebSocket-Extensions")
}

func reservedHandshakeHeader(name string) bool {
	return strings.EqualFold(name, "Connection") ||
		strings.EqualFold(name, "Upgrade") ||
		strings.EqualFold(name, "Sec-WebSocket-Key") ||
		strings.EqualFold(name, "Sec-WebSocket-Version") ||
		strings.EqualFold(name, "Sec-WebSocket-Protocol") ||
		strings.EqualFold(name, "Sec-WebSocket-Extensions")
}

func deleteHeaderFold(header http.Header, name string) {
	for candidate := range header {
		if strings.EqualFold(candidate, name) {
			delete(header, candidate)
		}
	}
}

func validSubprotocol(protocol string) bool {
	if protocol == "" {
		return false
	}
	for _, r := range protocol {
		if r <= 0x20 || r >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", r) {
			return false
		}
	}
	return true
}

func bufferResponseBody(resp *http.Response) {
	if resp.Body == nil {
		resp.Body = http.NoBody
		return
	}
	body := resp.Body
	closed := make(chan struct{})
	timer := time.AfterFunc(handshakeErrorBodyTimeout, func() {
		_ = body.Close()
		close(closed)
	})
	data, _ := io.ReadAll(io.LimitReader(body, 64<<10))
	if !timer.Stop() {
		<-closed
	} else {
		_ = body.Close()
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
}

func preserveHandshakeErrorBody(resp *http.Response) {
	if resp.StatusCode == http.StatusSwitchingProtocols {
		_ = resp.Body.Close()
		resp.Body = http.NoBody
		return
	}
	bufferResponseBody(resp)
}

func contextOrError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return err
}

func slicesContains(values []string, value string) bool {
	return slices.Contains(values, value)
}
