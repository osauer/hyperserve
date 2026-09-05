package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"
)

// WebSocket message-type aliases. The wire-level Opcode* constants live in
// frame.go; these names are the public, gorilla-compatible API.
const (
	TextMessage   = OpcodeText
	BinaryMessage = OpcodeBinary
	CloseMessage  = OpcodeClose
	PingMessage   = OpcodePing
	PongMessage   = OpcodePong
)

// Conn represents a WebSocket connection
type Conn struct {
	conn *lowConn

	subprotocol string

	// Handler functions. The wire-side dispatch lives on lowConn — these
	// fields are kept so that *Handler() getters can return the active
	// callback without reaching across packages.
	closeHandler func(code int, text string) error
	pingHandler  func(appData string) error
	pongHandler  func(appData string) error

	// Handler mutex for thread safety
	handlerMu sync.Mutex
}

func newConn(low *lowConn) *Conn {
	c := &Conn{conn: low}
	c.SetCloseHandler(nil)
	c.SetPingHandler(nil)
	c.SetPongHandler(nil)
	return c
}

// Upgrader upgrades HTTP connections to WebSocket connections.
// The previously-advertised WriteBufferSize, ReadBufferSize, and
// EnableCompression fields were silent no-ops (never read by Upgrade) and
// were removed in this release; if you need compression negotiation or
// buffer tuning, ship a real implementation rather than a label.
type Upgrader struct {
	// CheckOrigin returns true if the request Origin header is acceptable
	// If nil, a safe default is used that checks for same-origin requests
	CheckOrigin func(r *http.Request) bool

	// Subprotocols specifies the server's supported protocols in order of preference
	Subprotocols []string

	// Error specifies the function for generating HTTP error responses
	Error func(w http.ResponseWriter, r *http.Request, status int, reason error)

	// MaxMessageSize is the maximum size for a message read from the peer
	MaxMessageSize int64

	// HandshakeTimeout bounds writing the upgrade response after the
	// application-owned BeforeUpgrade hook returns.
	HandshakeTimeout time.Duration

	// BeforeUpgrade is called after origin check but before sending upgrade response
	// This can be used for authentication, rate limiting, or other pre-upgrade checks
	BeforeUpgrade func(w http.ResponseWriter, r *http.Request) error

	// AllowedOrigins is a list of allowed origins for CORS
	// If empty and CheckOrigin is nil, same-origin policy is enforced
	AllowedOrigins []string

	// RequireProtocol ensures the client specifies one of the supported subprotocols
	RequireProtocol bool
}

// Upgrade upgrades an HTTP connection to a WebSocket connection
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	// Set defaults
	maxMessageSize := u.MaxMessageSize
	if maxMessageSize <= 0 {
		maxMessageSize = defaultMaxMessageSize
	}

	// Configure origin checking
	checkOrigin := u.CheckOrigin
	if checkOrigin == nil {
		if len(u.AllowedOrigins) > 0 {
			// Use allowed origins list
			checkOrigin = CheckOriginWithAllowedList(u.AllowedOrigins)
		} else {
			// Use safe default (same-origin only)
			checkOrigin = DefaultCheckOrigin
		}
	}

	// Create handshake options
	opts := &HandshakeOptions{
		CheckOrigin:     checkOrigin,
		Subprotocols:    u.Subprotocols,
		RequireProtocol: u.RequireProtocol,
		ResponseHeader:  responseHeader,
		BeforeUpgrade:   u.BeforeUpgrade,
	}

	// Perform handshake
	netConn, buf, err := performHandshake(w, r, opts, u.HandshakeTimeout)
	if err != nil {
		if u.Error != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrBadHandshake) {
				status = http.StatusForbidden
			} else if errors.Is(err, ErrSubprotocolRequired) {
				status = http.StatusBadRequest
			} else if errors.Is(err, ErrUnsupportedVersion) {
				status = http.StatusBadRequest
				w.Header().Set("Sec-WebSocket-Version", "13")
			}
			u.Error(w, r, status, err)
		}
		return nil, err
	}

	// Create WebSocket connection
	wsConn := newLowConn(netConn, buf, true, maxMessageSize)
	conn := newConn(wsConn)
	conn.subprotocol = negotiateSubprotocol(parseSubprotocols(r.Header.Get("Sec-WebSocket-Protocol")), u.Subprotocols)
	return conn, nil
}

