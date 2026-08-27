package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type streamTestSubscription struct {
	uri  string
	emit ResourceEmitter
}

type streamTestTemplate struct {
	quoteTemplate
	started         chan streamTestSubscription
	finished        chan string
	completeOnStart bool
}

type updateThenCompleteTemplate struct{ quoteTemplate }

func (t *updateThenCompleteTemplate) Subscribe(_ context.Context, uri string, _ map[string]string, emit ResourceEmitter) error {
	return emit.Update(uri)
}

func newStreamTestTemplate() *streamTestTemplate {
	return &streamTestTemplate{
		started:  make(chan streamTestSubscription, 16),
		finished: make(chan string, 16),
	}
}

func (t *streamTestTemplate) Subscribe(ctx context.Context, uri string, _ map[string]string, emit ResourceEmitter) error {
	t.started <- streamTestSubscription{uri: uri, emit: emit}
	defer func() { t.finished <- uri }()
	if t.completeOnStart {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

type sseFrame struct {
	event string
	data  string
	raw   string
}

func openListen(t *testing.T, ctx context.Context, url string, id any, notifications map[string]any) (*http.Response, *bufio.Reader) {
	t.Helper()
	params := map[string]any{
		"_meta":         map[string]any{protocolVersionMetaKey: StreamableHTTPProtocolVersion},
		"notifications": notifications,
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "subscriptions/listen",
		"params":  params,
		"id":      id,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerProtocolVersion, StreamableHTTPProtocolVersion)
	req.Header.Set(headerMethod, "subscriptions/listen")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open subscriptions/listen: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("subscriptions/listen status = %d; body=%s", resp.StatusCode, data)
	}
	return resp, bufio.NewReader(resp.Body)
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) sseFrame {
	t.Helper()
	type result struct {
		frame sseFrame
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		var frame sseFrame
		var raw strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			raw.WriteString(line)
			trimmed := strings.TrimSuffix(line, "\n")
			if trimmed == "" {
				frame.raw = raw.String()
				resultCh <- result{frame: frame}
				return
			}
			if value, ok := strings.CutPrefix(trimmed, "event: "); ok {
				frame.event = value
			}
			if value, ok := strings.CutPrefix(trimmed, "data: "); ok {
				frame.data = value
			}
		}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("read SSE frame: %v", result.err)
		}
		return result.frame
	case <-time.After(2 * time.Second):
		t.Fatal("timed out reading SSE frame")
		return sseFrame{}
	}
}

func decodeSSEData(t *testing.T, frame sseFrame) map[string]any {
	t.Helper()
	var message map[string]any
	if err := json.Unmarshal([]byte(frame.data), &message); err != nil {
		t.Fatalf("SSE data is not JSON: %v; frame=%q", err, frame.raw)
	}
	return message
}

func TestSubscriptionsListenAcknowledgesFirstAndDeliversResourceUpdates(t *testing.T) {
	h := newHandlerForTest(t)
	template := newStreamTestTemplate()
	h.RegisterResourceTemplate(template)
	server := httptest.NewServer(h)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	resp, reader := openListen(t, ctx, server.URL, "listen-1", map[string]any{
		"resourceSubscriptions": []string{"quotes://AAPL"},
	})
	defer resp.Body.Close()

	ackFrame := readSSEFrame(t, reader)
	if ackFrame.event != "message" || strings.Contains(ackFrame.raw, "id:") || !strings.HasSuffix(ackFrame.raw, "\n\n") {
		t.Fatalf("ack frame = %q, want exact message/data framing without id", ackFrame.raw)
	}
	ack := decodeSSEData(t, ackFrame)
	if ack["method"] != "notifications/subscriptions/acknowledged" {
		t.Fatalf("first message = %v, want acknowledgement", ack)
	}

	started := <-template.started
	if err := started.emit.Update("quotes://AAPL"); err != nil {
		t.Fatalf("emit update: %v", err)
	}
	updateFrame := readSSEFrame(t, reader)
	if updateFrame.event != "message" || strings.Contains(updateFrame.raw, "id:") {
		t.Fatalf("update frame = %q", updateFrame.raw)
	}
	update := decodeSSEData(t, updateFrame)
	if update["method"] != "notifications/resources/updated" {
		t.Fatalf("update = %v", update)
	}
	params := update["params"].(map[string]any)
	meta := params["_meta"].(map[string]any)
	if meta[subscriptionIDMetaKey] != "listen-1" {
		t.Fatalf("subscription id = %v, want listen-1", meta[subscriptionIDMetaKey])
	}

	cancel()
	resp.Body.Close()
	select {
	case <-template.finished:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not cancel resource producer")
	}
	if err := started.emit.Update("quotes://AAPL"); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-disconnect update error = %v, want context.Canceled", err)
	}
}

