package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/osauer/hyperserve/v2/jsonrpc"
)

type notificationCaptureTransport struct {
	notifications chan rpcNotification
}

func newNotificationCaptureTransport() *notificationCaptureTransport {
	return &notificationCaptureTransport{notifications: make(chan rpcNotification, 8)}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (t *notificationCaptureTransport) SendNotification(method string, params any) error {
	t.notifications <- rpcNotification{
		JSONRPC: jsonrpc.Version,
		Method:  method,
		Params:  params,
	}
	return nil
}

type liveQuoteTemplate struct {
	quoteTemplate
	started chan struct{}
	done    chan struct{}
	updates chan string
}

func newLiveQuoteTemplate() *liveQuoteTemplate {
	return &liveQuoteTemplate{
		started: make(chan struct{}, 1),
		done:    make(chan struct{}),
		updates: make(chan string, 8),
	}
}

func (t *liveQuoteTemplate) Subscribe(ctx context.Context, uri string, _ map[string]string, emit ResourceEmitter) error {
	t.started <- struct{}{}
	defer close(t.done)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-t.updates:
			if update == "" {
				update = uri
			}
			if err := emit.Update(update); err != nil {
				return err
			}
		}
	}
}

func TestResourceSubscribeCapabilityDependsOnRegisteredTemplates(t *testing.T) {
	h := newHandlerForTest(t)
	if h.Capabilities().Resources.Subscribe {
		t.Fatal("Subscribe capability = true with no subscribable templates")
	}
	h.RegisterResourceTemplate(&quoteTemplate{})
	if h.Capabilities().Resources.Subscribe {
		t.Fatal("Subscribe capability = true with only non-subscribable template")
	}
	h.RegisterResourceTemplate(newLiveQuoteTemplate())
	if !h.Capabilities().Resources.Subscribe {
		t.Fatal("Subscribe capability = false with subscribable template")
	}
}

func TestResourceSubscribeAckBeforeNotificationAndUnsubscribeCancels(t *testing.T) {
	h := newHandlerForTest(t)
	template := newLiveQuoteTemplate()
	h.RegisterResourceTemplate(template)

	notifier := newNotificationCaptureTransport()
	session := newMCPSession(context.Background(), h, notifier)
	defer session.close()
	engine := h.newRPCEngine(session)
	transport := newResponseCaptureTransport()

	err := h.processRequestObjectWithSession(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/subscribe",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      1,
	}, transport, session, engine)
	if err != nil {
		t.Fatalf("subscribe request: %v", err)
	}

	select {
	case response := <-transport.responses:
		if response.Error != nil {
			t.Fatalf("subscribe response error: %+v", response.Error)
		}
		if response.ID != 1 {
			t.Fatalf("subscribe response ID = %v, want 1", response.ID)
		}
	default:
		t.Fatal("subscribe ack was not sent synchronously")
	}

	select {
	case <-template.started:
	case <-time.After(time.Second):
		t.Fatal("subscription did not start")
	}
	template.updates <- "quotes://AAPL"

	select {
	case notification := <-notifier.notifications:
		if notification.Method != "notifications/resources/updated" {
			t.Fatalf("notification method = %q", notification.Method)
		}
		params := notification.Params.(map[string]any)
		if params["uri"] != "quotes://AAPL" {
			t.Fatalf("notification params = %v, want uri only", params)
		}
		if len(params) != 1 {
			t.Fatalf("notification carried extra params: %v", params)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resource update notification")
	}

	err = h.processRequestObjectWithSession(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/unsubscribe",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      2,
	}, transport, session, engine)
	if err != nil {
		t.Fatalf("unsubscribe request: %v", err)
	}
	<-transport.responses

	select {
	case <-template.done:
	case <-time.After(time.Second):
		t.Fatal("subscription was not canceled by unsubscribe")
	}

	err = h.processRequestObjectWithSession(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/unsubscribe",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      3,
	}, transport, session, engine)
	if err != nil {
		t.Fatalf("idempotent unsubscribe request: %v", err)
	}
	response := <-transport.responses
	if response.Error != nil {
		t.Fatalf("idempotent unsubscribe response error: %+v", response.Error)
	}
}

func TestResourceSubscribePlainHTTPRequiresLiveSession(t *testing.T) {
	h := newHandlerForTest(t)
	h.RegisterResourceTemplate(newLiveQuoteTemplate())

	resp := h.RPCEngine().ProcessRequestDirect(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/subscribe",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      1,
	})
	if resp.Error == nil {
		t.Fatal("resources/subscribe without session succeeded, want JSON-RPC error")
	}
	if !strings.Contains(resp.Error.Data.(string), "live MCP session") {
		t.Fatalf("error data = %v, want live session guidance", resp.Error.Data)
	}
}