// Read reads one text or binary message. Canceling ctx interrupts the read.
// Only one goroutine may call Read or ReadMessage at a time.
func (c *Conn) Read(ctx context.Context) (messageType int, p []byte, err error) {
	if ctx == nil {
		return 0, nil, errors.New("websocket: nil context")
	}
	var setDeadline func(time.Time) error
	if c.conn.netConn != nil {
		setDeadline = c.conn.SetReadDeadline
	}
	err = withContextIO(ctx, setDeadline, c.conn.abort, func() error {
		messageType, p, err = c.conn.ReadMessage()
		return err
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		c.conn.abort()
	}
	return messageType, p, err
}

// Write writes one complete text or binary message. Canceling ctx interrupts
// the write. Only one goroutine may call Write, WriteMessage, or WriteControl
// at a time.
func (c *Conn) Write(ctx context.Context, messageType int, data []byte) error {
	if ctx == nil {
		return errors.New("websocket: nil context")
	}
	var setDeadline func(time.Time) error
	if c.conn.netConn != nil {
		setDeadline = c.conn.SetWriteDeadline
	}
	err := withContextIO(ctx, setDeadline, c.conn.abort, func() error {
		return c.conn.WriteMessage(messageType, data)
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		c.conn.abort()
	}
	return err
}

// ReadMessage reads a message from the WebSocket connection. Control-frame
// dispatch (ping/pong/close) is handled inside lowConn so user-installed
// handlers fire even when this method blocks on a Text/Binary read.
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	return c.conn.ReadMessage()
}

// WriteMessage writes a message to the WebSocket connection
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	return c.conn.WriteMessage(messageType, data)
}

// WriteControl writes a control message with the given deadline
func (c *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	if err := c.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer func() { _ = c.conn.SetWriteDeadline(time.Time{}) }()
	return c.conn.WriteControl(messageType, data)
}

// Close closes the WebSocket connection
func (c *Conn) Close() error {
	return c.conn.Close()
}

// CloseWithStatus sends a close frame with code and reason, then closes the
// network connection. reason must be valid UTF-8 and fit in one control frame.
func (c *Conn) CloseWithStatus(code int, reason string) error {
	return c.conn.CloseWithStatus(code, reason)
}

// Subprotocol returns the subprotocol selected during the opening handshake.
func (c *Conn) Subprotocol() string {
	return c.subprotocol
}

// CloseHandler returns the current close handler
func (c *Conn) CloseHandler() func(code int, text string) error {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if c.closeHandler == nil {
		return func(code int, text string) error {
			return nil
		}
	}
	return c.closeHandler
}

// SetCloseHandler sets the handler invoked when a close frame arrives.
// Passing nil installs the default no-op (the auto-echo of the close frame
// happens unconditionally inside the wire reader).
func (c *Conn) SetCloseHandler(h func(code int, text string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if h == nil {
		c.closeHandler = func(code int, text string) error { return nil }
	} else {
		c.closeHandler = h
	}
	if c.conn != nil {
		c.conn.setCloseHandler(c.closeHandler)
	}
}

// PingHandler returns the current ping handler
func (c *Conn) PingHandler() func(appData string) error {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if c.pingHandler == nil {
		return func(appData string) error {
			return c.WriteControl(PongMessage, []byte(appData), time.Now().Add(time.Second))
		}
	}
	return c.pingHandler
}

// SetPingHandler sets the handler invoked when a ping frame arrives. When a
// user handler is installed the wire reader stops sending its automatic pong
// — the handler must do that (or not) explicitly. Passing nil clears the
// handler and restores the default auto-pong behaviour.
func (c *Conn) SetPingHandler(h func(appData string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	c.pingHandler = h
	if c.conn != nil {
		c.conn.setPingHandler(h)
	}
}

// PongHandler returns the current pong handler
func (c *Conn) PongHandler() func(appData string) error {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if c.pongHandler == nil {
		return func(appData string) error {
			return nil
		}
	}
	return c.pongHandler
}

// SetPongHandler sets the handler invoked when a pong frame arrives.
// Passing nil installs a no-op default.
func (c *Conn) SetPongHandler(h func(appData string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if h == nil {
		c.pongHandler = func(appData string) error { return nil }
	} else {
		c.pongHandler = h
	}
	if c.conn != nil {
		c.conn.setPongHandler(c.pongHandler)
	}
}

// SetReadDeadline sets the read deadline on the connection
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the connection
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// LocalAddr returns the local network address
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// WriteJSON writes a JSON-encoded message to the connection
func (c *Conn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteMessage(TextMessage, data)
}

// ReadJSON reads a JSON-encoded message from the connection
func (c *Conn) ReadJSON(v any) error {
	_, data, err := c.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// IsUnexpectedCloseError checks if the error is an unexpected close error
func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	if err == nil {
		return false
	}

	if closeErr, ok := errors.AsType[*closeError](err); ok {
		return !slices.Contains(expectedCodes, closeErr.Code)
	}

	return false
}

// IsCloseError returns true if the error is a close error with one of the specified codes
func IsCloseError(err error, codes ...int) bool {
	if closeErr, ok := errors.AsType[*closeError](err); ok {
		if slices.Contains(codes, closeErr.Code) {
			return true
		}
	}
	return false
}

func withContextIO(ctx context.Context, setDeadline func(time.Time) error, abort func(), fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && setDeadline != nil {
		if err := setDeadline(deadline); err != nil {
			return err
		}
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		// A read can be writing an automatic pong or close reply. Closing
		// the transport interrupts both directions, including wrapped I/O.
		abort()
		close(done)
	})
	err := fn()
	if !stop() {
		<-done
	}
	var clearErr error
	if setDeadline != nil {
		clearErr = setDeadline(time.Time{})
	}
	if err != nil {
		return contextOrError(ctx, err)
	}
	return clearErr
}