func TestSubscriptionsListenDeduplicatesAndOmitsUnsupportedFilters(t *testing.T) {
	h := newHandlerForTest(t)
	template := newStreamTestTemplate()
	h.RegisterResourceTemplate(template)
	server := httptest.NewServer(h)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	resp, reader := openListen(t, ctx, server.URL, 42, map[string]any{
		"toolsListChanged":      true,
		"promptsListChanged":    true,
		"resourcesListChanged":  true,
		"resourceSubscriptions": []string{"quotes://AAPL", "quotes://AAPL", "missing://x"},
	})
	defer resp.Body.Close()
	ack := decodeSSEData(t, readSSEFrame(t, reader))
	params := ack["params"].(map[string]any)
	notifications := params["notifications"].(map[string]any)
	if len(notifications) != 1 {
		t.Fatalf("acknowledged unsupported filters: %v", notifications)
	}
	resources := notifications["resourceSubscriptions"].([]any)
	if len(resources) != 1 || resources[0] != "quotes://AAPL" {
		t.Fatalf("acknowledged resources = %v", resources)
	}
	cancel()
}

func TestSubscriptionsListenNaturalAndShutdownCompletion(t *testing.T) {
	t.Run("natural completion", func(t *testing.T) {
		h := newHandlerForTest(t)
		template := newStreamTestTemplate()
		template.completeOnStart = true
		h.RegisterResourceTemplate(template)
		server := httptest.NewServer(h)
		defer server.Close()

		resp, reader := openListen(t, context.Background(), server.URL, "natural", map[string]any{
			"resourceSubscriptions": []string{"quotes://AAPL"},
		})
		defer resp.Body.Close()
		decodeSSEData(t, readSSEFrame(t, reader))
		complete := decodeSSEData(t, readSSEFrame(t, reader))
		if complete["id"] != "natural" || complete["result"].(map[string]any)["resultType"] != "complete" {
			t.Fatalf("completion = %v", complete)
		}
	})

	t.Run("final queued update precedes completion", func(t *testing.T) {
		h := newHandlerForTest(t)
		h.RegisterResourceTemplate(&updateThenCompleteTemplate{})
		server := httptest.NewServer(h)
		defer server.Close()

		resp, reader := openListen(t, context.Background(), server.URL, "ordered", map[string]any{
			"resourceSubscriptions": []string{"quotes://AAPL"},
		})
		defer resp.Body.Close()
		decodeSSEData(t, readSSEFrame(t, reader))
		update := decodeSSEData(t, readSSEFrame(t, reader))
		if update["method"] != "notifications/resources/updated" {
			t.Fatalf("message before completion = %v, want resource update", update)
		}
		complete := decodeSSEData(t, readSSEFrame(t, reader))
		if complete["id"] != "ordered" || complete["result"].(map[string]any)["resultType"] != "complete" {
			t.Fatalf("completion = %v", complete)
		}
	})

	t.Run("handler shutdown", func(t *testing.T) {
		h := newHandlerForTest(t)
		template := newStreamTestTemplate()
		h.RegisterResourceTemplate(template)
		server := httptest.NewServer(h)
		defer server.Close()

		resp, reader := openListen(t, context.Background(), server.URL, "shutdown", map[string]any{
			"resourceSubscriptions": []string{"quotes://AAPL"},
		})
		defer resp.Body.Close()
		decodeSSEData(t, readSSEFrame(t, reader))
		<-template.started

		done := make(chan error, 1)
		go func() { done <- h.Shutdown(context.Background()) }()
		complete := decodeSSEData(t, readSSEFrame(t, reader))
		if complete["result"].(map[string]any)["resultType"] != "complete" {
			t.Fatalf("completion = %v", complete)
		}
		if err := <-done; err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		if err := h.Shutdown(context.Background()); err != nil {
			t.Fatalf("second Shutdown: %v", err)
		}

		legacy := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		legacy.Header.Set("Accept", "text/event-stream")
		legacyRec := httptest.NewRecorder()
		h.ServeHTTP(legacyRec, legacy)
		if legacyRec.Code != http.StatusServiceUnavailable {
			t.Fatalf("post-shutdown legacy admission status = %d, want 503", legacyRec.Code)
		}
	})
}

