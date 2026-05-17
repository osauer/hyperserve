package websocket

import (
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

	// Handler functions
	closeHandler func(code int, text string) error
	pingHandler  func(appData string) error
	pongHandler  func(appData string) error

	// Handler mutex for thread safety
	handlerMu sync.Mutex
}

// Upgrader upgrades HTTP connections to WebSocket connections
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

	// WriteBufferSize is the size of the write buffer
	WriteBufferSize int

	// ReadBufferSize is the size of the read buffer
	ReadBufferSize int

	// HandshakeTimeout specifies the duration for the handshake to complete
	HandshakeTimeout time.Duration

	// EnableCompression specifies if the server should attempt to negotiate compression
	EnableCompression bool

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
		maxMessageSize = 1024 * 1024 // 1MB default
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
		CheckOrigin:   checkOrigin,
		Subprotocols:  u.Subprotocols,
		BeforeUpgrade: u.BeforeUpgrade,
	}

	// Perform handshake
	netConn, buf, err := PerformHandshake(w, r, opts)
	if err != nil {
		if u.Error != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrBadHandshake) {
				status = http.StatusForbidden
			} else if errors.Is(err, ErrUnsupportedVersion) {
				status = http.StatusBadRequest
				w.Header().Set("Sec-WebSocket-Version", "13")
			}
			u.Error(w, r, status, err)
		}
		return nil, err
	}

	// Validate protocol negotiation if required
	if u.RequireProtocol && len(u.Subprotocols) > 0 {
		// Check if a protocol was negotiated
		protocol := r.Header.Get("Sec-WebSocket-Protocol")
		if protocol == "" {
			if u.Error != nil {
				u.Error(w, r, http.StatusBadRequest, errors.New("subprotocol required"))
			}
			netConn.Close()
			return nil, errors.New("subprotocol required")
		}
	}

	// Apply handshake timeout if specified
	if u.HandshakeTimeout > 0 {
		netConn.SetDeadline(time.Now().Add(u.HandshakeTimeout))
		defer netConn.SetDeadline(time.Time{})
	}

	// Create WebSocket connection
	wsConn := newLowConn(netConn, buf, true, maxMessageSize)

	c := &Conn{
		conn: wsConn,
	}

	// Set default handlers
	c.SetCloseHandler(nil)
	c.SetPingHandler(nil)
	c.SetPongHandler(nil)

	return c, nil
}

// ReadMessage reads a message from the WebSocket connection
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	// Read the message
	messageType, p, err = c.conn.ReadMessage()
	if err != nil {
		return messageType, p, err
	}

	// Handle control messages
	switch messageType {
	case PingMessage:
		c.handlerMu.Lock()
		handler := c.pingHandler
		c.handlerMu.Unlock()
		if handler != nil {
			if err := handler(string(p)); err != nil {
				return messageType, p, err
			}
		}
		// Continue reading for the next message
		return c.ReadMessage()

	case PongMessage:
		c.handlerMu.Lock()
		handler := c.pongHandler
		c.handlerMu.Unlock()
		if handler != nil {
			if err := handler(string(p)); err != nil {
				return messageType, p, err
			}
		}
		// Continue reading for the next message
		return c.ReadMessage()

	case CloseMessage:
		var code int
		var text string
		if len(p) >= 2 {
			code = int(p[0])<<8 | int(p[1])
			if len(p) > 2 {
				text = string(p[2:])
			}
		} else {
			code = CloseNoStatusReceived
		}

		c.handlerMu.Lock()
		handler := c.closeHandler
		c.handlerMu.Unlock()
		if handler != nil {
			if err := handler(code, text); err != nil {
				return messageType, p, err
			}
		}
		return messageType, p, err
	}

	return messageType, p, err
}

// WriteMessage writes a message to the WebSocket connection
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	return c.conn.WriteMessage(messageType, data)
}

// WriteControl writes a control message with the given deadline
func (c *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.conn.SetWriteDeadline(deadline)
	defer c.conn.SetWriteDeadline(time.Time{})
	return c.conn.WriteControl(messageType, data)
}

// Close closes the WebSocket connection
func (c *Conn) Close() error {
	return c.conn.Close()
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

// SetCloseHandler sets the handler for close messages
func (c *Conn) SetCloseHandler(h func(code int, text string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if h == nil {
		// Set default close handler
		c.closeHandler = func(code int, text string) error {
			// Send close frame back
			message := make([]byte, 2+len(text))
			message[0] = byte(code >> 8)
			message[1] = byte(code)
			copy(message[2:], text)
			return c.WriteControl(CloseMessage, message, time.Now().Add(time.Second))
		}
	} else {
		c.closeHandler = h
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

// SetPingHandler sets the handler for ping messages
func (c *Conn) SetPingHandler(h func(appData string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if h == nil {
		// Set default ping handler
		c.pingHandler = func(appData string) error {
			// Respond with pong
			return c.WriteControl(PongMessage, []byte(appData), time.Now().Add(time.Second))
		}
	} else {
		c.pingHandler = h
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

// SetPongHandler sets the handler for pong messages
func (c *Conn) SetPongHandler(h func(appData string) error) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	if h == nil {
		// Set default pong handler (no-op)
		c.pongHandler = func(appData string) error {
			return nil
		}
	} else {
		c.pongHandler = h
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

	var closeErr *closeError
	if errors.As(err, &closeErr) {
		return !slices.Contains(expectedCodes, closeErr.Code)
	}

	return false
}

// IsCloseError returns true if the error is a close error with one of the specified codes
func IsCloseError(err error, codes ...int) bool {
	var closeErr *closeError
	if errors.As(err, &closeErr) {
		if slices.Contains(codes, closeErr.Code) {
			return true
		}
	}
	return false
}
