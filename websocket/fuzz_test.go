package websocket

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

// FuzzWebSocketFrameParse feeds arbitrary bytes into the frame reader.
// Any panic on attacker-controlled data is a real bug; ReadFrame should
// always return either a frame or an error.
func FuzzWebSocketFrameParse(f *testing.F) {
	// Seed: a couple of well-formed frames and short edge cases.
	seeds := [][]byte{
		// Empty text frame, FIN set, no mask, no payload.
		{0x81, 0x00},
		// Text frame "hi" (len=2), unmasked.
		{0x81, 0x02, 'h', 'i'},
		// Close frame with 1000 (normal).
		{0x88, 0x02, 0x03, 0xe8},
		// Fragmented binary message with an interleaved ping.
		{0x02, 2, 1, 2, 0x89, 0, 0x80, 2, 3, 4},
		// Truncated header.
		{0x81},
		// Empty.
		{},
		// Ping with payload "ab".
		{0x89, 0x02, 'a', 'b'},
		// Masked text "ab" — mask key 0x12345678, payload XOR'd.
		{0x81, 0x82, 0x12, 0x34, 0x56, 0x78, 'a' ^ 0x12, 'b' ^ 0x34},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bufio.NewReader(bytes.NewReader(data))
		fr := NewFrameReader(reader, 1<<16)
		// Don't care about correctness — just that we don't panic.
		_, _ = fr.ReadFrame()
		// Exercise accumulation and interleaved controls with the same bound.
		reader.Reset(bytes.NewReader(data))
		conn := &lowConn{
			reader: NewFrameReader(reader, 1<<16),
			writer: NewFrameWriter(bufio.NewWriter(io.Discard), false),
		}
		_, _, _ = conn.ReadMessage()
	})
}
