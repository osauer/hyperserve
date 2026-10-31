package hyperserve

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osauer/hyperserve/v2/websocket"
)

// baseResponseWriter is a minimal ResponseWriter that doesn't implement optional interfaces
type baseResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBaseResponseWriter() *baseResponseWriter {
	return &baseResponseWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (b *baseResponseWriter) Header() http.Header {
	return b.header
}

func (b *baseResponseWriter) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(data)
}

func (b *baseResponseWriter) WriteHeader(status int) {
	b.status = status
}

// mockResponseWriter can optionally implement interfaces
type mockResponseWriter struct {
	*baseResponseWriter
	hijackable bool
	pushable   bool
}

func (m *mockResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !m.hijackable {
		return nil, nil, http.ErrNotSupported
	}
	// Return mock connection
	return &mockConn{}, bufio.NewReadWriter(bufio.NewReader(bytes.NewReader(nil)), bufio.NewWriter(io.Discard)), nil
}

func (m *mockResponseWriter) Push(target string, opts *http.PushOptions) error {
	if !m.pushable {
		return http.ErrNotSupported
	}
	return nil
}

func (m *mockResponseWriter) ReadFrom(r io.Reader) (n int64, err error) {
	// Simulate optimized file serving
	return io.Copy(m, r)
}

type mockConn struct {
	net.Conn
}

func (mc *mockConn) Close() error { return nil }

func TestLoggingResponseWriterInterfaces(t *testing.T) {
	// Test that interfaces are properly delegated through loggingResponseWriter

	t.Run("hijacker interface preserved", func(t *testing.T) {
		mock := &mockResponseWriter{
			baseResponseWriter: newBaseResponseWriter(),
			hijackable:         true,
		}

		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}

		// Hijacker method should work
		_, _, err := lrw.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v, want nil", err)
		}
	})

	t.Run("hijacker interface error when not available", func(t *testing.T) {
		mock := &mockResponseWriter{
			baseResponseWriter: newBaseResponseWriter(),
			hijackable:         false,
		}

		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}

		// Hijacker method should return error
		_, _, err := lrw.Hijack()
		if err == nil {
			t.Error("Hijack() error = nil, want error")
		}
	})

	t.Run("pusher interface preserved", func(t *testing.T) {
		mock := &mockResponseWriter{
			baseResponseWriter: newBaseResponseWriter(),
			pushable:           true,
		}

		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}

		// Push method should work
		err := lrw.Push("/test", nil)
		if err != nil {
			t.Errorf("Push() error = %v, want nil", err)
		}
	})

	t.Run("pusher interface error when not available", func(t *testing.T) {
		mock := &mockResponseWriter{
			baseResponseWriter: newBaseResponseWriter(),
			pushable:           false,
		}

		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}

		// Push method should return ErrNotSupported
		err := lrw.Push("/test", nil)
		if err != http.ErrNotSupported {
			t.Errorf("Push() error = %v, want ErrNotSupported", err)
		}
	})
}

func TestLoggingResponseWriterFlusher(t *testing.T) {
	// Test that Flush is properly delegated
	recorder := httptest.NewRecorder()
	lrw := &loggingResponseWriter{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
		bytesWritten:   0,
	}

	// httptest.ResponseRecorder implements Flusher
	if _, ok := lrw.ResponseWriter.(http.Flusher); !ok {
		t.Skip("Test ResponseWriter doesn't implement Flusher")
	}

	// Should not panic
	lrw.Flush()
	if !recorder.Flushed {
		t.Fatal("legacy Flush did not reach the underlying writer")
	}
}

type flushErrorResponseWriter struct {
	*baseResponseWriter
	flushErr      error
	flushes       int
	legacyFlushes int
	deadline      time.Time
}

func (w *flushErrorResponseWriter) FlushError() error {
	w.flushes++
	return w.flushErr
}

func (w *flushErrorResponseWriter) Flush() { w.legacyFlushes++ }