func TestSubscriptionsListenSupportsSimultaneousStreams(t *testing.T) {
	h := newHandlerForTest(t)
	template := newStreamTestTemplate()
	h.RegisterResourceTemplate(template)
	server := httptest.NewServer(h)
	defer server.Close()

	ctx1 := t.Context()
	ctx2 := t.Context()
	resp1, reader1 := openListen(t, ctx1, server.URL, "one", map[string]any{"resourceSubscriptions": []string{"quotes://AAPL"}})
	defer resp1.Body.Close()
	resp2, reader2 := openListen(t, ctx2, server.URL, "two", map[string]any{"resourceSubscriptions": []string{"quotes://MSFT"}})
	defer resp2.Body.Close()
	decodeSSEData(t, readSSEFrame(t, reader1))
	decodeSSEData(t, readSSEFrame(t, reader2))

	started := map[string]ResourceEmitter{}
	for range 2 {
		sub := <-template.started
		started[sub.uri] = sub.emit
	}
	if err := started["quotes://AAPL"].Update("quotes://AAPL"); err != nil {
		t.Fatal(err)
	}
	if err := started["quotes://MSFT"].Update("quotes://MSFT"); err != nil {
		t.Fatal(err)
	}
	for reader, want := range map[*bufio.Reader]string{reader1: "one", reader2: "two"} {
		message := decodeSSEData(t, readSSEFrame(t, reader))
		params := message["params"].(map[string]any)
		if params["_meta"].(map[string]any)[subscriptionIDMetaKey] != want {
			t.Fatalf("message = %v, want subscription %s", message, want)
		}
	}
}

func TestSubscriptionsListenKeepaliveAndLimits(t *testing.T) {
	h := newHandlerForTest(t)
	h.streamKeepalive = 10 * time.Millisecond
	template := newStreamTestTemplate()
	h.RegisterResourceTemplate(template)
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	resp, reader := openListen(t, ctx, server.URL, "keepalive", map[string]any{
		"resourceSubscriptions": []string{"quotes://AAPL"},
	})
	defer resp.Body.Close()
	decodeSSEData(t, readSSEFrame(t, reader))
	<-template.started
	if frame := readSSEFrame(t, reader); frame.raw != ": keepalive\n\n" {
		t.Fatalf("keepalive frame = %q", frame.raw)
	}
	cancel()

	resources := make([]string, streamableSSEMaxSubscriptions+1)
	for i := range resources {
		resources[i] = "quotes://AAPL"
	}
	req := newStreamableRequest(t, "subscriptions/listen", map[string]any{
		"notifications": map[string]any{"resourceSubscriptions": resources},
	}, "too-many")
	rec := serveStreamable(t, h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionsListenWithoutMatchedProducersCompletes(t *testing.T) {
	h := newHandlerForTest(t)
	server := httptest.NewServer(h)
	defer server.Close()
	resp, reader := openListen(t, context.Background(), server.URL, "none", map[string]any{
		"toolsListChanged":      true,
		"resourceSubscriptions": []string{"missing://resource"},
	})
	defer resp.Body.Close()
	decodeSSEData(t, readSSEFrame(t, reader))
	complete := decodeSSEData(t, readSSEFrame(t, reader))
	if complete["id"] != "none" || complete["result"].(map[string]any)["resultType"] != "complete" {
		t.Fatalf("completion = %v", complete)
	}
}

func TestStreamableSSEQueueBlocksUntilCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := newStreamableSSE(ctx, httptest.NewRecorder(), "id", ServerInfo{}, nil, nil, time.Second, time.Second)
	emitter := &streamableResourceEmitter{stream: stream}
	for range streamableSSEQueueSize {
		if err := emitter.Update("quotes://AAPL"); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- emitter.Update("quotes://AAPL") }()
	select {
	case err := <-done:
		t.Fatalf("queue did not block: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked producer error = %v, want context.Canceled", err)
	}
}

type deadlineWriter struct {
	mu       sync.Mutex
	deadline time.Time
	history  []time.Time
	bytes.Buffer
}

func (w *deadlineWriter) Header() http.Header { return make(http.Header) }
func (w *deadlineWriter) WriteHeader(_ int)   {}
func (w *deadlineWriter) Flush()              {}
func (w *deadlineWriter) SetWriteDeadline(d time.Time) error {
	w.mu.Lock()
	w.deadline = d
	w.history = append(w.history, d)
	w.mu.Unlock()
	return nil
}

func TestStreamableSSEWriteDeadlineAndPostCancelGuard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &deadlineWriter{}
	stream := newStreamableSSE(ctx, w, "id", ServerInfo{}, nil, nil, 50*time.Millisecond, time.Second)
	before := time.Now()
	if err := stream.writeComment("keepalive"); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	deadline := w.deadline
	history := append([]time.Time(nil), w.history...)
	w.mu.Unlock()
	if !deadline.IsZero() {
		t.Fatalf("write deadline was not cleared: %v", deadline)
	}
	if len(history) < 2 || history[0].Before(before.Add(40*time.Millisecond)) || history[0].After(before.Add(time.Second)) {
		t.Fatalf("per-write deadline history = %v", history)
	}
	if w.Len() == 0 || time.Since(before) > time.Second {
		t.Fatal("deadline-protected write did not complete")
	}

	beforeLen := w.Len()
	cancel()
	if err := stream.writeComment("forbidden"); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-cancel write error = %v", err)
	}
	if w.Len() != beforeLen {
		t.Fatal("bytes were written after request cancellation")
	}
}

