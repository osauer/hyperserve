package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand"
	"net/http"
	"slices"
	"sync"
	"time"

	jsonrpc "github.com/osauer/hyperserve/pkg/jsonrpc"
)

// SSEClient represents a connected SSE client.
type SSEClient struct {
	id            string
	w             http.ResponseWriter
	flusher       http.Flusher
	messageChan   chan *jsonrpc.Response
	closeChan     chan struct{}
	closeOnce     sync.Once
	lastMessageID int
	logger        *slog.Logger
	initialized   bool
	ready         bool
	mu            sync.RWMutex
}

// SSEManager manages SSE connections for MCP.
type SSEManager struct {
	clients      map[string]*SSEClient
	mu           sync.RWMutex
	logger       *slog.Logger
	pingInterval time.Duration
}

// NewSSEManager creates a new SSE connection manager.
func NewSSEManager() *SSEManager {
	return &SSEManager{
		clients:      make(map[string]*SSEClient),
		logger:       logger,
		pingInterval: 30 * time.Second,
	}
}

func newSSEClient(id string, w http.ResponseWriter, flusher http.Flusher) *SSEClient {
	return &SSEClient{
		id:          id,
		w:           w,
		flusher:     flusher,
		messageChan: make(chan *jsonrpc.Response, 100),
		closeChan:   make(chan struct{}),
		logger:      logger,
	}
}

// Send sends a JSON-RPC response to the SSE client.
func (c *SSEClient) Send(response *jsonrpc.Response) (err error) {
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
func (c *SSEClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		close(c.messageChan)
	})
}

// SetInitialized marks the client as initialized.
func (c *SSEClient) SetInitialized() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = true
}

// SetReady marks the client as ready.
func (c *SSEClient) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

// IsReady reports whether the client is ready to receive messages.
func (c *SSEClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

func (c *SSEClient) writeSSEMessage(eventType string, data []byte) error {
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
func (m *SSEManager) HandleSSE(w http.ResponseWriter, r *http.Request, mcpHandler *Handler) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientID := generateClientID()
	client := newSSEClient(clientID, w, flusher)
	m.addClient(clientID, client)
	defer m.removeClient(clientID)

	requestChan := mcpHandler.RegisterSSEClient(clientID)
	defer mcpHandler.UnregisterSSEClient(clientID)

	m.logger.Info("SSE client connected", "client", clientID)

	initialEvent := map[string]any{
		"type":     "connection",
		"clientId": clientID,
		"message":  "Connected to MCP SSE endpoint",
	}
	if data, err := json.Marshal(initialEvent); err == nil {
		_ = client.writeSSEMessage("connection", data)
	}

	transport := newSSETransport(clientID, m, requestChan)

	ctx := r.Context()
	pingTicker := time.NewTicker(m.pingInterval)
	defer pingTicker.Stop()

	// Goroutine: process incoming MCP requests.
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
				response := mcpHandler.rpcEngine.ProcessRequestDirect(request)
				if err := transport.Send(response); err != nil {
					m.logger.Error("Failed to send response", "error", err, "client", clientID)
				}
				if request.Method == "initialized" {
					client.SetInitialized()
					readyNotification := map[string]any{
						"jsonrpc": "2.0",
						"method":  "ready",
						"params":  map[string]any{},
					}
					if data, err := json.Marshal(readyNotification); err == nil {
						_ = client.writeSSEMessage("notification", data)
						client.SetReady()
					}
				}
			}
		}
	}()

	// Main loop: deliver responses and pings.
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
func (m *SSEManager) SendToClient(clientID string, response *jsonrpc.Response) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	return client.Send(response)
}

// BroadcastToAll sends a response to all connected SSE clients.
func (m *SSEManager) BroadcastToAll(response *jsonrpc.Response) {
	m.mu.RLock()
	clients := slices.Collect(maps.Values(m.clients))
	m.mu.RUnlock()
	for _, client := range clients {
		if err := client.Send(response); err != nil {
			m.logger.Debug("Failed to send to client", "client", client.id, "error", err)
		}
	}
}

// GetClientCount returns the number of connected SSE clients.
func (m *SSEManager) GetClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

func (m *SSEManager) addClient(id string, client *SSEClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[id] = client
}

func (m *SSEManager) removeClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, exists := m.clients[id]; exists {
		client.Close()
		delete(m.clients, id)
	}
}

func generateClientID() string {
	return fmt.Sprintf("sse-%d-%d", time.Now().UnixNano(), rand.Int())
}

// sseTransport implements Transport for SSE-based communication.
type sseTransport struct {
	clientID    string
	sseManager  *SSEManager
	requestChan <-chan *jsonrpc.Request
	logger      *slog.Logger
}

func newSSETransport(clientID string, sseManager *SSEManager, requestChan <-chan *jsonrpc.Request) *sseTransport {
	return &sseTransport{
		clientID:    clientID,
		sseManager:  sseManager,
		requestChan: requestChan,
		logger:      logger,
	}
}

func (t *sseTransport) Send(response *jsonrpc.Response) error {
	return t.sseManager.SendToClient(t.clientID, response)
}

func (t *sseTransport) Receive() (*jsonrpc.Request, error) {
	request, ok := <-t.requestChan
	if !ok {
		return nil, io.EOF
	}
	return request, nil
}

func (t *sseTransport) Close() error { return nil }
