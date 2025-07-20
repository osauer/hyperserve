package hyperserve

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSEClient represents a connected SSE client
type SSEClient struct {
	id            string
	w             http.ResponseWriter
	flusher       http.Flusher
	messageChan   chan *JSONRPCResponse
	closeChan     chan struct{}
	closeOnce     sync.Once
	lastMessageID int
	logger        *slog.Logger
	initialized   bool          // Track if client has completed initialization
	ready         bool          // Track if client is ready to receive messages
	mu            sync.RWMutex  // Protect state fields
}

// SSEManager manages SSE connections for MCP
type SSEManager struct {
	clients      map[string]*SSEClient
	mu           sync.RWMutex
	logger       *slog.Logger
	pingInterval time.Duration
}

// NewSSEManager creates a new SSE connection manager
func NewSSEManager() *SSEManager {
	return &SSEManager{
		clients:      make(map[string]*SSEClient),
		logger:       logger,
		pingInterval: 30 * time.Second, // Send keepalive every 30 seconds
	}
}

// newSSEClient creates a new SSE client
func newSSEClient(id string, w http.ResponseWriter, flusher http.Flusher) *SSEClient {
	return &SSEClient{
		id:          id,
		w:           w,
		flusher:     flusher,
		messageChan: make(chan *JSONRPCResponse, 100), // Buffer for messages
		closeChan:   make(chan struct{}),
		logger:      logger,
	}
}

// Send sends a JSON-RPC response to the SSE client
func (c *SSEClient) Send(response *JSONRPCResponse) (err error) {
	// Recover from panic if channel is closed
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
		// Channel full, drop message
		c.logger.Warn("SSE client message channel full, dropping message", "client", c.id)
		return fmt.Errorf("message channel full")
	}
}

// Close closes the SSE client connection
func (c *SSEClient) Close() {
	c.closeOnce.Do(func() {
		close(c.closeChan)
		close(c.messageChan)
	})
}

// SetInitialized marks the client as initialized
func (c *SSEClient) SetInitialized() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = true
}

// SetReady marks the client as ready
func (c *SSEClient) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

// IsReady returns whether the client is ready to receive messages
func (c *SSEClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// writeSSEMessage writes an SSE message to the client
func (c *SSEClient) writeSSEMessage(eventType string, data []byte) error {
	c.lastMessageID++
	
	// Write SSE format
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
	
	// Flush the data immediately
	c.flusher.Flush()
	return nil
}

// HandleSSE handles SSE connections for MCP
func (m *SSEManager) HandleSSE(w http.ResponseWriter, r *http.Request, mcpHandler *MCPHandler) {
	// Handle POST requests for JSON-RPC over SSE
	if r.Method == http.MethodPost {
		m.handleJSONRPCOverSSE(w, r, mcpHandler)
		return
	}
	
	// Handle GET requests for SSE connection establishment
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET for SSE connection or POST for JSON-RPC requests.", http.StatusMethodNotAllowed)
		return
	}
	
	// Check if SSE is supported
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering
	
	// Generate client ID
	clientID := generateClientID()
	client := newSSEClient(clientID, w, flusher)
	
	// Register client with SSE manager
	m.addClient(clientID, client)
	defer m.removeClient(clientID)
	
	// Register client with MCP handler for request routing
	requestChan := mcpHandler.RegisterSSEClient(clientID)
	defer mcpHandler.UnregisterSSEClient(clientID)
	
	m.logger.Info("SSE client connected", "client", clientID)
	
	// Send initial connection event
	initialEvent := map[string]interface{}{
		"type":     "connection",
		"clientId": clientID,
		"message":  "Connected to MCP SSE endpoint",
	}
	if data, err := json.Marshal(initialEvent); err == nil {
		client.writeSSEMessage("connection", data)
	}
	
	// Create SSE transport for processing requests
	transport := newSSETransport(clientID, m, requestChan)
	
	// Use request context for this connection
	ctx := r.Context()
	
	// Start ping timer
	pingTicker := time.NewTicker(m.pingInterval)
	defer pingTicker.Stop()
	
	// Start a goroutine to process MCP requests
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.closeChan:
				return
			case request := <-requestChan:
				if request != nil {
					// Process the request directly using the RPC engine
					response := mcpHandler.rpcEngine.ProcessRequestDirect(request)
					
					// Send response back via SSE
					if err := transport.Send(response); err != nil {
						m.logger.Error("Failed to send response", "error", err, "client", clientID)
					}
					
					// Check if this was an initialized notification
					if request.Method == "initialized" {
						client.SetInitialized()
						// Send ready notification
						readyNotification := map[string]interface{}{
							"jsonrpc": "2.0",
							"method":  "ready",
							"params":  map[string]interface{}{},
						}
						if data, err := json.Marshal(readyNotification); err == nil {
							client.writeSSEMessage("notification", data)
							client.SetReady()
						}
					}
				}
			}
		}
	}()
	
	// Main event loop for sending responses
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			m.logger.Debug("SSE client disconnected", "client", clientID)
			return
			
		case <-client.closeChan:
			// Client closed
			return
			
		case response := <-client.messageChan:
			// Send JSON-RPC response
			if response != nil {
				data, err := json.Marshal(response)
				if err != nil {
					m.logger.Error("Failed to marshal response", "error", err, "client", clientID)
					continue
				}
				
				if err := client.writeSSEMessage("message", data); err != nil {
					m.logger.Error("Failed to write SSE message", "error", err, "client", clientID)
					return
				}
			}
			
		case <-pingTicker.C:
			// Send keepalive ping
			pingData := map[string]interface{}{
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

// handleJSONRPCOverSSE handles JSON-RPC requests sent directly to the SSE endpoint
func (m *SSEManager) handleJSONRPCOverSSE(w http.ResponseWriter, r *http.Request, mcpHandler *MCPHandler) {
	// Validate Content-Type
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}
	
	// Parse JSON-RPC request
	var request JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON-RPC request: "+err.Error(), http.StatusBadRequest)
		return
	}
	
	m.logger.Debug("Received JSON-RPC request on SSE endpoint", "method", request.Method, "id", request.ID)
	
	// Process the request directly using the MCP handler's RPC engine
	response := mcpHandler.rpcEngine.ProcessRequestDirect(&request)
	
	// Send JSON response back to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		m.logger.Error("Failed to send JSON-RPC response", "error", err)
		http.Error(w, "Failed to send response", http.StatusInternalServerError)
		return
	}
	
	m.logger.Debug("JSON-RPC response sent via SSE endpoint", "method", request.Method, "id", request.ID)
}