func (w *flushErrorResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

type unwrapResponseWriter struct{ http.ResponseWriter }

func (w unwrapResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestRequestLoggerPreservesResponseController(t *testing.T) {
	wantErr := errors.New("stream flush failed")
	deadline := time.Now().Add(5 * time.Second)
	for _, level := range []slog.Level{slog.LevelWarn, slog.LevelInfo, slog.LevelDebug} {
		for _, wrapped := range []bool{false, true} {
			name := level.String() + "/direct"
			if wrapped {
				name = level.String() + "/unwrapped"
			}
			t.Run(name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: level}))
				srv, err := New(WithLogger(logger))
				if err != nil {
					t.Fatal(err)
				}
				srv.HandleFunc("/events", func(w http.ResponseWriter, _ *http.Request) {
					controller := http.NewResponseController(w)
					if err := controller.SetWriteDeadline(deadline); err != nil {
						t.Errorf("set deadline: %v", err)
					}
					w.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(w, "event")
					if err := controller.Flush(); !errors.Is(err, wantErr) {
						t.Errorf("flush error = %v, want %v", err, wantErr)
					}
				})
				base := &flushErrorResponseWriter{baseResponseWriter: newBaseResponseWriter(), flushErr: wantErr}
				var writer http.ResponseWriter = base
				if wrapped {
					writer = unwrapResponseWriter{writer}
				}
				srv.Handler().ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/events", nil))
				if base.flushes != 1 || base.legacyFlushes != 0 || !base.deadline.Equal(deadline) {
					t.Errorf("controller calls changed: error flushes=%d, legacy flushes=%d, deadline=%v", base.flushes, base.legacyFlushes, base.deadline)
				}
				if base.body.String() != "event" || base.status != http.StatusAccepted {
					t.Errorf("response changed: body=%q, status=%d", base.body.String(), base.status)
				}
				if level <= slog.LevelInfo {
					record := requestLogRecord(t, &logs)
					if record["status"] != float64(http.StatusAccepted) || record["bytes"] != float64(5) {
						t.Errorf("request accounting changed: %v", record)
					}
				}
			})
		}
	}
}

func TestLoggingResponseWriterReportsUnsupportedFlush(t *testing.T) {
	writer := &loggingResponseWriter{ResponseWriter: newBaseResponseWriter()}
	if err := http.NewResponseController(writer).Flush(); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("flush error = %v, want ErrNotSupported", err)
	}
}

func TestLoggingResponseWriterUnwrap(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := &loggingResponseWriter{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
		bytesWritten:   0,
	}

	if got := lrw.Unwrap(); got != recorder {
		t.Fatalf("Unwrap() = %T, want %T", got, recorder)
	}
}

func TestLoggingResponseWriterReadFrom(t *testing.T) {
	data := []byte("Hello, World!")
	reader := bytes.NewReader(data)

	recorder := httptest.NewRecorder()
	lrw := &loggingResponseWriter{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
		bytesWritten:   0,
	}

	// Test ReadFrom
	n, err := lrw.ReadFrom(reader)
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("ReadFrom() n = %v, want %v", n, len(data))
	}

	if lrw.bytesWritten != len(data) {
		t.Errorf("bytesWritten = %v, want %v", lrw.bytesWritten, len(data))
	}

	if recorder.Body.String() != string(data) {
		t.Errorf("Body = %v, want %v", recorder.Body.String(), string(data))
	}
}

func TestMiddlewareWithWebSocket(t *testing.T) {
	// New includes request logging in its default middleware stack.
	srv, err := New()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add WebSocket handler
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	var wsHandlerCalled atomic.Bool
	srv.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// Verify that w can be hijacked through the middleware
		if _, ok := w.(http.Hijacker); !ok {
			t.Error("ResponseWriter doesn't implement Hijacker after middleware")
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade failed: %v", err)
			return
		}
		conn.Close()
		wsHandlerCalled.Store(true)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping websocket middleware test: %v", err)
		return
	}

	middlewareDone := make(chan struct{})
	handler := srv.Handler()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		close(middlewareDone)
	})}
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	baseURL := "http://" + listener.Addr().String()
	defer func() {
		server.Close()
		<-done
	}()

	// Make WebSocket request
	req, _ := http.NewRequest("GET", baseURL+"/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	if got := resp.Header.Get("Upgrade"); got != "websocket" {
		t.Errorf("Upgrade header = %q, want websocket", got)
	}

	// The handler stores the atomic after writing the response, so a short
	// wait avoids racing the test assertion against the server goroutine.
	deadline := time.Now().Add(500 * time.Millisecond)
	for !wsHandlerCalled.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !wsHandlerCalled.Load() {
		t.Error("WebSocket handler was not called")
	}
	select {
	case <-middlewareDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("middleware did not finish after WebSocket upgrade")
	}
	if got := srv.totalRequests.Load(); got != 1 {
		t.Errorf("middleware request count = %d, want 1", got)
	}
}
