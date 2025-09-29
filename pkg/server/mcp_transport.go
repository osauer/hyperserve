package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// DISCOVERY ENDPOINTS
// =============================================================================

// DiscoveryPolicy defines how MCP tools and resources are exposed in discovery endpoints
type DiscoveryPolicy int

const (
	// DiscoveryPublic shows all discoverable tools/resources (default)
	DiscoveryPublic DiscoveryPolicy = iota
	// DiscoveryCount only shows counts, not names
	DiscoveryCount
	// DiscoveryAuthenticated shows all if request has valid auth
	DiscoveryAuthenticated
	// DiscoveryNone hides all tool/resource information
	DiscoveryNone
)

// MCPDiscoveryInfo represents the discovery information for MCP endpoints
type MCPDiscoveryInfo struct {
	Version      string                 `json:"version"`
	Transports   []MCPTransportInfo     `json:"transports"`
	Endpoints    map[string]string      `json:"endpoints"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// MCPTransportInfo describes available transport mechanisms
type MCPTransportInfo struct {
	Type        string            `json:"type"`
	Endpoint    string            `json:"endpoint"`
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// setupDiscoveryEndpoints registers the discovery endpoints for Claude Code
func (srv *Server) setupDiscoveryEndpoints() {
	if !srv.MCPEnabled() {
		return
	}

	// Register /.well-known/mcp.json endpoint
	srv.mux.HandleFunc("/.well-known/mcp.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		discoveryInfo := srv.buildDiscoveryInfo(r)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
		if err := json.NewEncoder(w).Encode(discoveryInfo); err != nil {
			logger.Error("Failed to encode discovery info", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})

	// Register /mcp/discover endpoint
	srv.mux.HandleFunc(srv.Options.MCPEndpoint+"/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		discoveryInfo := srv.buildDiscoveryInfo(r)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(discoveryInfo); err != nil {
			logger.Error("Failed to encode discovery info", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})

	logger.Debug("MCP discovery endpoints registered",
		"endpoints", []string{"/.well-known/mcp.json", srv.Options.MCPEndpoint + "/discover"})
}

// buildDiscoveryInfo constructs the discovery information based on server configuration
func (srv *Server) buildDiscoveryInfo(r *http.Request) MCPDiscoveryInfo {
	// Determine the base URL
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if host == "" {
		host = "localhost" + srv.Options.Addr
	}

	baseURL := scheme + "://" + host
	mcpEndpoint := baseURL + srv.Options.MCPEndpoint

	info := MCPDiscoveryInfo{
		Version: MCPVersion,
		Transports: []MCPTransportInfo{
			{
				Type:        "http",
				Endpoint:    mcpEndpoint,
				Description: "Standard HTTP POST requests with JSON-RPC 2.0",
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			{
				Type:        "sse",
				Endpoint:    mcpEndpoint,
				Description: "Server-Sent Events for real-time communication",
				Headers: map[string]string{
					"Accept": "text/event-stream",
				},
			},
		},
		Endpoints: map[string]string{
			"mcp":        mcpEndpoint,
			"initialize": mcpEndpoint,
			"tools":      mcpEndpoint,
			"resources":  mcpEndpoint,
		},
	}

	// Add capabilities with dynamic tool/resource information
	if srv.mcpHandler != nil {
		// Get registered tools and resources
		tools := srv.mcpHandler.GetRegisteredTools()
		resources := srv.mcpHandler.GetRegisteredResources()

		// Build tool capability info based on policy
		toolCapability := map[string]interface{}{
			"supported": true,
			"count":     len(tools),
		}

		// Apply discovery policy for tools
		if srv.shouldIncludeToolList(r) {
			filteredTools := make([]string, 0, len(tools))
			for _, toolName := range tools {
				if srv.shouldExposeToolInDiscovery(toolName, r) {
					filteredTools = append(filteredTools, toolName)
				}
			}
			if len(filteredTools) > 0 {
				toolCapability["available"] = filteredTools
			}
		}

		// Build resource capability info
		resourceCapability := map[string]interface{}{
			"supported": true,
			"count":     len(resources),
		}

		// Resources follow the same policy as tools
		if srv.shouldIncludeToolList(r) {
			resourceCapability["available"] = resources
		}

		info.Capabilities = map[string]interface{}{
			"tools":     toolCapability,
			"resources": resourceCapability,
			"sse": map[string]interface{}{
				"enabled":       true,
				"endpoint":      "same",
				"headerRouting": true,
			},
		}

		// Add transport-specific capabilities
		if srv.Options.MCPTransport == StdioTransport {
			info.Capabilities["stdio"] = map[string]interface{}{
				"supported": true,
			}
		}
	}

	return info
}

// shouldIncludeToolList determines if tool/resource lists should be included based on policy
func (srv *Server) shouldIncludeToolList(r *http.Request) bool {
	switch srv.Options.MCPDiscoveryPolicy {
	case DiscoveryNone, DiscoveryCount:
		return false
	case DiscoveryAuthenticated:
		// Check for Authorization header
		return r.Header.Get("Authorization") != ""
	case DiscoveryPublic:
		return true
	default:
		return true // Default to public
	}
}

// shouldExposeToolInDiscovery determines if a specific tool should be exposed
func (srv *Server) shouldExposeToolInDiscovery(toolName string, r *http.Request) bool {
	// Use custom filter if provided
	if srv.Options.MCPDiscoveryFilter != nil {
		return srv.Options.MCPDiscoveryFilter(toolName, r)
	}

	// Default filtering logic
	switch srv.Options.MCPDiscoveryPolicy {
	case DiscoveryNone:
		return false

	case DiscoveryCount:
		return false // Only counts, no names

	case DiscoveryAuthenticated:
		// Must have auth to see any tools
		if r.Header.Get("Authorization") == "" {
			return false
		}
		// Fall through to default filtering

	case DiscoveryPublic:
		// Apply default filtering rules
	}

	// Default rules for all policies except None/Count

	// Hide internal tools
	if strings.HasPrefix(toolName, "internal_") || strings.HasPrefix(toolName, "_") {
		return false
	}

	// Hide sensitive tools unless in dev mode
	if !srv.Options.MCPDev {
		if strings.Contains(toolName, "debug") || strings.Contains(toolName, "admin") {
			return false
		}
		// Hide dev tools like server_control
		if toolName == "server_control" || toolName == "request_debugger" {
			return false
		}
	}

	// Check if tool implements IsDiscoverable
	if tool, exists := srv.mcpHandler.GetToolByName(toolName); exists {
		if discoverable, ok := tool.(interface{ IsDiscoverable() bool }); ok {
			return discoverable.IsDiscoverable()
		}
	}

	return true // Default to discoverable
}

// getMCPBaseURL returns the base URL for MCP endpoints, handling various host configurations
func getMCPBaseURL(r *http.Request, addr string) string {
	// Check for forwarded headers first
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		scheme := "http"
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		return scheme + "://" + forwardedHost
	}

	// Use the Host header if available
	if r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + r.Host
	}

	// Fallback to configured address
	host := "localhost"
	if addr != "" && !strings.HasPrefix(addr, ":") {
		// Extract host from addr if it's not just a port
		parts := strings.Split(addr, ":")
		if len(parts) > 0 && parts[0] != "" {
			host = parts[0]
		}
	}

	// Extract port from addr
	port := "8080"
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		port = addr[idx+1:]
	}

	return "http://" + host + ":" + port
}

// =============================================================================
// SSE TRANSPORT
// =============================================================================

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
	initialized   bool         // Track if client has completed initialization
	ready         bool         // Track if client is ready to receive messages
	mu            sync.RWMutex // Protect state fields
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

// =============================================================================
// STDIO TRANSPORT
// =============================================================================

// stdioTransport implements MCPTransport for stdin/stdout communication
// Note: Both Send and Receive are thread-safe, protected by mutex
type stdioTransport struct {
	scanner *bufio.Scanner
	encoder *json.Encoder
	logger  *slog.Logger
	mu      sync.Mutex // Protects both encoder and scanner for thread safety
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(logger *slog.Logger) *stdioTransport {
	scanner := bufio.NewScanner(os.Stdin)
	// Set reasonable buffer limits to prevent memory exhaustion
	// Max line size: 1MB (suitable for most JSON-RPC requests)
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, 0, 64*1024)      // 64KB initial buffer
	scanner.Buffer(buf, maxScanTokenSize)

	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(os.Stdout),
		logger:  logger,
	}
}

// NewStdioTransportWithIO creates a new stdio transport with custom IO
func NewStdioTransportWithIO(r io.Reader, w io.Writer, logger *slog.Logger) *stdioTransport {
	scanner := bufio.NewScanner(r)
	// Set reasonable buffer limits to prevent memory exhaustion
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, 0, 64*1024)      // 64KB initial buffer
	scanner.Buffer(buf, maxScanTokenSize)

	return &stdioTransport{
		scanner: scanner,
		encoder: json.NewEncoder(w),
		logger:  logger,
	}
}

// Send sends a JSON-RPC response to stdout
func (t *stdioTransport) Send(response *JSONRPCResponse) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.encoder.Encode(response); err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return nil
}

// Receive receives a JSON-RPC request from stdin
func (t *stdioTransport) Receive() (*JSONRPCRequest, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, fmt.Errorf("scanner error: %w", err)
		}
		return nil, io.EOF
	}

	var request JSONRPCRequest
	if err := json.Unmarshal(t.scanner.Bytes(), &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &request, nil
}

// Close closes the stdio transport (no-op)
func (t *stdioTransport) Close() error {
	return nil
}

// createErrorResponse creates a standard JSON-RPC error response
func createErrorResponse(code int, message string, data interface{}) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// RunStdioLoop runs the MCP handler in stdio mode
// The loop continues processing requests until EOF is received on stdin.
// EOF is treated as a normal shutdown signal (e.g., when stdin is closed).
// This behavior is appropriate for stdio servers which typically run
// for the lifetime of the parent process.
func (h *MCPHandler) RunStdioLoop() error {
	transport := NewStdioTransport(h.logger)
	// Note: Close() is currently a no-op but called for future compatibility
	defer transport.Close()

	h.logger.Debug("MCP stdio server started")

	// Main message loop
	for {
		err := h.ProcessRequestWithTransport(transport)
		if errors.Is(err, io.EOF) {
			h.logger.Debug("MCP stdio server shutting down", "reason", "EOF received")
			break
		}
		if err != nil {
			h.logger.Error("Error processing request", "error", err)
			// Determine appropriate error code based on error type
			errorCode := ErrorCodeInternalError
			if strings.Contains(err.Error(), "unmarshal") || strings.Contains(err.Error(), "parse") {
				errorCode = ErrorCodeParseError
			} else if strings.Contains(err.Error(), "scanner error") {
				errorCode = ErrorCodeInvalidRequest
			}

			// Send error response
			errorResponse := createErrorResponse(errorCode, "Request processing error", err.Error())
			if sendErr := transport.Send(errorResponse); sendErr != nil {
				h.logger.Error("Failed to send error response", "error", sendErr)
				// Critical failure: both request processing and error response failed
				// Log the critical state but continue processing to maintain service
				h.logger.Error("Critical: Unable to send error response to client",
					"original_error", err.Error(),
					"send_error", sendErr.Error())
			}
		}
	}

	return nil
}