type slowDeadlineWriter struct {
	mu       sync.Mutex
	deadline time.Time
}

func (w *slowDeadlineWriter) Header() http.Header { return make(http.Header) }
func (w *slowDeadlineWriter) WriteHeader(_ int)   {}
func (w *slowDeadlineWriter) Flush()              {}
func (w *slowDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadline = deadline
	w.mu.Unlock()
	return nil
}
func (w *slowDeadlineWriter) Write(_ []byte) (int, error) {
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	if deadline.IsZero() {
		return 0, errors.New("write started without a deadline")
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	<-timer.C
	return 0, errors.New("simulated write deadline")
}

func TestStreamableSSESlowWriterHonorsDeadline(t *testing.T) {
	w := &slowDeadlineWriter{}
	stream := newStreamableSSE(context.Background(), w, "id", ServerInfo{}, nil, nil, 20*time.Millisecond, time.Second)
	started := time.Now()
	err := stream.writeComment("keepalive")
	elapsed := time.Since(started)
	if err == nil || elapsed < 15*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("slow write error/elapsed = %v/%v, want bounded deadline failure", err, elapsed)
	}
}

type interruptibleDeadlineWriter struct {
	started chan struct{}
	unblock chan struct{}
	once    sync.Once
}

func newInterruptibleDeadlineWriter() *interruptibleDeadlineWriter {
	return &interruptibleDeadlineWriter{started: make(chan struct{}), unblock: make(chan struct{})}
}

func (w *interruptibleDeadlineWriter) Header() http.Header { return make(http.Header) }
func (w *interruptibleDeadlineWriter) WriteHeader(_ int)   {}
func (w *interruptibleDeadlineWriter) Flush()              {}
func (w *interruptibleDeadlineWriter) Write(_ []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.unblock
	return 0, errors.New("interrupted write")
}
func (w *interruptibleDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		select {
		case <-w.unblock:
		default:
			close(w.unblock)
		}
	}
	return nil
}

func TestHandlerShutdownInterruptsWriteAtContextDeadline(t *testing.T) {
	h := newHandlerForTest(t)
	w := newInterruptibleDeadlineWriter()
	stream := newStreamableSSE(context.Background(), w, "id", ServerInfo{}, nil, nil, 30*time.Second, time.Second)
	if !h.registerStream(stream) {
		t.Fatal("register stream")
	}
	go func() {
		defer h.unregisterStream(stream)
		stream.serve(h)
	}()
	<-w.started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := h.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline", err)
	}
	select {
	case <-stream.done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("deadline did not interrupt blocked stream write")
	}
}
