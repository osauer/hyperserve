// Package sse writes Server-Sent Events to ordinary net/http responses.
// Applications own heartbeat scheduling, authorization, event IDs, replay,
// and the lifetime of any work observed through a stream.
package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Writer encodes, writes, and flushes one frame at a time. Use it from one
// goroutine. A transport failure ends the stream: subsequent calls return the
// failure without writing again. Invalid input leaves the writer usable.
type Writer struct {
	response   http.ResponseWriter
	controller *http.ResponseController
	timeout    time.Duration
	err        error
}

// NewWriter prepares a stream with a positive per-frame write timeout. It does
// not write headers or start goroutines. The response must support flushing and
// write deadlines through http.ResponseController; unsupported operations are
// returned as errors from Send, SendJSON, or Comment. Middleware should expose
// its underlying response through Unwrap when it does not provide these methods.
//
// The first frame sets Content-Type and, if absent, Cache-Control: no-cache and
// X-Accel-Buffering: no. Set application-specific headers before sending it.
// Each frame clears its write deadline after flushing so an idle stream does
// not time out between frames. The application should observe r.Context() while
// waiting for data; a disconnected viewer need not cancel the work it observes.
func NewWriter(w http.ResponseWriter, writeTimeout time.Duration) (*Writer, error) {
	if w == nil || writeTimeout <= 0 {
		return nil, errors.New("sse: response and positive write timeout required")
	}
	return &Writer{response: w, controller: http.NewResponseController(w), timeout: writeTimeout}, nil
}

// Send writes UTF-8 data as one event, splitting CR, LF, or CRLF into data lines.
// Event and id must be UTF-8 without CR, LF, or NUL. An empty event uses the
// browser's default message type; an empty id leaves its last event ID unchanged.
// Invalid fields are rejected before any bytes are written.
func (w *Writer) Send(event, id string, data []byte) error {
	if w.err != nil {
		return w.err
	}
	if !validField(event) || !validField(id) || !utf8.Valid(data) {
		return errors.New("sse: invalid event, id, or UTF-8 data")
	}
	var frame strings.Builder
	if event != "" {
		frame.WriteString("event: " + event + "\n")
	}
	if id != "" {
		frame.WriteString("id: " + id + "\n")
	}
	writeLines(&frame, "data: ", string(data))
	frame.WriteByte('\n')
	return w.write(frame.String())
}

// SendJSON marshals data as JSON and sends it as one event. Encoding errors are
// returned before writing, including errors from custom JSON marshalers.
func (w *Writer) SendJSON(event, id string, data any) error {
	if w.err != nil {
		return w.err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return w.Send(event, id, encoded)
}

// Comment writes UTF-8 text as comment lines and flushes them. Comments do not
// dispatch browser events or change the last event ID; callers can use them for
// an initial connection acknowledgement and scheduled heartbeats.
func (w *Writer) Comment(text string) error {
	if w.err != nil {
		return w.err
	}
	if !utf8.ValidString(text) {
		return errors.New("sse: invalid UTF-8 comment")
	}
	var frame strings.Builder
	writeLines(&frame, ": ", text)
	frame.WriteByte('\n')
	return w.write(frame.String())
}

func validField(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsAny(value, "\r\n\x00")
}

func writeLines(frame *strings.Builder, prefix, text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for line := range strings.SplitSeq(text, "\n") {
		frame.WriteString(prefix)
		frame.WriteString(line)
		frame.WriteByte('\n')
	}
}

func (w *Writer) write(frame string) (err error) {
	if err = w.controller.SetWriteDeadline(time.Now().Add(w.timeout)); err != nil {
		w.err = fmt.Errorf("sse: set write deadline: %w", err)
		return w.err
	}
	defer func() {
		err = errors.Join(err, w.controller.SetWriteDeadline(time.Time{}))
		w.err = err
	}()
	headers := w.response.Header()
	headers.Set("Content-Type", "text/event-stream")
	if headers.Get("Cache-Control") == "" {
		headers.Set("Cache-Control", "no-cache")
	}
	if headers.Get("X-Accel-Buffering") == "" {
		headers.Set("X-Accel-Buffering", "no")
	}
	n, err := io.WriteString(w.response, frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return w.controller.Flush()
}
