package hyperserve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// MCPUnifiedHandler implements the MCP SSE transport specification with unified endpoint
type MCPUnifiedHandler struct {
	*MCPHandler                    // Embed existing MCPHandler for backward compatibility
	sessionManager *MCPSessionManager // Protocol-compliant session management
	logger         *slog.Logger
	serverInfo     MCPServerInfo
	discovery      *MCPDiscoveryService // For Claude Code integration
}

// NewMCPUnifiedHandler creates a new unified MCP handler
func NewMCPUnifiedHandler(serverInfo MCPServerInfo) *MCPUnifiedHandler {
	baseHandler := NewMCPHandler(serverInfo)
	
	handler := &MCPUnifiedHandler{
		MCPHandler:     baseHandler,
		sessionManager: NewMCPSessionManager(),
		logger:         logger,
		serverInfo:     serverInfo,
		discovery:      NewMCPDiscoveryService(serverInfo),
	}
	
	// Override JSON-RPC methods to use session-aware handlers
	handler.registerUnifiedMethods()
	
	return handler
}

// ServeHTTP implements the unified MCP endpoint according to SSE transport specification
func (h *MCPUnifiedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Debug logging
	if h.logger.Enabled(context.Background(), slog.LevelDebug) {
		h.logger.Debug("Unified MCP endpoint called", 
			"path", r.URL.Path, 
			"method", r.Method,
			"accept", r.Header.Get("Accept"),
			"user_agent", r.Header.Get("User-Agent"),
		)
	}
	
	// Handle discovery endpoints first
	if h.discovery.HandleDiscoveryRequest(w, r) {
		return
	}
	
	// Check for SSE connection based on Accept header (MCP SSE transport spec)
	acceptHeader := r.Header.Get("Accept")
	if strings.Contains(acceptHeader, "text/event-stream") {
		h.handleSSEConnection(w, r)
		return
	}
	
	// Handle GET requests with helpful information
	if r.Method == http.MethodGet {
		h.handleGetRequest(w, r)
		return
	}
	
	// Handle HTTP POST requests
	if r.Method == http.MethodPost {
		h.handleHTTPRequest(w, r)
		return
	}
	
	// Method not allowed
	http.Error(w, "Method not allowed. Use POST for HTTP transport or GET with Accept: text/event-stream for SSE.", http.StatusMethodNotAllowed)
}

// handleHTTPRequest handles regular HTTP POST requests
func (h *MCPUnifiedHandler) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Parse JSON-RPC request
	var request JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.sendErrorResponse(w, &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      nil,
			Error:   &JSONRPCError{Code: -32700, Message: "Parse error"},
		})
		return
	}
	
	// For HTTP transport, create or get session based on request
	sessionID := h.getOrCreateHTTPSession(&request)
	session, exists := h.sessionManager.GetSession(sessionID)
	if !exists {
		h.sendErrorResponse(w, &JSONRPCResponse{
			JSONRPC: JSONRPCVersion,
			ID:      request.ID,
			Error:   &JSONRPCError{Code: -32000, Message: "Session not found"},
		})
		return
	}
	
	// Process the request with session context
	response := h.processRequestWithSession(session, &request)
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSSEConnection handles Server-Sent Events connections
func (h *MCPUnifiedHandler) handleSSEConnection(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("X-Accel-Buffering", "no")
	
	// Generate session ID
	sessionID := generateSessionID()
	session := h.sessionManager.CreateSession(sessionID, TransportSSE)
	
	// Create SSE client
	sseClient := newSSEClient(sessionID, w, flusher)
	session.SetSSEClient(sseClient)
	
	h.logger.Info("MCP SSE client connected", "session", sessionID)
	
	// Send initial connection event according to MCP spec
	h.sendSSEConnectionEvent(sseClient, sessionID)
	
	// Use request context
	ctx := r.Context()
	
	// Start ping timer for keepalive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	
	// Main event loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			h.logger.Debug("MCP SSE client disconnected", "session", sessionID)
			h.sessionManager.RemoveSession(sessionID)
			return
			
		case <-session.Context().Done():
			// Session closed
			return
			
		case response := <-sseClient.messageChan:
			// Send JSON-RPC response via SSE
			if response != nil {
				data, err := json.Marshal(response)
				if err != nil {
					h.logger.Error("Failed to marshal SSE response", "error", err, "session", sessionID)
					continue
				}
				
				if err := sseClient.writeSSEMessage("message", data); err != nil {
					h.logger.Error("Failed to write SSE message", "error", err, "session", sessionID)
					return
				}
			}
			
		case <-pingTicker.C:
			// Send keepalive ping
			if err := h.sendSSEPing(sseClient); err != nil {
				h.logger.Debug("Failed to send SSE ping", "error", err, "session", sessionID)
				return
			}
		}
	}
}

