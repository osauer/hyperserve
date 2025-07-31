package hyperserve

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
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
			hijackable:        true,
		}
		
		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:    http.StatusOK,
			bytesWritten:  0,
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
			hijackable:        false,
		}
		
		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:    http.StatusOK,
			bytesWritten:  0,
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
			pushable:          true,
		}
		
		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:    http.StatusOK,
			bytesWritten:  0,
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
			pushable:          false,
		}
		
		lrw := &loggingResponseWriter{
			ResponseWriter: mock,
			statusCode:    http.StatusOK,
			bytesWritten:  0,
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
		statusCode:    http.StatusOK,
		bytesWritten:  0,
	}
	
	// httptest.ResponseRecorder implements Flusher
	if _, ok := lrw.ResponseWriter.(http.Flusher); !ok {
		t.Skip("Test ResponseWriter doesn't implement Flusher")
	}
	
	// Should not panic
	lrw.Flush()
}

func TestLoggingResponseWriterReadFrom(t *testing.T) {
	data := []byte("Hello, World!")
	reader := bytes.NewReader(data)
	
	recorder := httptest.NewRecorder()
	lrw := &loggingResponseWriter{
		ResponseWriter: recorder,
		statusCode:    http.StatusOK,
		bytesWritten:  0,
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
	// Create server with logging middleware
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	
	// Add logging middleware
	srv.AddMiddleware("*", RequestLoggerMiddleware)
	
	// Add WebSocket handler
	upgrader := Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	
	wsHandlerCalled := false
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
		wsHandlerCalled = true
	})
	
	// Create test server
	ts := httptest.NewServer(srv.mux)
	defer ts.Close()
	
	// Make WebSocket request
	req, _ := http.NewRequest("GET", ts.URL+"/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	
	if !wsHandlerCalled {
		t.Error("WebSocket handler was not called")
	}
}