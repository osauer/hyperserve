package websocket

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
)

var performanceMessageSink []byte

func BenchmarkFragmentedRead(b *testing.B) {
	const size = 256 * 1024
	for _, parts := range []int{1, 64, 256} {
		b.Run(fmt.Sprintf("frames_%d", parts), func(b *testing.B) {
			var wire bytes.Buffer
			fw := NewFrameWriter(bufio.NewWriter(&wire), false)
			payload := make([]byte, size/parts)
			for i := range parts {
				op := OpcodeContinuation
				if i == 0 {
					op = OpcodeBinary
				}
				if err := fw.WriteFrame(&Frame{Fin: i == parts-1, Opcode: op, Masked: true, MaskKey: [4]byte{1, 2, 3, 4}, Payload: payload}); err != nil {
					b.Fatal(err)
				}
			}
			raw := wire.Bytes()
			var source bytes.Reader
			br := bufio.NewReader(&source)
			c := &lowConn{reader: NewFrameReader(br, size), isServer: true}
			b.ReportAllocs()
			b.SetBytes(size)
			for b.Loop() {
				source.Reset(raw)
				br.Reset(&source)
				_, data, err := c.ReadMessage()
				if err != nil || len(data) != size {
					b.Fatalf("read %d: %v", len(data), err)
				}
				performanceMessageSink = data
			}
		})
	}
}
