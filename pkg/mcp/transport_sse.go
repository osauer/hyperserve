package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

// sseEvent is a queued write to the SSE wire. Routing every wire-side write
// through the main goroutine via eventChan is what makes writeSSEMessage
// single-writer safe — earlier shapes had the request-processing goroutine
// reaching the writer directly, racing with the main loop's pings and
// response delivery.
type sseEvent struct {
	event string
	data  []byte
}

// sseClient represents a connected SSE client.
type sseClient struct {
	id      string
	w       http.ResponseWriter
	flusher http.Flusher
	// bindingToken is a per-client capability that routed POSTs must present
	// (via X-SSE-Binding header) to be queued on this client's request
	// channel. Generated alongside the client ID; never transmitted in URLs
	// or logs. Empty for legacy clients (compatibility shim only — new
	// connections always get one).
	bindingToken  string
	messageChan   chan *jsonrpc.Response
	eventChan     chan sseEvent
	closeChan     chan struct{}
	closeOnce     sync.Once
	lastMessageID int
	logger        *slog.Logger
	initialized   bool
	ready         bool
	mu            sync.RWMutex
}

// sseManager owns the per-connection client state plus the per-client
// request channels used by the SSE-routed POST flow. Previously the channel
// map lived on Handler (`sseRequests`/`sseMutex`), forming a parallel state
// machine; consolidated here so HandleSSE and handleSSERoutedRequest agree
// on a single source of truth.
type sseManager struct {
	clients      map[string]*sseClient
	requestChans map[string]chan *jsonrpc.Request
	mu           sync.RWMutex
	logger       *slog.Logger
	pingInterval time.Duration
}

// newSSEManager creates a new SSE connection manager.
func newSSEManager() *sseManager {
	return &sseManager{
		clients:      make(map[string]*sseClient),
		requestChans: make(map[string]chan *jsonrpc.Request),
		logger:       logger,
		pingInterval: 30 * time.Second,
	}
}

// registerRequestChan creates the per-client request channel that routed
// POSTs are queued onto. Caller (HandleSSE) must hold no locks; this method
// takes the manager's write lock.
func (m *sseManager) registerRequestChan(clientID string) chan *jsonrpc.Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *jsonrpc.Request, 10)
	m.requestChans[clientID] = ch
	return ch
}

// unregisterRequestChan closes and removes the per-client request channel.
func (m *sseManager) unregisterRequestChan(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.requestChans[clientID]; ok {
		close(ch)
		delete(m.requestChans, clientID)
	}
}

// requestChanFor returns the channel for clientID, or nil if unknown.
func (m *sseManager) requestChanFor(clientID string) (chan *jsonrpc.Request, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.requestChans[clientID]
	return ch, ok
}

func newSSEClient(id, bindingToken string, w http.ResponseWriter, flusher http.Flusher) *sseClient {
	return &sseClient{
		id:           id,
		bindingToken: bindingToken,
		w:            w,
		flusher:      flusher,
		messageChan:  make(chan *jsonrpc.Response, 100),
		eventChan:    make(chan sseEvent, 16),
		closeChan:    make(chan struct{}),
		logger:       logger,
	}
}

// enqueueEvent submits a wire-side event to the per-client event channel,
// preserving the single-writer guarantee for writeSSEMessage. Callers must
// not panic on a full or closed channel; drop and warn instead.
func (c *sseClient) enqueueEvent(event string, data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("client closed: %v", r)
		}
	}()
	select {
	case c.eventChan <- sseEvent{event: event, data: data}:
		return nil
	case <-c.closeChan:
		return fmt.Errorf("client closed")
	default:
		c.logger.Warn("SSE client event channel full, dropping event", "client", c.id, "event", event)
		return fmt.Errorf("event channel full")
	}
}

// VerifyBinding constant-time-compares the supplied token to this client's
// binding token. Returns false for empty supplied token (no compatibility
// shortcut for missing header).
func (c *sseClient) VerifyBinding(supplied string) bool {
	if c.bindingToken == "" || supplied == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.bindingToken), []byte(supplied)) == 1
}

