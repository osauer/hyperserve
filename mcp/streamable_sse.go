package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	jsonrpc "github.com/osauer/hyperserve/v2/jsonrpc"
)

const (
	streamableSSEQueueSize        = 32
	streamableSSEMaxSubscriptions = 128
	streamableSSEWriteTimeout     = 30 * time.Second
	streamableSSEKeepalive        = 30 * time.Second
	subscriptionIDMetaKey         = "io.modelcontextprotocol/subscriptionId"
)

type notificationSubscriptions struct {
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
}

type subscriptionsListenParams struct {
	Notifications *notificationSubscriptions `json:"notifications"`
}

type resolvedSubscription struct {
	uri      string
	template SubscribableResourceTemplate
	params   map[string]string
}

type streamableSSE struct {
	w             http.ResponseWriter
	requestID     any
	serverInfo    ServerInfo
	subscriptions []resolvedSubscription
	acknowledged  []string

	ctx    context.Context
	cancel context.CancelFunc
	// requestCtx distinguishes peer disconnect from graceful server shutdown:
	// producers stop in both cases, but only shutdown may still write complete.
	requestCtx context.Context

	events       chan []byte
	producerDone chan struct{}
	shutdown     chan struct{}
	done         chan struct{}
	shutdownOnce sync.Once
	doneOnce     sync.Once
	closing      atomic.Bool

	writeTimeout time.Duration
	keepalive    time.Duration
}

func newStreamableSSE(
	parent context.Context,
	w http.ResponseWriter,
	requestID any,
	serverInfo ServerInfo,
	subscriptions []resolvedSubscription,
	acknowledged []string,
	writeTimeout time.Duration,
	keepalive time.Duration,
) *streamableSSE {
	ctx, cancel := context.WithCancel(parent)
	return &streamableSSE{
		w:             w,
		requestID:     requestID,
		serverInfo:    serverInfo,
		subscriptions: subscriptions,
		acknowledged:  acknowledged,
		ctx:           ctx,
		cancel:        cancel,
		requestCtx:    parent,
		events:        make(chan []byte, streamableSSEQueueSize),
		producerDone:  make(chan struct{}, len(subscriptions)),
		shutdown:      make(chan struct{}),
		done:          make(chan struct{}),
		writeTimeout:  writeTimeout,
		keepalive:     keepalive,
	}
}

func (s *streamableSSE) requestShutdown() {
	s.shutdownOnce.Do(func() {
		s.closing.Store(true)
		close(s.shutdown)
	})
}

func (s *streamableSSE) finish() {
	s.closing.Store(true)
	s.cancel()
	s.doneOnce.Do(func() { close(s.done) })
}

func (s *streamableSSE) abort() {
	s.closing.Store(true)
	s.cancel()
	_ = http.NewResponseController(s.w).SetWriteDeadline(time.Now())
}

