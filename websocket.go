package hyperserve

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const (
	// WebSocket magic string defined in RFC 6455
	websocketMagicString = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	// WebSocket version
	websocketVersion = "13"
)

// WebSocket message types
const (
	TextMessage   = 1
	BinaryMessage = 2
	CloseMessage  = 8
	PingMessage   = 9
	PongMessage   = 10
)

// WebSocket errors
var (
	ErrNotWebSocket = errors.New("not a websocket handshake")
	ErrBadHandshake = errors.New("bad handshake")
)

// Conn represents a WebSocket connection
type Conn struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	masked bool
}

// Upgrader upgrades HTTP connections to WebSocket connections
type Upgrader struct {
	// CheckOrigin returns true if the request Origin header is acceptable
	CheckOrigin func(r *http.Request) bool
}

// Upgrade upgrades an HTTP connection to a WebSocket connection
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	// Check if this is a WebSocket upgrade request
	if !isWebSocketUpgrade(r) {
		return nil, ErrNotWebSocket
	}

	// Check origin if function is provided
	if u.CheckOrigin != nil && !u.CheckOrigin(r) {
		return nil, ErrBadHandshake
	}

	// Get the connection using hijacker
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("responsewriter does not support hijacking")
	}

	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	// Generate accept key
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		conn.Close()
		return nil, ErrBadHandshake
	}

	acceptKey := generateAcceptKey(key)

	// Send upgrade response
	response := fmt.Sprintf(
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n"+
			"\r\n",
		acceptKey,
	)

	if _, err := conn.Write([]byte(response)); err != nil {
		conn.Close()
		return nil, err
	}

	return &Conn{
		conn:   conn,
		reader: buf.Reader,
		writer: buf.Writer,
		masked: false, // Server connections don't mask
	}, nil
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		r.Header.Get("Sec-WebSocket-Version") == websocketVersion &&
		r.Header.Get("Sec-WebSocket-Key") != ""
}

// generateAcceptKey generates the Sec-WebSocket-Accept header value
func generateAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte(websocketMagicString))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ReadMessage reads a message from the WebSocket connection
func (c *Conn) ReadMessage() (messageType int, p []byte, err error) {
	// Read frame header
	header := make([]byte, 2)
	if _, err = c.reader.Read(header); err != nil {
		return 0, nil, err
	}

	// Parse frame header
	fin := (header[0] & 0x80) != 0
	opcode := int(header[0] & 0x0F)
	masked := (header[1] & 0x80) != 0
	payloadLen := int(header[1] & 0x7F)

	// Handle extended payload length
	if payloadLen == 126 {
		extLen := make([]byte, 2)
		if _, err = c.reader.Read(extLen); err != nil {
			return 0, nil, err
		}
		payloadLen = int(binary.BigEndian.Uint16(extLen))
	} else if payloadLen == 127 {
		extLen := make([]byte, 8)
		if _, err = c.reader.Read(extLen); err != nil {
			return 0, nil, err
		}
		payloadLen = int(binary.BigEndian.Uint64(extLen))
	}

	// Read mask key if present
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err = c.reader.Read(maskKey); err != nil {
			return 0, nil, err
		}
	}

	// Read payload
	payload := make([]byte, payloadLen)
	if _, err = c.reader.Read(payload); err != nil {
		return 0, nil, err
	}

	// Unmask payload if needed
	if masked {
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	if !fin {
		return 0, nil, errors.New("fragmented frames not supported")
	}

	return opcode, payload, nil
}

// WriteMessage writes a message to the WebSocket connection
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	return c.writeFrame(messageType, data)
}

// writeFrame writes a WebSocket frame
func (c *Conn) writeFrame(opcode int, data []byte) error {
	dataLen := len(data)
	
	// Create frame header
	var header []byte
	
	// First byte: FIN=1, RSV=0, opcode
	header = append(header, byte(0x80|opcode))
	
	// Second byte and extended length
	if dataLen < 126 {
		header = append(header, byte(dataLen))
	} else if dataLen < 65536 {
		header = append(header, 126)
		lenBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBytes, uint16(dataLen))
		header = append(header, lenBytes...)
	} else {
		header = append(header, 127)
		lenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(lenBytes, uint64(dataLen))
		header = append(header, lenBytes...)
	}
	
	// Write header
	if _, err := c.writer.Write(header); err != nil {
		return err
	}
	
	// Write payload
	if _, err := c.writer.Write(data); err != nil {
		return err
	}
	
	return c.writer.Flush()
}

// Close closes the WebSocket connection
func (c *Conn) Close() error {
	// Send close frame
	c.writeFrame(CloseMessage, []byte{})
	return c.conn.Close()
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
	// Simplified implementation - in a real implementation you'd check for specific WebSocket close codes
	return err != nil && !strings.Contains(err.Error(), "use of closed network connection")
}