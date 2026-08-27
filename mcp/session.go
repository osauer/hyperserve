package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
)

type notificationTransport interface {
	SendNotification(method string, params any) error
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type resourceSubscription struct {
	uri      string
	template SubscribableResourceTemplate
	params   map[string]string
	ctx      context.Context
	cancel   context.CancelFunc
}

type mcpSession struct {
	handler *Handler
	sender  notificationTransport
	ctx     context.Context
	cancel  context.CancelFunc

	mu            sync.Mutex
	subscriptions map[string]*resourceSubscription
	pending       []*resourceSubscription
	closed        bool
}

func newMCPSession(parent context.Context, handler *Handler, sender notificationTransport) *mcpSession {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &mcpSession{
		handler:       handler,
		sender:        sender,
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string]*resourceSubscription),
	}
}

func (s *mcpSession) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancel()
	for _, sub := range s.subscriptions {
		sub.cancel()
	}
	clear(s.subscriptions)
	s.pending = nil
	s.mu.Unlock()
}

func (s *mcpSession) subscribe(uri string, template SubscribableResourceTemplate, params map[string]string) error {
	if s == nil || s.sender == nil {
		return fmt.Errorf("resources/subscribe requires a live MCP session (SSE or stdio)")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("MCP session is closed")
	}
	if _, exists := s.subscriptions[uri]; exists {
		return nil
	}
	ctx, cancel := context.WithCancel(s.ctx)
	sub := &resourceSubscription{
		uri:      uri,
		template: template,
		params:   copyStringMap(params),
		ctx:      ctx,
		cancel:   cancel,
	}
	s.subscriptions[uri] = sub
	s.pending = append(s.pending, sub)
	return nil
}

func (s *mcpSession) unsubscribe(uri string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, exists := s.subscriptions[uri]
	if !exists {
		return
	}
	sub.cancel()
	delete(s.subscriptions, uri)
	for i := 0; i < len(s.pending); i++ {
		if s.pending[i].uri == uri {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			i--
		}
	}
}

func (s *mcpSession) startPending() {
	if s == nil {
		return
	}
	s.mu.Lock()
	pending := append([]*resourceSubscription(nil), s.pending...)
	s.pending = nil
	s.mu.Unlock()

	for _, sub := range pending {
		select {
		case <-sub.ctx.Done():
			continue
		default:
		}
		go s.runSubscription(sub)
	}
}

func (s *mcpSession) runSubscription(sub *resourceSubscription) {
	err := sub.template.Subscribe(sub.ctx, sub.uri, copyStringMap(sub.params), s)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(sub.ctx.Err(), context.Canceled) {
		s.handler.logger.Warn("MCP resource subscription ended with error",
			"uri", sub.uri,
			"error", err)
	}

	s.mu.Lock()
	if current, ok := s.subscriptions[sub.uri]; ok && current == sub {
		delete(s.subscriptions, sub.uri)
	}
	s.mu.Unlock()
}

// Update implements ResourceEmitter.
func (s *mcpSession) Update(uri string) error {
	if s == nil || s.sender == nil {
		return fmt.Errorf("MCP session cannot send resource updates")
	}
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}
	return s.sender.SendNotification("notifications/resources/updated", map[string]any{
		"uri": uri,
	})
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