func (s *streamableSSE) serve(h *Handler) {
	defer s.finish()

	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-cache, no-transform")
	s.w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(s.w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.logger.Debug("Unable to clear Streamable HTTP write deadline", "error", err)
	}

	ack := rpcNotification{
		JSONRPC: jsonrpc.Version,
		Method:  "notifications/subscriptions/acknowledged",
		Params: map[string]any{
			"_meta": map[string]any{subscriptionIDMetaKey: s.requestID},
			"notifications": map[string]any{
				"resourceSubscriptions": s.acknowledged,
			},
		},
	}
	if err := s.writeJSON(ack); err != nil {
		return
	}
	if len(s.subscriptions) == 0 {
		s.closing.Store(true)
		_ = s.writeJSON(s.completionResponse())
		return
	}

	for _, subscription := range s.subscriptions {
		go s.runSubscription(h, subscription)
	}

	keepalive := time.NewTicker(s.keepalive)
	defer keepalive.Stop()
	completed := 0
	for {
		select {
		case data := <-s.events:
			if err := s.writeEvent(data); err != nil {
				return
			}
		case <-s.producerDone:
			completed++
			if len(s.subscriptions) > 0 && completed == len(s.subscriptions) {
				s.closing.Store(true)
				s.cancel()
				if err := s.writeQueuedEvents(); err != nil {
					return
				}
				_ = s.writeJSON(s.completionResponse())
				return
			}
		case <-keepalive.C:
			if err := s.writeComment("keepalive"); err != nil {
				return
			}
		case <-s.shutdown:
			s.cancel()
			if err := s.writeQueuedEvents(); err != nil {
				return
			}
			_ = s.writeJSON(s.completionResponse())
			return
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *streamableSSE) writeQueuedEvents() error {
	for {
		select {
		case data := <-s.events:
			if err := s.writeEvent(data); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (h *Handler) registerStream(stream *streamableSSE) bool {
	h.streamMu.Lock()
	defer h.streamMu.Unlock()
	if h.shuttingDown {
		return false
	}
	h.streams[stream] = struct{}{}
	return true
}

func (h *Handler) unregisterStream(stream *streamableSSE) {
	h.streamMu.Lock()
	delete(h.streams, stream)
	h.streamMu.Unlock()
}

func (h *Handler) isShuttingDown() bool {
	h.streamMu.Lock()
	defer h.streamMu.Unlock()
	return h.shuttingDown
}

// Shutdown gracefully completes active subscriptions/listen streams and
// closes legacy routed SSE connections. It is safe to call more than once.
func (h *Handler) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	h.streamMu.Lock()
	h.shuttingDown = true
	streams := make([]*streamableSSE, 0, len(h.streams))
	for stream := range h.streams {
		streams = append(streams, stream)
	}
	h.streamMu.Unlock()

	h.sseManager.CloseAll()
	for _, stream := range streams {
		stream.requestShutdown()
	}
	for _, stream := range streams {
		select {
		case <-stream.done:
		case <-ctx.Done():
			for _, active := range streams {
				active.abort()
			}
			return ctx.Err()
		}
	}
	return nil
}

func (h *Handler) serveSubscriptionsListen(w http.ResponseWriter, r *http.Request, request *jsonrpc.Request) {
	if request.IsNotification() || !validJSONRPCID(request.ID) {
		h.writeStreamableError(w, http.StatusBadRequest, nil, jsonrpc.ErrorCodeInvalidRequest,
			"subscriptions/listen requires a string or numeric request id", nil)
		return
	}

	data, err := json.Marshal(request.Params)
	if err != nil {
		h.writeStreamableError(w, http.StatusBadRequest, request.ID, jsonrpc.ErrorCodeInvalidParams,
			"Invalid subscriptions/listen params", nil)
		return
	}
	var params subscriptionsListenParams
	if err := json.Unmarshal(data, &params); err != nil || params.Notifications == nil {
		h.writeStreamableError(w, http.StatusBadRequest, request.ID, jsonrpc.ErrorCodeInvalidParams,
			"subscriptions/listen requires a notifications object", nil)
		return
	}
	if len(params.Notifications.ResourceSubscriptions) > streamableSSEMaxSubscriptions {
		h.writeStreamableError(w, http.StatusBadRequest, request.ID, jsonrpc.ErrorCodeInvalidParams,
			fmt.Sprintf("resourceSubscriptions exceeds the %d URI limit", streamableSSEMaxSubscriptions), nil)
		return
	}

	seen := make(map[string]struct{}, len(params.Notifications.ResourceSubscriptions))
	resolved := make([]resolvedSubscription, 0, len(params.Notifications.ResourceSubscriptions))
	acknowledged := make([]string, 0, len(params.Notifications.ResourceSubscriptions))
	for _, uri := range params.Notifications.ResourceSubscriptions {
		if _, duplicate := seen[uri]; duplicate {
			continue
		}
		seen[uri] = struct{}{}
		template, templateParams, ok := h.matchSubscribableResourceTemplate(uri)
		if !ok {
			continue
		}
		resolved = append(resolved, resolvedSubscription{uri: uri, template: template, params: templateParams})
		acknowledged = append(acknowledged, uri)
	}

	stream := newStreamableSSE(
		r.Context(),
		w,
		request.ID,
		h.serverInfo,
		resolved,
		acknowledged,
		h.streamWriteTimeout,
		h.streamKeepalive,
	)
	if !h.registerStream(stream) {
		h.writeStreamableError(w, http.StatusServiceUnavailable, request.ID, jsonrpc.ErrorCodeInternalError,
			"Server is shutting down", nil)
		return
	}
	defer h.unregisterStream(stream)
	stream.serve(h)
}

func (s *streamableSSE) runSubscription(h *Handler, subscription resolvedSubscription) {
	emitter := &streamableResourceEmitter{stream: s}
	err := subscription.template.Subscribe(
		s.ctx,
		subscription.uri,
		copyStringMap(subscription.params),
		emitter,
	)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(s.ctx.Err(), context.Canceled) {
		h.logger.Warn("MCP Streamable HTTP subscription ended with error",
			"uri", subscription.uri,
			"error", err)
	}
	select {
	case s.producerDone <- struct{}{}:
	case <-s.ctx.Done():
	}
}

func (s *streamableSSE) completionResponse() *jsonrpc.Response {
	return &jsonrpc.Response{
		JSONRPC: jsonrpc.Version,
		ID:      s.requestID,
		Result: map[string]any{
			"resultType": "complete",
			"_meta": map[string]any{
				subscriptionIDMetaKey:                s.requestID,
				"io.modelcontextprotocol/serverInfo": s.serverInfo,
			},
		},
	}
}

type streamableResourceEmitter struct {
	stream *streamableSSE
}

func (e *streamableResourceEmitter) Update(uri string) error {
	if e == nil || e.stream == nil {
		return fmt.Errorf("MCP subscription stream is unavailable")
	}
	notification := rpcNotification{
		JSONRPC: jsonrpc.Version,
		Method:  "notifications/resources/updated",
		Params: map[string]any{
			"_meta": map[string]any{subscriptionIDMetaKey: e.stream.requestID},
			"uri":   uri,
		},
	}
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal MCP resource notification: %w", err)
	}
	if e.stream.closing.Load() {
		return context.Canceled
	}
	select {
	case <-e.stream.ctx.Done():
		return e.stream.ctx.Err()
	default:
	}
	select {
	case e.stream.events <- data:
		if e.stream.closing.Load() {
			return context.Canceled
		}
		select {
		case <-e.stream.ctx.Done():
			return e.stream.ctx.Err()
		default:
		}
		return nil
	case <-e.stream.ctx.Done():
		return e.stream.ctx.Err()
	}
}

func (s *streamableSSE) writeJSON(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal Streamable HTTP message: %w", err)
	}
	return s.writeEvent(data)
}

func (s *streamableSSE) writeEvent(data []byte) error {
	var event bytes.Buffer
	event.Grow(len(data) + len("event: message\ndata: \n\n"))
	event.WriteString("event: message\n")
	event.WriteString("data: ")
	event.Write(data)
	event.WriteString("\n\n")
	return s.writeBytes(event.Bytes())
}

func (s *streamableSSE) writeComment(comment string) error {
	return s.writeBytes([]byte(": " + comment + "\n\n"))
}

func (s *streamableSSE) writeBytes(data []byte) error {
	select {
	case <-s.requestCtx.Done():
		return s.requestCtx.Err()
	default:
	}
	controller := http.NewResponseController(s.w)
	if err := controller.SetWriteDeadline(time.Now().Add(s.writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("set Streamable HTTP write deadline: %w", err)
	}
	defer func() {
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	if _, err := s.w.Write(data); err != nil {
		return fmt.Errorf("write Streamable HTTP event: %w", err)
	}
	if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return fmt.Errorf("flush Streamable HTTP event: %w", err)
	}
	return nil
}
