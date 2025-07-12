package hyperserve

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/osauer/hyperserve/internal/ws"
)

// WebSocket message types
const (
	TextMessage   = ws.OpcodeText
	BinaryMessage = ws.OpcodeBinary
	CloseMessage  = ws.OpcodeClose
	PingMessage   = ws.OpcodePing
	PongMessage   = ws.OpcodePong
)

// WebSocket close codes
const (
	CloseNormalClosure           = ws.CloseNormalClosure
	CloseGoingAway               = ws.CloseGoingAway
	CloseProtocolError           = ws.CloseProtocolError
	CloseUnsupportedData         = ws.CloseUnsupportedData
	CloseNoStatusReceived        = ws.CloseNoStatusReceived
	CloseAbnormalClosure         = ws.CloseAbnormalClosure
	CloseInvalidFramePayloadData = ws.CloseInvalidFramePayloadData
	ClosePolicyViolation         = ws.ClosePolicyViolation
	CloseMessageTooBig           = ws.CloseMessageTooBig
	CloseMandatoryExtension      = ws.CloseMandatoryExtension
	CloseInternalServerError     = ws.CloseInternalServerError
	CloseServiceRestart          = ws.CloseServiceRestart
	CloseTryAgainLater           = ws.CloseTryAgainLater
	CloseTLSHandshake            = ws.CloseTLSHandshake
)

// WebSocket errors
var (
	ErrNotWebSocket = ws.ErrNotWebSocket
	ErrBadHandshake = ws.ErrBadHandshake
)

// Conn represents a WebSocket connection
type Conn struct {
	conn         *ws.Conn
	pingInterval time.Duration
	pongTimeout  time.Duration
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
			checkOrigin = checkOriginWithAllowedList(u.AllowedOrigins)
		} else {
			// Use safe default (same-origin only)
			checkOrigin = defaultCheckOrigin
		}
	}
	
	// Create handshake options
	opts := &ws.HandshakeOptions{
		CheckOrigin:   checkOrigin,
		Subprotocols:  u.Subprotocols,
		BeforeUpgrade: u.BeforeUpgrade,
	}
	
	// Perform handshake
	netConn, buf, err := ws.PerformHandshake(w, r, opts)
	if err != nil {
		if u.Error != nil {
			status := http.StatusBadRequest
			if err == ws.ErrBadHandshake {
				status = http.StatusForbidden
			} else if err == ws.ErrUnsupportedVersion {
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
	wsConn := ws.NewConn(netConn, buf, true, maxMessageSize)
	
	return &Conn{
		conn: wsConn,
	}, nil
}

// ReadMessage reads a message from the WebSocket connection
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	return c.conn.ReadMessage()
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
	return func(code int, text string) error {
		// Default close handler
		return nil
	}
}

// SetCloseHandler sets the handler for close messages
func (c *Conn) SetCloseHandler(h func(code int, text string) error) {
	// TODO: Implement close handler support
}

// PingHandler returns the current ping handler
func (c *Conn) PingHandler() func(appData string) error {
	return func(appData string) error {
		// Default ping handler - send pong
		return c.WriteControl(PongMessage, []byte(appData), time.Now().Add(time.Second))
	}
}

// SetPingHandler sets the handler for ping messages
func (c *Conn) SetPingHandler(h func(appData string) error) {
	// TODO: Implement ping handler support
}

// PongHandler returns the current pong handler
func (c *Conn) PongHandler() func(appData string) error {
	return func(appData string) error {
		// Default pong handler - no-op
		return nil
	}
}

// SetPongHandler sets the handler for pong messages
func (c *Conn) SetPongHandler(h func(appData string) error) {
	// TODO: Implement pong handler support
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
func (c *Conn) WriteJSON(v interface{}) error {
	// This is a simplified version - in a real implementation you'd want to use encoding/json
	// For now, we'll return an error suggesting to use WriteMessage with JSON manually
	return errors.New("WriteJSON not implemented - use json.Marshal and WriteMessage")
}

// ReadJSON reads a JSON-encoded message from the connection
func (c *Conn) ReadJSON(v interface{}) error {
	// This is a simplified version - in a real implementation you'd want to use encoding/json
	// For now, we'll return an error suggesting to use ReadMessage with JSON manually
	return errors.New("ReadJSON not implemented - use ReadMessage and json.Unmarshal")
}

// IsUnexpectedCloseError checks if the error is an unexpected close error
func IsUnexpectedCloseError(err error, expectedCodes ...int) bool {
	if err == nil {
		return false
	}
	
	if closeErr, ok := err.(*ws.CloseError); ok {
		for _, code := range expectedCodes {
			if closeErr.Code == code {
				return false
			}
		}
		return true
	}
	
	return false
}

// IsCloseError returns true if the error is a close error with one of the specified codes
func IsCloseError(err error, codes ...int) bool {
	if closeErr, ok := err.(*ws.CloseError); ok {
		for _, code := range codes {
			if closeErr.Code == code {
				return true
			}
		}
	}
	return false
}