// SendToClient sends a response to a specific SSE client
func (m *SSEManager) SendToClient(clientID string, response *JSONRPCResponse) error {
	m.mu.RLock()
	client, exists := m.clients[clientID]
	m.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("client not found: %s", clientID)
	}
	
	return client.Send(response)
}

// BroadcastToAll sends a response to all connected SSE clients
func (m *SSEManager) BroadcastToAll(response *JSONRPCResponse) {
	m.mu.RLock()
	clients := make([]*SSEClient, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.mu.RUnlock()
	
	for _, client := range clients {
		if err := client.Send(response); err != nil {
			m.logger.Debug("Failed to send to client", "client", client.id, "error", err)
		}
	}
}

// GetClientCount returns the number of connected SSE clients
func (m *SSEManager) GetClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// addClient registers a new SSE client
func (m *SSEManager) addClient(id string, client *SSEClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[id] = client
}

// removeClient unregisters an SSE client
func (m *SSEManager) removeClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if client, exists := m.clients[id]; exists {
		client.Close()
		delete(m.clients, id)
	}
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("sse-%d-%d", time.Now().UnixNano(), rand.Int())
}

// sseTransport implements MCPTransport for SSE-based communication
type sseTransport struct {
	clientID    string
	sseManager  *SSEManager
	requestChan <-chan *JSONRPCRequest
	logger      *slog.Logger
}

// newSSETransport creates a new SSE transport
func newSSETransport(clientID string, sseManager *SSEManager, requestChan <-chan *JSONRPCRequest) *sseTransport {
	return &sseTransport{
		clientID:    clientID,
		sseManager:  sseManager,
		requestChan: requestChan,
		logger:      logger,
	}
}

// Send sends a JSON-RPC response over SSE
func (t *sseTransport) Send(response *JSONRPCResponse) error {
	return t.sseManager.SendToClient(t.clientID, response)
}

// Receive receives a JSON-RPC request (from the request channel)
func (t *sseTransport) Receive() (*JSONRPCRequest, error) {
	request, ok := <-t.requestChan
	if !ok {
		return nil, io.EOF
	}
	return request, nil
}

// Close closes the SSE transport
func (t *sseTransport) Close() error {
	// The SSE manager handles client cleanup
	return nil
}

// MCPOverSSE configures MCP to use SSE transport
func MCPOverSSE(endpoint string) MCPTransportConfig {
	return func(o *mcpTransportOptions) {
		o.transport = HTTPTransport // Still HTTP-based
		o.endpoint = endpoint
		// Additional SSE-specific configuration could go here
	}
}