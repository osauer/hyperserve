// Package responsewriter provides utilities for wrapping http.ResponseWriter
// while preserving optional interfaces like Hijacker, Flusher, and ReaderFrom.
package responsewriter

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// Wrapper is an interface that all ResponseWriter wrappers should implement
type Wrapper interface {
	http.ResponseWriter
	// Unwrap returns the original ResponseWriter
	Unwrap() http.ResponseWriter
}

// WrapperBase provides a base implementation for ResponseWriter wrappers
// that preserves optional interfaces (Hijacker, Flusher, ReaderFrom)
type WrapperBase struct {
	http.ResponseWriter
}

// Unwrap returns the original ResponseWriter
func (w *WrapperBase) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Hijack implements http.Hijacker interface if the underlying ResponseWriter supports it
func (w *WrapperBase) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Flush implements http.Flusher interface if the underlying ResponseWriter supports it
func (w *WrapperBase) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

// ReadFrom implements io.ReaderFrom interface if the underlying ResponseWriter supports it
func (w *WrapperBase) ReadFrom(r io.Reader) (n int64, err error) {
	rf, ok := w.ResponseWriter.(io.ReaderFrom)
	if !ok {
		// Fall back to default behavior
		return io.Copy(w.ResponseWriter, r)
	}
	return rf.ReadFrom(r)
}

// Push implements http.Pusher interface if the underlying ResponseWriter supports it
func (w *WrapperBase) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

// CloseNotify implements the deprecated http.CloseNotifier interface if the underlying ResponseWriter supports it
// Deprecated: Use Request.Context() instead
func (w *WrapperBase) CloseNotify() <-chan bool {
	notifier, ok := w.ResponseWriter.(http.CloseNotifier)
	if !ok {
		return nil
	}
	return notifier.CloseNotify()
}

// Ensure WrapperBase implements all optional interfaces
var (
	_ http.Hijacker      = (*WrapperBase)(nil)
	_ http.Flusher       = (*WrapperBase)(nil)
	_ io.ReaderFrom      = (*WrapperBase)(nil)
	_ http.Pusher        = (*WrapperBase)(nil)
	_ http.CloseNotifier = (*WrapperBase)(nil)
)