// Send sends a JSON-RPC response to the SSE client.
func (c *sseClient) Send(response *jsonrpc.Response) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("client closed: %v", r)
		}
	}()

	select {
	case c.messageChan <- response:
		return nil
	case <-c.closeChan:
		return fmt.Errorf("client closed")
	default:
		c.logger.Warn("SSE client message channel full, dropping message", "client", c.id)
		return fmt.Errorf("message channel full")
	}
}

// Close closes the SSE client connection.
func (c *sseClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		close(c.messageChan)
		close(c.eventChan)
	})
}

// SetInitialized marks the client as initialized.
func (c *sseClient) SetInitialized() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = true
}

// SetReady marks the client as ready.
func (c *sseClient) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

// IsReady reports whether the client is ready to receive messages.
func (c *sseClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

func (c *sseClient) writeSSEMessage(eventType string, data []byte) error {
	c.lastMessageID++
	if _, err := fmt.Fprintf(c.w, "id: %d\n", c.lastMessageID); err != nil {
		return err
	}
	if eventType != "" {
		if _, err := fmt.Fprintf(c.w, "event: %s\n", eventType); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.w, "data: %s\n\n", data); err != nil {
		return err
	}
	c.flusher.Flush()
	return nil
}

// HandleSSE handles SSE connections for MCP.
func (m *sseManager) HandleSSE(w http.ResponseWriter, r *http.Request, mcpHandler *Handler) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		m.logger.Debug("Unable to clear SSE write deadline", "error", err)
	}

	// Generate IDs FIRST, then bail out before any wire side effect or
	// registry insertion if crypto/rand failed. The previous shape wrote
	// the connection event and added to m.clients before checking, so a
	// degraded RNG would still leak an empty-token connection event to
	// the wire (even though routing was still fail-closed).
	clientID, bindingToken := generateClientIDAndBinding()
	if clientID == "" || bindingToken == "" {
		http.Error(w, "Unable to allocate SSE session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := newSSEClient(clientID, bindingToken, w, flusher)
	m.addClient(clientID, client)
	defer m.removeClient(clientID)

	requestChan := m.registerRequestChan(clientID)
	defer m.unregisterRequestChan(clientID)

	m.logger.Info("SSE client connected", "client", clientID)

	initialEvent := map[string]any{
		"type":     "connection",
		"clientId": clientID,
		// bindingToken is the capability that routed POSTs must echo back
		// via the X-SSE-Binding header. Knowing the clientId alone is not
		// enough to inject requests into another client's stream.
		"bindingToken": bindingToken,
		"message":      "Connected to MCP SSE endpoint",
	}
	// The initial connection event is the only writeSSEMessage call outside
	// the main loop. It runs before the request-processing goroutine starts,
	// so no other writer can race it.
	if data, err := json.Marshal(initialEvent); err == nil {
		_ = client.writeSSEMessage("connection", data)
	}

	transport := newSSETransport(clientID, m, requestChan)
	session := newMCPSession(r.Context(), mcpHandler, transport)
	defer session.close()
	engine := mcpHandler.newRPCEngine(session)

	ctx := r.Context()
	pingTicker := time.NewTicker(m.pingInterval)
	defer pingTicker.Stop()

	// Goroutine: process incoming MCP requests. Never writes to the SSE wire
	// directly — wire writes happen only in the main loop below.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.closeChan:
				return
			case request := <-requestChan:
				if request == nil {
					continue
				}
				if err := mcpHandler.processRequestObjectWithSession(request, transport, session, engine); err != nil {
					m.logger.Error("Failed to process SSE request", "error", err, "client", clientID)
				}
				if request.Method == "initialized" {
					client.SetInitialized()
					readyNotification := map[string]any{
						"jsonrpc": "2.0",
						"method":  "ready",
						"params":  map[string]any{},
					}
					if data, err := json.Marshal(readyNotification); err == nil {
						_ = client.enqueueEvent("notification", data)
						client.SetReady()
					}
				}
			}
		}
	}()

	// Main loop: deliver responses, queued events, and pings — the single
	// writer for client.writeSSEMessage.
	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("SSE client disconnected", "client", clientID)
			return
		case <-client.closeChan:
			return
		case response := <-client.messageChan:
			if response == nil {
				continue
			}
			data, err := json.Marshal(response)
			if err != nil {
				m.logger.Error("Failed to marshal response", "error", err, "client", clientID)
				continue
			}
			if err := client.writeSSEMessage("message", data); err != nil {
				m.logger.Error("Failed to write SSE message", "error", err, "client", clientID)
				return
			}
		case event := <-client.eventChan:
			if event.data == nil {
				continue
			}
			if err := client.writeSSEMessage(event.event, event.data); err != nil {
				m.logger.Error("Failed to write SSE event", "error", err, "client", clientID, "event", event.event)
				return
			}
		case <-pingTicker.C:
			pingData := map[string]any{
				"type":      "ping",
				"timestamp": time.Now().Unix(),
			}
			if data, err := json.Marshal(pingData); err == nil {
				if err := client.writeSSEMessage("ping", data); err != nil {
					m.logger.Debug("Failed to send ping", "error", err, "client", clientID)
					return
				}
			}
		}
	}
}

