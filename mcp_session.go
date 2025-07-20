package hyperserve

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

// MCPSessionState represents the state of an MCP session
type MCPSessionState int

const (
	// SessionStateNew indicates a new session that hasn't been initialized
	SessionStateNew MCPSessionState = iota
	// SessionStateInitialized indicates the session has received initialize request
	SessionStateInitialized
	// SessionStateReady indicates the session has been marked ready after initialized notification
	SessionStateReady
	// SessionStateActive indicates the session is actively processing requests
	SessionStateActive
	// SessionStateClosed indicates the session has been closed
	SessionStateClosed
)

// String returns the string representation of the session state
func (s MCPSessionState) String() string {
	switch s {
	case SessionStateNew:
		return "new"
	case SessionStateInitialized:
		return "initialized"
	case SessionStateReady:
		return "ready"
	case SessionStateActive:
		return "active"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// MCPTransportMode represents the transport mode for the session
type MCPTransportMode int

const (
	// TransportHTTP indicates HTTP POST requests
	TransportHTTP MCPTransportMode = iota
	// TransportSSE indicates Server-Sent Events
	TransportSSE
)

// String returns the string representation of the transport mode
func (t MCPTransportMode) String() string {
	switch t {
	case TransportHTTP:
		return "http"
	case TransportSSE:
		return "sse"
	default:
		return "unknown"
	}
}

// MCPSession represents a protocol-compliant MCP session
type MCPSession struct {
	ID           string                 `json:"id"`
	State        MCPSessionState        `json:"state"`
	Transport    MCPTransportMode       `json:"transport"`
	ClientInfo   map[string]interface{} `json:"clientInfo,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	LastActivity time.Time              `json:"lastActivity"`
	
	// Internal fields
	mu           sync.RWMutex
	ctx          context.Context
	cancelFunc   context.CancelFunc
	sseClient    *SSEClient // Only set for SSE sessions
	logger       *slog.Logger
}

// NewMCPSession creates a new MCP session
func NewMCPSession(id string, transport MCPTransportMode) *MCPSession {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &MCPSession{
		ID:           id,
		State:        SessionStateNew,
		Transport:    transport,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ctx:          ctx,
		cancelFunc:   cancel,
		logger:       logger,
	}
}

// SetState safely updates the session state with validation
func (s *MCPSession) SetState(newState MCPSessionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Validate state transitions
	if !s.isValidTransition(s.State, newState) {
		return fmt.Errorf("invalid state transition from %s to %s", s.State, newState)
	}
	
	oldState := s.State
	s.State = newState
	s.LastActivity = time.Now()
	
	s.logger.Debug("MCP session state changed", 
		"session", s.ID, 
		"from", oldState, 
		"to", newState,
		"transport", s.Transport,
	)
	
	return nil
}

// GetState safely returns the current session state
func (s *MCPSession) GetState() MCPSessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetClientInfo stores client information from the initialize request
func (s *MCPSession) SetClientInfo(clientInfo map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ClientInfo = clientInfo
	s.LastActivity = time.Now()
}

// GetClientInfo safely returns the client information
func (s *MCPSession) GetClientInfo() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ClientInfo
}

// SetSSEClient associates an SSE client with this session
func (s *MCPSession) SetSSEClient(client *SSEClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sseClient = client
}

// GetSSEClient returns the associated SSE client (if any)
func (s *MCPSession) GetSSEClient() *SSEClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sseClient
}

// UpdateActivity updates the last activity timestamp
func (s *MCPSession) UpdateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActivity = time.Now()
}

// IsExpired checks if the session has expired
func (s *MCPSession) IsExpired(timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastActivity) > timeout
}

// Close closes the session and cleans up resources
func (s *MCPSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.State == SessionStateClosed {
		return nil // Already closed
	}
	
	s.State = SessionStateClosed
	s.cancelFunc()
	
	// Close SSE client if present
	if s.sseClient != nil {
		s.sseClient.Close()
		s.sseClient = nil
	}
	
	s.logger.Debug("MCP session closed", "session", s.ID)
	return nil
}

// Context returns the session context
func (s *MCPSession) Context() context.Context {
	return s.ctx
}

// isValidTransition checks if a state transition is valid
func (s *MCPSession) isValidTransition(from, to MCPSessionState) bool {
	switch from {
	case SessionStateNew:
		return to == SessionStateInitialized || to == SessionStateClosed
	case SessionStateInitialized:
		return to == SessionStateReady || to == SessionStateClosed
	case SessionStateReady:
		return to == SessionStateActive || to == SessionStateClosed
	case SessionStateActive:
		return to == SessionStateClosed // Can only go to closed from active
	case SessionStateClosed:
		return false // No transitions from closed
	default:
		return false
	}
}

// MCPSessionManager manages multiple MCP sessions
type MCPSessionManager struct {
	sessions      map[string]*MCPSession
	mu            sync.RWMutex
	logger        *slog.Logger
	cleanupTicker *time.Ticker
	cleanupDone   chan struct{}
	sessionTimeout time.Duration
}

// NewMCPSessionManager creates a new session manager
func NewMCPSessionManager() *MCPSessionManager {
	manager := &MCPSessionManager{
		sessions:       make(map[string]*MCPSession),
		logger:         logger,
		cleanupDone:    make(chan struct{}),
		sessionTimeout: 30 * time.Minute, // Default session timeout
	}
	
	// Start cleanup goroutine
	manager.cleanupTicker = time.NewTicker(5 * time.Minute)
	go manager.cleanupLoop()
	
	return manager
}

// CreateSession creates a new session with the specified transport
func (m *MCPSessionManager) CreateSession(sessionID string, transport MCPTransportMode) *MCPSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	session := NewMCPSession(sessionID, transport)
	m.sessions[sessionID] = session
	
	m.logger.Debug("MCP session created", 
		"session", sessionID, 
		"transport", transport,
		"total_sessions", len(m.sessions),
	)
	
	return session
}

// GetSession retrieves a session by ID
func (m *MCPSessionManager) GetSession(sessionID string) (*MCPSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	session, exists := m.sessions[sessionID]
	if exists && session != nil {
		session.UpdateActivity()
	}
	return session, exists
}

// RemoveSession removes a session by ID
func (m *MCPSessionManager) RemoveSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if session, exists := m.sessions[sessionID]; exists {
		session.Close()
		delete(m.sessions, sessionID)
		
		m.logger.Debug("MCP session removed", 
			"session", sessionID,
			"remaining_sessions", len(m.sessions),
		)
	}
}

// ListSessions returns a list of all active sessions
func (m *MCPSessionManager) ListSessions() []*MCPSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	sessions := make([]*MCPSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	
	return sessions
}

// GetSessionCount returns the number of active sessions
func (m *MCPSessionManager) GetSessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Close shuts down the session manager
func (m *MCPSessionManager) Close() error {
	// Stop cleanup goroutine
	m.cleanupTicker.Stop()
	close(m.cleanupDone)
	
	// Close all sessions
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, session := range m.sessions {
		session.Close()
	}
	
	m.sessions = make(map[string]*MCPSession)
	m.logger.Debug("MCP session manager closed")
	
	return nil
}

// cleanupLoop periodically cleans up expired sessions
func (m *MCPSessionManager) cleanupLoop() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanupExpiredSessions()
		case <-m.cleanupDone:
			return
		}
	}
}

// cleanupExpiredSessions removes expired sessions
func (m *MCPSessionManager) cleanupExpiredSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var expiredSessions []string
	
	for id, session := range m.sessions {
		if session.IsExpired(m.sessionTimeout) {
			expiredSessions = append(expiredSessions, id)
		}
	}
	
	for _, id := range expiredSessions {
		if session := m.sessions[id]; session != nil {
			session.Close()
		}
		delete(m.sessions, id)
	}
	
	if len(expiredSessions) > 0 {
		m.logger.Debug("Cleaned up expired MCP sessions", 
			"expired_count", len(expiredSessions),
			"remaining_sessions", len(m.sessions),
		)
	}
}

// generateSessionID generates a unique session ID for MCP sessions
func generateSessionID() string {
	return fmt.Sprintf("mcp-session-%d-%d", time.Now().UnixNano(), rand.Int())
}