// handleGetRequest provides helpful information for GET requests
func (h *MCPUnifiedHandler) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	
	// Temporary simplified version to fix compilation error
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>MCP Unified Endpoint - %s</title></head>
<body>
    <h1>MCP Unified Endpoint - %s v%s</h1>
    <p>Protocol Support: HTTP POST %s and SSE GET %s</p>
    <p>Discovery: <a href="%s/discover">%s/discover</a></p>
    <p>HTTP Example: %s</p>
    <p>SSE Example: %s</p>
    <p>Active sessions: <strong>%s</strong></p>
</body>
</html>`, 
		h.serverInfo.Name, 
		h.serverInfo.Name, h.serverInfo.Version,
		r.URL.Path, r.URL.Path,
		r.URL.Path, r.URL.Path,
		r.URL.Path, r.URL.Path,
		fmt.Sprintf("%d", h.sessionManager.GetSessionCount()),
	)
}

// processRequestWithSession processes a request within a session context
func (h *MCPUnifiedHandler) processRequestWithSession(session *MCPSession, request *JSONRPCRequest) *JSONRPCResponse {
	start := time.Now()
	
	// Update session activity
	session.UpdateActivity()
	
	// Handle session state transitions
	if request.Method == "initialize" {
		if err := session.SetState(SessionStateInitialized); err != nil {
			return &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      request.ID,
				Error:   &JSONRPCError{Code: -32000, Message: fmt.Sprintf("State error: %s", err.Error())},
			}
		}
	} else if request.Method == "initialized" {
		if err := session.SetState(SessionStateReady); err != nil {
			return &JSONRPCResponse{
				JSONRPC: JSONRPCVersion,
				ID:      request.ID,
				Error:   &JSONRPCError{Code: -32000, Message: fmt.Sprintf("State error: %s", err.Error())},
			}
		}
	} else {
		// For other methods, ensure we're in a valid state
		currentState := session.GetState()
		if currentState == SessionStateReady || currentState == SessionStateActive {
			session.SetState(SessionStateActive)
		} else if currentState == SessionStateNew || currentState == SessionStateInitialized {
			// Allow some methods during initialization
			if request.Method != "ping" {
				return &JSONRPCResponse{
					JSONRPC: JSONRPCVersion,
					ID:      request.ID,
					Error:   &JSONRPCError{Code: -32000, Message: "Session not ready. Complete initialization first."},
				}
			}
		}
	}
	
	// Process with the existing RPC engine
	response := h.rpcEngine.ProcessRequestDirect(request)
	
	// Record metrics
	var responseErr error
	if response.Error != nil {
		responseErr = fmt.Errorf("error: %s", response.Error.Message)
	}
	h.metrics.recordRequest(request.Method, time.Since(start), responseErr)
	
	return response
}

// getOrCreateHTTPSession creates or retrieves a session for HTTP transport
func (h *MCPUnifiedHandler) getOrCreateHTTPSession(request *JSONRPCRequest) string {
	// For HTTP transport, we could use various strategies:
	// 1. Session ID in request metadata
	// 2. Create session per initialize request
	// 3. Use a default session for backward compatibility
	
	// For now, use a simple approach: one session per initialize request
	if request.Method == "initialize" {
		sessionID := generateSessionID()
		h.sessionManager.CreateSession(sessionID, TransportHTTP)
		return sessionID
	}
	
	// For other methods, try to find an existing HTTP session
	// This is a simplification - in a real implementation you'd want better session tracking
	sessions := h.sessionManager.ListSessions()
	for _, session := range sessions {
		if session.Transport == TransportHTTP && session.GetState() != SessionStateClosed {
			return session.ID
		}
	}
	
	// Fallback: create a new session
	sessionID := generateSessionID()
	h.sessionManager.CreateSession(sessionID, TransportHTTP)
	return sessionID
}

// sendSSEConnectionEvent sends the initial connection event for SSE
func (h *MCPUnifiedHandler) sendSSEConnectionEvent(client *SSEClient, sessionID string) {
	connectionEvent := map[string]interface{}{
		"type":      "connection",
		"sessionId": sessionID,
		"message":   "Connected to MCP server",
		"server":    h.serverInfo,
	}
	
	if data, err := json.Marshal(connectionEvent); err == nil {
		client.writeSSEMessage("connection", data)
	}
}

// sendSSEPing sends a keepalive ping via SSE
func (h *MCPUnifiedHandler) sendSSEPing(client *SSEClient) error {
	pingData := map[string]interface{}{
		"type":      "ping",
		"timestamp": time.Now().Unix(),
	}
	
	if data, err := json.Marshal(pingData); err == nil {
		return client.writeSSEMessage("ping", data)
	}
	
	return nil
}

// sendErrorResponse sends an error response for HTTP requests
func (h *MCPUnifiedHandler) sendErrorResponse(w http.ResponseWriter, response *JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(response)
}

// registerUnifiedMethods registers session-aware MCP methods
func (h *MCPUnifiedHandler) registerUnifiedMethods() {
	// Override the initialize method to handle session state
	h.rpcEngine.RegisterMethod("initialize", h.handleUnifiedInitialize)
	h.rpcEngine.RegisterMethod("initialized", h.handleUnifiedInitialized)
	
	// Keep all other existing methods from the base handler
	// They will automatically benefit from session management
}

// handleUnifiedInitialize handles the initialize method with session awareness
func (h *MCPUnifiedHandler) handleUnifiedInitialize(params interface{}) (interface{}, error) {
	var initParams MCPInitializeParams
	
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		
		if err := json.Unmarshal(paramBytes, &initParams); err != nil {
			return nil, fmt.Errorf("failed to unmarshal init params: %w", err)
		}
	}
	
	h.logger.Debug("MCP unified client initialized", 
		"client", initParams.ClientInfo.Name, 
		"version", initParams.ClientInfo.Version,
		"protocol", initParams.ProtocolVersion,
	)
	
	// Return server capabilities with session information
	return map[string]interface{}{
		"protocolVersion": MCPVersion,
		"capabilities": MCPCapabilities{
			Resources: &ResourcesCapability{
				Subscribe:   false,
				ListChanged: false,
			},
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		"serverInfo": h.serverInfo,
		"sessionInfo": map[string]interface{}{
			"transport":    "unified",
			"supportsSSE":  true,
			"supportsHTTP": true,
		},
	}, nil
}

// handleUnifiedInitialized handles the initialized notification
func (h *MCPUnifiedHandler) handleUnifiedInitialized(params interface{}) (interface{}, error) {
	h.logger.Debug("MCP unified client confirmed initialization")
	return nil, nil
}

// Close closes the unified handler and cleans up resources
func (h *MCPUnifiedHandler) Close() error {
	if h.sessionManager != nil {
		h.sessionManager.Close()
	}
	return nil
}