// SendToClient sends a response to a specific SSE client.
func (m *sseManager) SendToClient(clientID string, response *jsonrpc.Response) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	return client.Send(response)
}

// SendEventToClient sends an arbitrary SSE event to a specific client.
func (m *sseManager) SendEventToClient(clientID, event string, data []byte) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	return client.enqueueEvent(event, data)
}

// BroadcastToAll sends a response to all connected SSE clients.
func (m *sseManager) BroadcastToAll(response *jsonrpc.Response) {
	m.mu.RLock()
	clients := slices.Collect(maps.Values(m.clients))
	m.mu.RUnlock()
	for _, client := range clients {
		if err := client.Send(response); err != nil {
			m.logger.Debug("Failed to send to client", "client", client.id, "error", err)
		}
	}
}

// ClientCount returns the number of connected SSE clients.
func (m *sseManager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// lookup returns the client registered under id, or (nil, false) if absent.
func (m *sseManager) lookup(id string) (*sseClient, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[id]
	return c, ok
}

func (m *sseManager) addClient(id string, client *sseClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[id] = client
}

func (m *sseManager) removeClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, exists := m.clients[id]; exists {
		client.Close()
		delete(m.clients, id)
	}
}

// generateClientIDAndBinding returns a (clientID, bindingToken) pair sourced
// from crypto/rand. The clientID is the routing key; the bindingToken is the
// per-client capability that subsequent X-SSE-Binding headers must echo back
// to be queued onto this client's request channel. Using crypto/rand means
// state recovery from observed IDs is not feasible (math/rand was).
func generateClientIDAndBinding() (string, string) {
	idBytes := make([]byte, 16)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(idBytes); err != nil {
		// crypto/rand failure on a unix system means /dev/urandom is
		// unavailable — the server is in deep trouble. Refuse to fabricate
		// a predictable ID; the caller (HandleSSE) treats an empty token as
		// "binding unavailable" and rejects all subsequent routed POSTs.
		return "", ""
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", ""
	}
	return "sse-" + hex.EncodeToString(idBytes), hex.EncodeToString(tokenBytes)
}

// sseTransport implements Transport for SSE-based communication.
type sseTransport struct {
	clientID    string
	sseManager  *sseManager
	requestChan <-chan *jsonrpc.Request
	logger      *slog.Logger
}

func newSSETransport(clientID string, sseManager *sseManager, requestChan <-chan *jsonrpc.Request) *sseTransport {
	return &sseTransport{
		clientID:    clientID,
		sseManager:  sseManager,
		requestChan: requestChan,
		logger:      logger,
	}
}

func (t *sseTransport) Send(response *jsonrpc.Response) error {
	if response == nil {
		return nil
	}
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	return t.sseManager.SendEventToClient(t.clientID, "message", data)
}

func (t *sseTransport) SendNotification(method string, params any) error {
	notification := rpcNotification{
		JSONRPC: jsonrpc.Version,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}
	return t.sseManager.SendEventToClient(t.clientID, "notification", data)
}

func (t *sseTransport) Receive() (*jsonrpc.Request, error) {
	request, ok := <-t.requestChan
	if !ok {
		return nil, io.EOF
	}
	return request, nil
}

func (t *sseTransport) Close() error { return nil }
