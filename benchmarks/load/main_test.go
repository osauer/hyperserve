package main

import (
	"testing"
	"time"
)

func TestValidateInputs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		target   string
		duration time.Duration
		workers  int
		wantErr  bool
	}{
		{name: "valid", target: "http://127.0.0.1:8080/path", duration: time.Second, workers: 1},
		{name: "relative URL", target: "/path", duration: time.Second, workers: 1, wantErr: true},
		{name: "unsupported scheme", target: "file:///tmp/result", duration: time.Second, workers: 1, wantErr: true},
		{name: "zero duration", target: "http://localhost", workers: 1, wantErr: true},
		{name: "zero workers", target: "http://localhost", duration: time.Second, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputs(tt.target, tt.duration, tt.workers)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateInputs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPercentileUsesHistogramBounds(t *testing.T) {
	stats := newWorkerStats()
	stats.recordResponse(200, 2, 90*time.Microsecond, nil)
	stats.recordResponse(200, 2, 200*time.Microsecond, nil)
	stats.recordResponse(200, 2, 2*time.Millisecond, nil)
	stats.recordResponse(503, 0, 6*time.Second, nil)

	if got := stats.percentile(0.50); got != "≤250µs" {
		t.Fatalf("p50 = %q, want ≤250µs", got)
	}
	if got := stats.percentile(0.95); got != ">5s" {
		t.Fatalf("p95 = %q, want >5s", got)
	}
	if stats.succeeded != 3 || stats.failed != 1 {
		t.Fatalf("results = %d succeeded, %d failed; want 3, 1", stats.succeeded, stats.failed)
	}
}
