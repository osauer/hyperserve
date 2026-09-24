package sse_test

import (
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/sse"
)

type response struct {
	recorder    *httptest.ResponseRecorder
	deadlines   []time.Time
	flushes     int
	writeErr    error
	flushErr    error
	deadlineErr error
	resetErr    error
	short       bool
}

func newResponse() *response             { return &response{recorder: httptest.NewRecorder()} }
func (w *response) Header() http.Header  { return w.recorder.Header() }
func (w *response) WriteHeader(code int) { w.recorder.WriteHeader(code) }
func (w *response) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return w.recorder.Write(p[:len(p)/2])
	}
	return w.recorder.Write(p)
}
func (w *response) FlushError() error {
	w.flushes++
	if w.flushErr != nil {
		return w.flushErr
	}
	w.recorder.Flush()
	return nil
}
func (w *response) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	if deadline.IsZero() {
		return w.resetErr
	}
	return w.deadlineErr
}

type wrappedResponse struct{ http.ResponseWriter }

func (w wrappedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestFramesAndWrappedResponse(t *testing.T) {
	r := newResponse()
	r.Header().Set("Cache-Control", "no-store")
	w, err := sse.NewWriter(wrappedResponse{r}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		w.Comment("connected\r\nevent: not-an-event\rready"),
		w.Send("update", "42", []byte("one\r\ntwo\rthree\n")),
		w.SendJSON("", "", map[string]string{"value": "a\nb"}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := ": connected\n: event: not-an-event\n: ready\n\nevent: update\nid: 42\ndata: one\ndata: two\ndata: three\ndata: \n\ndata: {\"value\":\"a\\nb\"}\n\n"
	if got := r.recorder.Body.String(); got != want {
		t.Fatalf("frames:\n%q\nwant:\n%q", got, want)
	}
	if r.Header().Get("Content-Type") != "text/event-stream" || r.Header().Get("Cache-Control") != "no-store" || r.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("headers: %v", r.Header())
	}
	if r.flushes != 3 || len(r.deadlines) != 6 {
		t.Fatalf("flushes=%d deadlines=%v", r.flushes, r.deadlines)
	}
	for i, deadline := range r.deadlines {
		if deadline.IsZero() != (i%2 == 1) {
			t.Fatalf("deadline %d not set/cleared: %v", i, deadline)
		}
	}
}

func TestInvalidInputDoesNotCommitResponse(t *testing.T) {
	for _, send := range []func(*sse.Writer) error{
		func(w *sse.Writer) error { return w.Send("bad\nevent", "", nil) },
		func(w *sse.Writer) error { return w.Send("", "bad\rid", nil) },
		func(w *sse.Writer) error { return w.Send("", "bad\x00id", nil) },
		func(w *sse.Writer) error { return w.Send("\xff", "", nil) },
		func(w *sse.Writer) error { return w.Send("", "\xff", nil) },
		func(w *sse.Writer) error { return w.Send("", "", []byte{0xff}) },
		func(w *sse.Writer) error { return w.Comment("\xff") },
		func(w *sse.Writer) error { return w.SendJSON("", "", math.Inf(1)) },
	} {
		r := newResponse()
		w, err := sse.NewWriter(r, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := send(w); err == nil {
			t.Fatal("invalid input accepted")
		}
		if r.recorder.Body.Len() != 0 || len(r.Header()) != 0 || len(r.deadlines) != 0 {
			t.Fatal("invalid input started the response")
		}
		if err := w.Send("", "", nil); err != nil {
			t.Fatalf("valid event after input error: %v", err)
		}
		if got := r.recorder.Body.String(); got != "data: \n\n" {
			t.Fatalf("empty event: %q", got)
		}
	}
}

func TestTransportFailureStopsFurtherWrites(t *testing.T) {
	failure := errors.New("transport failed")
	resetFailure := errors.New("deadline reset failed")
	for _, tc := range []struct {
		name      string
		configure func(*response)
		want      error
	}{
		{"write", func(r *response) { r.writeErr = failure }, failure},
		{"short write", func(r *response) { r.short = true }, io.ErrShortWrite},
		{"flush", func(r *response) { r.flushErr = failure }, failure},
		{"deadline", func(r *response) { r.deadlineErr = failure }, failure},
		{"reset", func(r *response) { r.resetErr = resetFailure }, resetFailure},
		{"flush and reset", func(r *response) { r.flushErr, r.resetErr = failure, resetFailure }, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newResponse()
			tc.configure(r)
			w, err := sse.NewWriter(r, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			err = w.Send("", "", []byte("first"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if r.resetErr != nil && !errors.Is(err, resetFailure) {
				t.Fatalf("lost reset error: %v", err)
			}
			body, deadlines := r.recorder.Body.String(), len(r.deadlines)
			for _, next := range []error{w.Send("", "", nil), w.SendJSON("", "", nil), w.Comment("retry")} {
				if !errors.Is(next, tc.want) {
					t.Fatalf("lost terminal error: %v", next)
				}
			}
			if r.recorder.Body.String() != body || len(r.deadlines) != deadlines {
				t.Fatal("wrote after transport failure")
			}
		})
	}
}

func TestUnsupportedDeadlineAndInvalidTimeout(t *testing.T) {
	r := httptest.NewRecorder()
	for _, timeout := range []time.Duration{0, -time.Second} {
		if _, err := sse.NewWriter(r, timeout); err == nil {
			t.Fatal("accepted nonpositive timeout")
		}
	}
	if _, err := sse.NewWriter(nil, time.Second); err == nil {
		t.Fatal("accepted nil response")
	}
	w, err := sse.NewWriter(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Comment("connected"); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("unsupported deadline: %v", err)
	}
	if r.Body.Len() != 0 || len(r.Header()) != 0 {
		t.Fatal("wrote without deadline support")
	}
}

type pipeResponse struct {
	net.Conn
	header http.Header
}

func (w pipeResponse) Header() http.Header { return w.header }
func (w pipeResponse) WriteHeader(int)     {}
func (w pipeResponse) FlushError() error   { return nil }

func TestSlowReaderIsBounded(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	w, err := sse.NewWriter(pipeResponse{Conn: server, header: make(http.Header)}, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- w.Send("", "", []byte("reader never consumes this")) }()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("slow reader error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow reader was not bounded")
	}
}

func TestHTTP2IdleDoesNotExpireWriteDeadline(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w, err := sse.NewWriter(rw, 100*time.Millisecond)
		if err != nil {
			t.Error(err)
			return
		}
		if err := w.Comment("connected"); err != nil {
			t.Error(err)
			return
		}
		select {
		case <-r.Context().Done():
			t.Error("stream expired while idle")
			return
		case <-time.After(200 * time.Millisecond):
		}
		if err := w.Send("", "", []byte("after idle")); err != nil {
			t.Error(err)
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 {
		t.Fatalf("protocol = %s", response.Proto)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != ": connected\n\ndata: after idle\n\n" {
		t.Fatalf("body = %q, error = %v", body, err)
	}
}

func FuzzFrames(f *testing.F) {
	f.Add("update", "42", "first\r\nsecond\rthird\n")
	f.Add("bad\nevent", "id", "data")
	f.Fuzz(func(t *testing.T, event, id, data string) {
		r := newResponse()
		w, err := sse.NewWriter(r, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Send(event, id, []byte(data)); err != nil {
			if r.recorder.Body.Len() != 0 {
				t.Fatal("invalid frame wrote bytes")
			}
			return
		}
		frame := r.recorder.Body.String()
		if !strings.HasSuffix(frame, "\n\n") || strings.Count(frame, "\n\n") != 1 || strings.Contains(frame, "\r") {
			t.Fatalf("injected frame boundary: %q", frame)
		}
		for line := range strings.SplitSeq(strings.TrimSuffix(frame, "\n\n"), "\n") {
			if !strings.HasPrefix(line, "event: ") && !strings.HasPrefix(line, "id: ") && !strings.HasPrefix(line, "data: ") {
				t.Fatalf("unframed line: %q", line)
			}
		}
	})
}
