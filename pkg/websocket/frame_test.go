package websocket

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
)

func TestFrameValidation(t *testing.T) {
	tests := []struct {
		name    string
		frame   Frame
		wantErr bool
	}{
		{
			name: "valid text frame",
			frame: Frame{
				Fin:    true,
				Opcode: OpcodeText,
			},
			wantErr: false,
		},
		{
			name: "invalid opcode",
			frame: Frame{
				Fin:    true,
				Opcode: 15, // Invalid
			},
			wantErr: true,
		},
		{
			name: "fragmented control frame",
			frame: Frame{
				Fin:    false,
				Opcode: OpcodeClose,
			},
			wantErr: true,
		},
		{
			name: "control frame too large",
			frame: Frame{
				Fin:     true,
				Opcode:  OpcodePing,
				Payload: make([]byte, 126),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.frame.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFrameReaderWriter(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
	}{
		{
			name: "text message",
			frame: Frame{
				Fin:     true,
				Opcode:  OpcodeText,
				Payload: []byte("Hello, WebSocket!"),
			},
		},
		{
			name: "binary message",
			frame: Frame{
				Fin:     true,
				Opcode:  OpcodeBinary,
				Payload: []byte{0x01, 0x02, 0x03, 0x04},
			},
		},
		{
			name: "ping frame",
			frame: Frame{
				Fin:     true,
				Opcode:  OpcodePing,
				Payload: []byte("ping"),
			},
		},
		{
			name: "close frame",
			frame: Frame{
				Fin:     true,
				Opcode:  OpcodeClose,
				Payload: []byte{0x03, 0xe8}, // 1000 - Normal closure
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			bw := bufio.NewWriter(&buf)
			br := bufio.NewReader(&buf)

			// Write frame
			fw := NewFrameWriter(bw, true) // Server mode
			if err := fw.WriteFrame(&tt.frame); err != nil {
				t.Fatalf("WriteFrame() error = %v", err)
			}

			// Read frame back
			fr := NewFrameReader(br, 1024*1024)
			readFrame, err := fr.ReadFrame()
			if err != nil {
				t.Fatalf("ReadFrame() error = %v", err)
			}

			// Compare
			if readFrame.Fin != tt.frame.Fin {
				t.Errorf("Fin = %v, want %v", readFrame.Fin, tt.frame.Fin)
			}
			if readFrame.Opcode != tt.frame.Opcode {
				t.Errorf("Opcode = %v, want %v", readFrame.Opcode, tt.frame.Opcode)
			}
			if !bytes.Equal(readFrame.Payload, tt.frame.Payload) {
				t.Errorf("Payload = %v, want %v", readFrame.Payload, tt.frame.Payload)
			}
		})
	}
}

func TestMasking(t *testing.T) {
	payload := []byte("Hello, World!")
	key := [4]byte{0x37, 0xfa, 0x21, 0x3d}

	// Mask the payload
	masked := make([]byte, len(payload))
	copy(masked, payload)
	maskPayload(masked, key)

	// Verify it's different
	if bytes.Equal(masked, payload) {
		t.Error("Masking didn't change the payload")
	}

	// Unmask should restore original
	maskPayload(masked, key)
	if !bytes.Equal(masked, payload) {
		t.Error("Unmasking didn't restore original payload")
	}
}

func TestExtendedPayloadLength(t *testing.T) {
	tests := []struct {
		name       string
		payloadLen int
	}{
		{"small payload", 125},
		{"16-bit length", 126},
		{"16-bit length max", 65535},
		{"64-bit length", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := Frame{
				Fin:     true,
				Opcode:  OpcodeBinary,
				Payload: make([]byte, tt.payloadLen),
			}

			var buf bytes.Buffer
			bw := bufio.NewWriter(&buf)
			br := bufio.NewReader(&buf)

			// Write and read back
			fw := NewFrameWriter(bw, true)
			if err := fw.WriteFrame(&frame); err != nil {
				t.Fatalf("WriteFrame() error = %v", err)
			}

			fr := NewFrameReader(br, 10*1024*1024)
			readFrame, err := fr.ReadFrame()
			if err != nil {
				t.Fatalf("ReadFrame() error = %v", err)
			}

			if len(readFrame.Payload) != tt.payloadLen {
				t.Errorf("Payload length = %v, want %v", len(readFrame.Payload), tt.payloadLen)
			}
		})
	}
}

func TestFrameReaderRejectsOversizedControlLength(t *testing.T) {
	t.Parallel()
	// A ping encoded with a 16-bit payload length is invalid even before the
	// payload arrives: control frames are limited to 125 bytes.
	reader := NewFrameReader(bufio.NewReader(bytes.NewReader([]byte{0x89, 0x7e, 0x00, 0x7e})), defaultMaxMessageSize)
	if _, err := reader.ReadFrame(); !errors.Is(err, ErrControlFrameTooBig) {
		t.Fatalf("ReadFrame() error = %v, want ErrControlFrameTooBig", err)
	}
}

func TestFrameReaderRejectsPayloadLengthOverflow(t *testing.T) {
	t.Parallel()
	// RFC 6455 reserves the high bit of a 64-bit payload length. Treating this
	// value as int64 without checking would wrap it negative before allocation.
	encoded := []byte{0x82, 0x7f, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	reader := NewFrameReader(bufio.NewReader(bytes.NewReader(encoded)), defaultMaxMessageSize)
	_, err := reader.ReadFrame()
	if err == nil {
		t.Fatal("ReadFrame() error = nil, want payload-length overflow rejection")
	}
	if got, want := err.Error(), "payload length exceeds maximum int64 value"; got != want {
		t.Fatalf("ReadFrame() error = %q, want %q", got, want)
	}
}
