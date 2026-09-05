package websocket

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

const defaultMaxMessageSize = 1024 * 1024

// Frame opcodes defined in RFC 6455
const (
	OpcodeContinuation = 0
	OpcodeText         = 1
	OpcodeBinary       = 2
	OpcodeClose        = 8
	OpcodePing         = 9
	OpcodePong         = 10
)

// Close codes defined in RFC 6455
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerError     = 1011
	CloseServiceRestart          = 1012
	CloseTryAgainLater           = 1013
	CloseTLSHandshake            = 1015
)

// Frame errors
var (
	ErrInvalidFrame           = errors.New("invalid frame")
	ErrControlFrameTooBig     = errors.New("control frame too big")
	ErrFragmentedControl      = errors.New("fragmented control frame")
	ErrUnexpectedContinuation = errors.New("unexpected continuation frame")
	ErrInvalidCloseCode       = errors.New("invalid close code")
	ErrInvalidUTF8            = errors.New("invalid UTF-8 in text frame")
	ErrMaskingViolation       = errors.New("invalid frame masking")
	ErrNonCanonicalLength     = errors.New("non-canonical payload length")
)

// Frame represents a WebSocket frame
type Frame struct {
	Fin     bool
	RSV1    bool
	RSV2    bool
	RSV3    bool
	Opcode  int
	Masked  bool
	Payload []byte
	MaskKey [4]byte
}

// FrameReader reads WebSocket frames
type FrameReader struct {
	reader         *bufio.Reader
	maxMessageSize int64
	scratch        [8]byte
}

// NewFrameReader creates a new frame reader
func NewFrameReader(r *bufio.Reader, maxMessageSize int64) *FrameReader {
	if maxMessageSize <= 0 {
		maxMessageSize = defaultMaxMessageSize
	}
	return &FrameReader{
		reader:         r,
		maxMessageSize: maxMessageSize,
	}
}

// ReadFrame reads a single frame from the reader
func (fr *FrameReader) ReadFrame() (*Frame, error) {
	frame := new(Frame)
	if err := fr.readFrame(frame, nil, false); err != nil {
		return nil, err
	}
	return frame, nil
}

// readFrame appends continuation payloads directly to the message accumulator.
// Public ReadFrame still returns an independently owned single-frame payload.
func (fr *FrameReader) readFrame(frame *Frame, message []byte, active bool) error {
	header := fr.scratch[:2]
	if _, err := io.ReadFull(fr.reader, header); err != nil {
		return err
	}
	*frame = Frame{
		Fin: header[0]&0x80 != 0, RSV1: header[0]&0x40 != 0,
		RSV2: header[0]&0x20 != 0, RSV3: header[0]&0x10 != 0,
		Opcode: int(header[0] & 0x0f), Masked: header[1]&0x80 != 0,
	}

	// Validate frame
	if err := frame.validate(); err != nil {
		return err
	}

	// Read payload length
	payloadLen := int64(header[1] & 0x7F)
	if payloadLen == 126 {
		// Extended 16-bit length
		if _, err := io.ReadFull(fr.reader, fr.scratch[:2]); err != nil {
			return err
		}
		payloadLen = int64(binary.BigEndian.Uint16(fr.scratch[:2]))
		if payloadLen < 126 {
			return ErrNonCanonicalLength
		}
	} else if payloadLen == 127 {
		// Extended 64-bit length
		if _, err := io.ReadFull(fr.reader, fr.scratch[:8]); err != nil {
			return err
		}
		len64 := binary.BigEndian.Uint64(fr.scratch[:8])
		// Check for int64 overflow
		if len64 > math.MaxInt64 {
			return errors.New("payload length exceeds maximum int64 value")
		}
		payloadLen = int64(len64)
		if payloadLen < 65536 {
			return ErrNonCanonicalLength
		}
	}

	// Check message size limit
	if payloadLen > fr.maxMessageSize {
		return ErrMessageTooBig
	}
	if frame.IsControl() && payloadLen > 125 {
		return ErrControlFrameTooBig
	}

	// Read mask key if present
	if frame.Masked {
		if _, err := io.ReadFull(fr.reader, fr.scratch[:4]); err != nil {
			return err
		}
		copy(frame.MaskKey[:], fr.scratch[:4])
	}

	start := 0
	if active && frame.Opcode == OpcodeContinuation {
		if payloadLen > fr.maxMessageSize-int64(len(message)) {
			return ErrMessageTooBig
		}
		start = len(message)
	}
	maxInt := int(^uint(0) >> 1)
	if payloadLen > int64(maxInt-start) {
		return ErrMessageTooBig
	}
	size := start + int(payloadLen)
	if active && frame.Opcode == OpcodeContinuation {
		if size > cap(message) {
			// Double within the configured limit instead of repeatedly copying
			// large messages through small capacity increments.
			capacity := maxInt
			if cap(message) <= maxInt/2 {
				capacity = 2 * cap(message)
			}
			capacity = max(size, capacity)
			if int64(capacity) > fr.maxMessageSize {
				capacity = int(fr.maxMessageSize)
			}
			grown := make([]byte, size, capacity)
			copy(grown, message)
			message = grown
		}
		frame.Payload = message[:size]
	} else if size > 0 {
		frame.Payload = make([]byte, size)
	}
	payload := frame.Payload[start:]
	if len(payload) > 0 {
		if _, err := io.ReadFull(fr.reader, payload); err != nil {
			return err
		}
		if frame.Masked {
			maskPayload(payload, frame.MaskKey)
		}
	}
	return nil
}