func TestResourceSubscribeStdioNotification(t *testing.T) {
	h := newHandlerForTest(t)
	template := newLiveQuoteTemplate()
	h.RegisterResourceTemplate(template)

	var out lockedBuffer
	transport := NewStdioTransportWithIO(strings.NewReader(""), &out, h.logger)
	session := newMCPSession(context.Background(), h, transport)
	defer session.close()
	engine := h.newRPCEngine(session)

	if err := h.processRequestObjectWithSession(&jsonrpc.Request{
		JSONRPC: jsonrpc.Version,
		Method:  "resources/subscribe",
		Params:  map[string]any{"uri": "quotes://AAPL"},
		ID:      1,
	}, transport, session, engine); err != nil {
		t.Fatalf("subscribe request: %v", err)
	}
	select {
	case <-template.started:
	case <-time.After(time.Second):
		t.Fatal("subscription did not start")
	}
	template.updates <- "quotes://AAPL"

	deadline := time.After(time.Second)
	for {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(lines) >= 2 {
			var response map[string]any
			if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
				t.Fatalf("response line is not JSON: %v", err)
			}
			if response["id"] != float64(1) {
				t.Fatalf("first line = %v, want subscribe response", response)
			}
			var notification map[string]any
			if err := json.Unmarshal([]byte(lines[1]), &notification); err != nil {
				t.Fatalf("notification line is not JSON: %v", err)
			}
			if notification["method"] != "notifications/resources/updated" {
				t.Fatalf("notification = %v", notification)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for stdio notification; output so far: %q", out.String())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestResourceSubscribeSSEDelivery(t *testing.T) {
	h := newHandlerForTest(t)
	h.SetLegacyRoutedSSEEnabled(true)
	template := newLiveQuoteTemplate()
	h.RegisterResourceTemplate(template)
	server := httptest.NewServer(h)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	events := make(chan string, 8)
	go func() {
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				events <- data
			}
		}
	}()

	var conn map[string]any
	select {
	case event := <-events:
		if err := json.Unmarshal([]byte(event), &conn); err != nil {
			t.Fatalf("connection event JSON: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for connection event")
	}

	body := strings.NewReader(`{"jsonrpc":"2.0","method":"resources/subscribe","params":{"uri":"quotes://AAPL"},"id":1}`)
	post, err := http.NewRequest(http.MethodPost, server.URL, body)
	if err != nil {
		t.Fatalf("new routed POST: %v", err)
	}
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("X-SSE-Client-ID", conn["clientId"].(string))
	post.Header.Set("X-SSE-Binding", conn["bindingToken"].(string))
	postResp, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Fatalf("routed POST: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("routed POST status = %d, want 202", postResp.StatusCode)
	}

	select {
	case <-template.started:
	case <-time.After(time.Second):
		t.Fatal("subscription did not start")
	}
	template.updates <- "quotes://AAPL"

	select {
	case event := <-events:
		var response map[string]any
		if err := json.Unmarshal([]byte(event), &response); err != nil {
			t.Fatalf("subscribe response JSON: %v", err)
		}
		if response["id"] != float64(1) {
			t.Fatalf("first SSE event after subscribe = %v, want ack", response)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe ack")
	}

	select {
	case event := <-events:
		var notification map[string]any
		if err := json.Unmarshal([]byte(event), &notification); err != nil {
			t.Fatalf("notification JSON: %v", err)
		}
		if notification["method"] != "notifications/resources/updated" {
			t.Fatalf("notification = %v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE resource update notification")
	}
}

func TestSSESurvivesHTTPWriteTimeout(t *testing.T) {
	h := newHandlerForTest(t)
	h.SetLegacyRoutedSSEEnabled(true)
	server := httptest.NewUnstartedServer(h)
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Start()
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect SSE: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	events := make(chan string, 8)
	go func() {
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				events <- data
			}
		}
	}()

	var conn map[string]any
	select {
	case event := <-events:
		if err := json.Unmarshal([]byte(event), &conn); err != nil {
			t.Fatalf("connection event JSON: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for connection event")
	}

	time.Sleep(400 * time.Millisecond)

	body := strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":77}`)
	post, err := http.NewRequest(http.MethodPost, server.URL, body)
	if err != nil {
		t.Fatalf("new routed POST: %v", err)
	}
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("X-SSE-Client-ID", conn["clientId"].(string))
	post.Header.Set("X-SSE-Binding", conn["bindingToken"].(string))
	postResp, err := http.DefaultClient.Do(post)
	if err != nil {
		t.Fatalf("routed POST after write timeout: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("routed POST status = %d, want 202", postResp.StatusCode)
	}

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-events:
			var response map[string]any
			if err := json.Unmarshal([]byte(event), &response); err != nil {
				t.Fatalf("SSE response JSON: %v", err)
			}
			if response["id"] == float64(77) {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for SSE response after HTTP write timeout")
		}
	}
}
