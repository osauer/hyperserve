package websocket

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
)

func TestFragmentAccumulatorMaskingControlAndOwnership(t *testing.T) {
	var wire bytes.Buffer
	writer := NewFrameWriter(bufio.NewWriter(&wire), false)
	write := func(op int, fin bool, data []byte) {
		t.Helper()
		if err := writer.WriteFrame(&Frame{Opcode: op, Fin: fin, Masked: true, MaskKey: [4]byte{9, 7, 5, 3}, Payload: data}); err != nil {
			t.Fatal(err)
		}
	}
	var want []byte
	for i := range 65 {
		data := bytes.Repeat([]byte{byte(i), byte(i + 1), byte(i + 2)}, i+1)
		want = append(want, data...)
		op := OpcodeContinuation
		if i == 0 {
			op = OpcodeBinary
		}
		write(op, i == 64, data)
		if i == 20 {
			write(OpcodePing, true, []byte("ping"))
		}
	}
	write(OpcodeBinary, true, []byte("next"))
	c := &lowConn{reader: NewFrameReader(bufio.NewReader(&wire), int64(len(want))), isServer: true}
	pings := 0
	c.setPingHandler(func(s string) error {
		if s != "ping" {
			t.Errorf("ping = %q", s)
		}
		pings++
		return nil
	})
	typ, data, err := c.ReadMessage()
	if err != nil || typ != OpcodeBinary || !bytes.Equal(data, want) || pings != 1 {
		t.Fatalf("fragmented read: type=%d len=%d ping=%d err=%v", typ, len(data), pings, err)
	}
	_, next, err := c.ReadMessage()
	if err != nil || string(next) != "next" {
		t.Fatalf("next=%q err=%v", next, err)
	}
	next[0] = 'X'
	if !bytes.Equal(data, want) {
		t.Fatal("subsequent message modified retained payload")
	}
}

func TestFragmentLimitBeforeReadingContinuationPayload(t *testing.T) {
	// Two bytes of data, then a continuation declaring two more bytes. Its
	// payload is deliberately absent: the total limit must win over EOF.
	wire := []byte{0x02, 2, 'a', 'b', 0x80, 2}
	c := &lowConn{reader: NewFrameReader(bufio.NewReader(bytes.NewReader(wire)), 3)}
	_, _, err := c.ReadMessage()
	if !errors.Is(err, ErrMessageTooBig) {
		t.Fatalf("error = %v", err)
	}
	if c.messageBuffer != nil || c.messageActive {
		t.Fatal("oversized message retained")
	}
}

func TestFramePayloadsRemainIndependent(t *testing.T) {
	reader := NewFrameReader(bufio.NewReader(bytes.NewReader([]byte{0x82, 2, 'a', 'b', 0x82, 2, 'c', 'd'})), 10)
	first, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	second.Payload[0] = 'X'
	if string(first.Payload) != "ab" {
		t.Fatal("ReadFrame reused returned payload")
	}
}