// FrameWriter writes WebSocket frames
type FrameWriter struct {
	writer   *bufio.Writer
	isServer bool
}

// NewFrameWriter creates a new frame writer
func NewFrameWriter(w *bufio.Writer, isServer bool) *FrameWriter {
	return &FrameWriter{
		writer:   w,
		isServer: isServer,
	}
}

// WriteFrame writes a frame to the writer
func (fw *FrameWriter) WriteFrame(frame *Frame) error {
	// Validate frame
	if err := frame.validate(); err != nil {
		return err
	}

	// Build header
	var header []byte

	// First byte: FIN, RSV, Opcode
	b0 := byte(frame.Opcode)
	if frame.Fin {
		b0 |= 0x80
	}
	if frame.RSV1 {
		b0 |= 0x40
	}
	if frame.RSV2 {
		b0 |= 0x20
	}
	if frame.RSV3 {
		b0 |= 0x10
	}
	header = append(header, b0)

	// Second byte: Mask flag and payload length
	payloadLen := len(frame.Payload)
	maskBit := byte(0)
	if !fw.isServer && frame.Masked {
		maskBit = 0x80
	}

	if payloadLen < 126 {
		header = append(header, maskBit|byte(payloadLen))
	} else if payloadLen < 65536 {
		header = append(header, maskBit|126)
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(payloadLen)) //nolint:gosec // payloadLen is already range-checked
		header = append(header, b...)
	} else {
		header = append(header, maskBit|127)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(payloadLen))
		header = append(header, b...)
	}

	// Write header
	if _, err := fw.writer.Write(header); err != nil {
		return err
	}

	// Write mask key if client
	if !fw.isServer && frame.Masked {
		if _, err := fw.writer.Write(frame.MaskKey[:]); err != nil {
			return err
		}
	}

	// Write payload
	if len(frame.Payload) > 0 {
		payload := frame.Payload
		if !fw.isServer && frame.Masked {
			// Mask payload for client
			payload = make([]byte, len(frame.Payload))
			copy(payload, frame.Payload)
			maskPayload(payload, frame.MaskKey)
		}
		if _, err := fw.writer.Write(payload); err != nil {
			return err
		}
	}

	return fw.writer.Flush()
}

// validate checks if the frame is valid according to RFC 6455
func (f *Frame) validate() error {
	if f.RSV1 || f.RSV2 || f.RSV3 {
		return ErrInvalidFrame
	}
	// Check for reserved opcodes
	if f.Opcode > 10 || (f.Opcode > 2 && f.Opcode < 8) {
		return ErrInvalidFrame
	}

	// Control frames must not be fragmented
	if f.IsControl() && !f.Fin {
		return ErrFragmentedControl
	}

	// Control frames must have payload <= 125 bytes
	if f.IsControl() && len(f.Payload) > 125 {
		return ErrControlFrameTooBig
	}

	return nil
}

// IsControl returns true if this is a control frame
func (f *Frame) IsControl() bool {
	return f.Opcode >= 8
}

// IsData returns true if this is a data frame
func (f *Frame) IsData() bool {
	return f.Opcode <= 2
}

// maskPayload applies XOR masking to the payload
func maskPayload(payload []byte, key [4]byte) {
	for i := range payload {
		payload[i] ^= key[i%4]
	}
}

// Error returned when message exceeds size limit
var ErrMessageTooBig = errors.New("message too big")
