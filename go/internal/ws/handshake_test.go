package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateHandshake(t *testing.T) {
	tests := []struct {
		name       string
		setupReq   func() *http.Request
		wantErr    error
	}{
		{
			name: "valid handshake",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/ws", nil)
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				req.Header.Set("Sec-WebSocket-Version", "13")
				return req
			},
			wantErr: nil,
		},
		{
			name: "missing upgrade header",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/ws", nil)
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				req.Header.Set("Sec-WebSocket-Version", "13")
				return req
			},
			wantErr: ErrNotWebSocket,
		},
		{
			name: "wrong version",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/ws", nil)
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				req.Header.Set("Sec-WebSocket-Version", "8")
				return req
			},
			wantErr: ErrUnsupportedVersion,
		},
		{
			name: "missing key",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/ws", nil)
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Sec-WebSocket-Version", "13")
				return req
			},
			wantErr: ErrMissingKey,
		},
		{
			name: "POST method",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("POST", "/ws", nil)
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				req.Header.Set("Sec-WebSocket-Version", "13")
				return req
			},
			wantErr: ErrNotWebSocket,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			
			err := ValidateHandshake(req)
			if err != tt.wantErr {
				t.Errorf("ValidateHandshake() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateAcceptKey(t *testing.T) {
	// Test vector from RFC 6455
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	expected := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	
	result := generateAcceptKey(key)
	if result != expected {
		t.Errorf("generateAcceptKey() = %v, want %v", result, expected)
	}
}

func TestSubprotocolNegotiation(t *testing.T) {
	tests := []struct {
		name     string
		client   []string
		server   []string
		expected string
	}{
		{
			name:     "exact match",
			client:   []string{"chat", "superchat"},
			server:   []string{"chat"},
			expected: "chat",
		},
		{
			name:     "no match",
			client:   []string{"chat", "superchat"},
			server:   []string{"mqtt"},
			expected: "",
		},
		{
			name:     "multiple server protocols",
			client:   []string{"mqtt", "chat"},
			server:   []string{"chat", "mqtt"},
			expected: "mqtt", // First client preference
		},
		{
			name:     "empty client",
			client:   []string{},
			server:   []string{"chat"},
			expected: "",
		},
		{
			name:     "empty server",
			client:   []string{"chat"},
			server:   []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := negotiateSubprotocol(tt.client, tt.server)
			if result != tt.expected {
				t.Errorf("negotiateSubprotocol() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseSubprotocols(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected []string
	}{
		{
			name:     "single protocol",
			header:   "chat",
			expected: []string{"chat"},
		},
		{
			name:     "multiple protocols",
			header:   "chat, superchat, mqtt",
			expected: []string{"chat", "superchat", "mqtt"},
		},
		{
			name:     "with spaces",
			header:   "  chat  ,  superchat  ",
			expected: []string{"chat", "superchat"},
		},
		{
			name:     "empty",
			header:   "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSubprotocols(tt.header)
			if len(result) != len(tt.expected) {
				t.Errorf("parseSubprotocols() len = %v, want %v", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseSubprotocols()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		requestHost string
		opts        *HandshakeOptions
		wantAllow   bool
	}{
		{
			name:        "nil check function allows",
			origin:      "https://example.com",
			requestHost: "api.example.com",
			opts:        &HandshakeOptions{},
			wantAllow:   true,
		},
		{
			name:        "custom check function denies",
			origin:      "https://evil.com",
			requestHost: "api.example.com",
			opts: &HandshakeOptions{
				CheckOrigin: func(r *http.Request) bool {
					return false
				},
			},
			wantAllow: false,
		},
		{
			name:        "custom check function allows",
			origin:      "https://trusted.com",
			requestHost: "api.example.com",
			opts: &HandshakeOptions{
				CheckOrigin: func(r *http.Request) bool {
					return strings.Contains(r.Header.Get("Origin"), "trusted")
				},
			},
			wantAllow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Origin", tt.origin)
			req.Host = tt.requestHost
			
			// Set up valid WebSocket headers
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")
			
			w := httptest.NewRecorder()
			
			_, _, err := PerformHandshake(w, req, tt.opts)
			
			gotAllow := err != ErrBadHandshake
			if gotAllow != tt.wantAllow {
				t.Errorf("PerformHandshake() allowed = %v, want %v (err: %v)", gotAllow, tt.wantAllow, err)
			}
		})
	}
}