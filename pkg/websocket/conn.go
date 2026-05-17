package websocket

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// lowConn represents a low-level WebSocket connection.
// It is wrapped by the public Conn type in websocket.go.
type lowConn struct {
	conn     net.Conn
	reader   *FrameReader
	writer   *FrameWriter
	isServer bool

	// Message assembly
	messageMu     sync.Mutex
	messageBuffer []byte
	messageType   int

	// Close handling
	closeMu   sync.Mutex
	closeSent bool
}

// newLowConn creates a new low-level WebSocket connection.
func newLowConn(netConn net.Conn, buf *bufio.ReadWriter, isServer bool, maxMessageSize int64) *lowConn {
	return &lowConn{
		conn:     netConn,
		reader:   NewFrameReader(buf.Reader, maxMessageSize),
		writer:   NewFrameWriter(buf.Writer, isServer),
		isServer: isServer,
	}
}

// ReadFrame reads the next frame from the connection
func (c *lowConn) ReadFrame() (*Frame, error) {
	return c.reader.ReadFrame()
}

// WriteFrame writes a frame to the connection
func (c *lowConn) WriteFrame(frame *Frame) error {
	return c.writer.WriteFrame(frame)
}

// ReadMessage reads a complete message (handling fragmentation)
func (c *lowConn) ReadMessage() (messageType int, data []byte, err error) {
	c.messageMu.Lock()
	defer c.messageMu.Unlock()

	for {
		frame, err := c.ReadFrame()
		if err != nil {
			return 0, nil, err
		}

		switch frame.Opcode {
		case OpcodeText, OpcodeBinary:
			// Start of new message
			if c.messageBuffer != nil {
				return 0, nil, errors.New("unexpected data frame")
			}
			c.messageType = frame.Opcode
			c.messageBuffer = frame.Payload

			if frame.Fin {
				// Complete message
				data := c.messageBuffer
				c.messageBuffer = nil
				return c.messageType, data, nil
			}

		case OpcodeContinuation:
			// Continuation of fragmented message
			if c.messageBuffer == nil {
				return 0, nil, ErrUnexpectedContinuation
			}
			c.messageBuffer = append(c.messageBuffer, frame.Payload...)

			if frame.Fin {
				// Message complete
				data := c.messageBuffer
				messageType := c.messageType
				c.messageBuffer = nil
				return messageType, data, nil
			}

		case OpcodeClose:
			// Handle close frame
			closeCode := CloseNoStatusReceived
			closeText := ""
			if len(frame.Payload) >= 2 {
				closeCode = int(binary.BigEndian.Uint16(frame.Payload[:2]))
				if len(frame.Payload) > 2 {
					closeText = string(frame.Payload[2:])
				}
			}

			// Send close response if we haven't already
			c.closeMu.Lock()
			if !c.closeSent {
				c.closeSent = true
				closeFrame := &Frame{
					Fin:     true,
					Opcode:  OpcodeClose,
					Payload: frame.Payload, // Echo the close code
				}
				_ = c.WriteFrame(closeFrame) // Best effort close frame
			}
			c.closeMu.Unlock()

			return 0, nil, &closeError{Code: closeCode, Text: closeText}

		case OpcodePing:
			// Respond with pong
			pongFrame := &Frame{
				Fin:     true,
				Opcode:  OpcodePong,
				Payload: frame.Payload,
			}
			if err := c.WriteFrame(pongFrame); err != nil {
				return 0, nil, err
			}

		case OpcodePong:
			// Pong received, no action needed
			continue

		default:
			return 0, nil, ErrInvalidFrame
		}
	}
}

// WriteMessage writes a complete message
func (c *lowConn) WriteMessage(messageType int, data []byte) error {
	if messageType != OpcodeText && messageType != OpcodeBinary {
		return errors.New("invalid message type")
	}

	frame := &Frame{
		Fin:     true,
		Opcode:  messageType,
		Payload: data,
		Masked:  !c.isServer,
	}

	// Generate mask key for client
	if !c.isServer {
		if _, err := rand.Read(frame.MaskKey[:]); err != nil {
			return err
		}
	}

	return c.WriteFrame(frame)
}

// WriteControl writes a control frame (close, ping, pong)
func (c *lowConn) WriteControl(opcode int, data []byte) error {
	if opcode < OpcodeClose || opcode > OpcodePong {
		return errors.New("invalid control opcode")
	}

	if len(data) > 125 {
		return ErrControlFrameTooBig
	}

	frame := &Frame{
		Fin:     true,
		Opcode:  opcode,
		Payload: data,
		Masked:  !c.isServer,
	}

	// Generate mask key for client
	if !c.isServer {
		if _, err := rand.Read(frame.MaskKey[:]); err != nil {
			return err
		}
	}

	return c.WriteFrame(frame)
}

// Close closes the WebSocket connection
func (c *lowConn) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if !c.closeSent {
		c.closeSent = true
		// Send close frame with normal closure
		closePayload := make([]byte, 2)
		binary.BigEndian.PutUint16(closePayload, uint16(CloseNormalClosure))
		_ = c.WriteControl(OpcodeClose, closePayload) // Best effort close notification
	}

	return c.conn.Close()
}

// SetDeadline sets the read and write deadlines
func (c *lowConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline
func (c *lowConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (c *lowConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// LocalAddr returns the local network address
func (c *lowConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address
func (c *lowConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// closeError represents a close frame error.
type closeError struct {
	Code int
	Text string
}

func (e *closeError) Error() string {
	return fmt.Sprintf("websocket: close %d %s", e.Code, e.Text)
}
