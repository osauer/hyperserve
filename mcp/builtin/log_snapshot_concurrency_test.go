package builtin

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

type blockedSnapshotJSON struct{ entered, release chan struct{} }

func (v blockedSnapshotJSON) MarshalJSON() ([]byte, error) {
	close(v.entered)
	<-v.release
	return []byte(`"ok"`), nil
}

func TestLogSnapshotAllowsWritesDuringEncoding(t *testing.T) {
	r := NewServerLogResource(2)
	v := blockedSnapshotJSON{make(chan struct{}), make(chan struct{})}
	defer close(v.release)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "first", 0)
	record.AddAttrs(slog.Any("value", v))
	if err := r.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		value, err := r.Read()
		if err == nil {
			var snapshot struct {
				Count     int  `json:"count"`
				Truncated bool `json:"truncated"`
			}
			err = json.Unmarshal([]byte(value.(string)), &snapshot)
			if snapshot.Count != 1 || snapshot.Truncated {
				t.Error("snapshot metadata changed during encoding")
			}
		}
		done <- err
	}()
	select {
	case <-v.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("encoding did not start")
	}
	wrote := make(chan error, 1)
	go func() {
		wrote <- r.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "second", 0))
	}()
	select {
	case err := <-wrote:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer blocked behind encoding")
	}
	// Release the encoder before waiting, while keeping deferred cleanup safe.
	v.release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
