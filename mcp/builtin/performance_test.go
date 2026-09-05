package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

var performanceLogSink any

func BenchmarkLogSnapshot(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			r := NewServerLogResource(n)
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "request", 0)
			record.AddAttrs(slog.String("payload", strings.Repeat("x", 1024)))
			for range n {
				if err := r.Handle(context.Background(), record); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				data, err := r.Read()
				if err != nil {
					b.Fatal(err)
				}
				performanceLogSink = data
			}
		})
	}
}
