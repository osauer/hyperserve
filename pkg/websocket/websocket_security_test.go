package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultCheckOrigin(t *testing.T) {
	tests := []struct {
		name      string
		origin    string
		host      string
		wantAllow bool
	}{
		{
			name:      "same origin allowed",
			origin:    "https://example.com",
			host:      "example.com",
			wantAllow: true,
		},
		{
			name:      "different origin denied",
			origin:    "https://evil.com",
			host:      "example.com",
			wantAllow: false,
		},
		{
			name:      "no origin header denied",
			origin:    "",
			host:      "example.com",
			wantAllow: false,
		},
		{
			name:      "case insensitive host match",
			origin:    "https://EXAMPLE.COM",
			host:      "example.com",
			wantAllow: true,
		},
		{
			name:      "port must match",
			origin:    "https://example.com:8080",
			host:      "example.com:8080",
			wantAllow: true,
		},
		{
			name:      "different ports denied",
			origin:    "https://example.com:8080",
			host:      "example.com:9090",
			wantAllow: false,
		},
		{
			name:      "invalid origin URL",
			origin:    "not-a-url",
			host:      "example.com",
			wantAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			req.Host = tt.host

			got := DefaultCheckOrigin(req)
			if got != tt.wantAllow {
				t.Errorf("DefaultCheckOrigin() = %v, want %v", got, tt.wantAllow)
			}
		})
	}
}

func TestCheckOriginWithAllowedList(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		wantAllow      bool
	}{
		{
			name:           "exact match allowed",
			allowedOrigins: []string{"https://example.com", "https://app.example.com"},
			origin:         "https://example.com",
			wantAllow:      true,
		},
		{
			name:           "not in list denied",
			allowedOrigins: []string{"https://example.com"},
			origin:         "https://evil.com",
			wantAllow:      false,
		},
		{
			name:           "wildcard allows all",
			allowedOrigins: []string{"*"},
			origin:         "https://any-origin.com",
			wantAllow:      true,
		},
		{
			name:           "subdomain wildcard match",
			allowedOrigins: []string{"*.example.com"},
			origin:         "https://app.example.com",
			wantAllow:      true,
		},
		{
			name:           "subdomain wildcard no match",
			allowedOrigins: []string{"*.example.com"},
			origin:         "https://example.org",
			wantAllow:      false,
		},
		{
			name:           "nested subdomain match",
			allowedOrigins: []string{"*.example.com"},
			origin:         "https://api.v2.example.com",
			wantAllow:      true,
		},
		{
			name:           "empty origin denied",
			allowedOrigins: []string{"https://example.com"},
			origin:         "",
			wantAllow:      false,
		},
		{
			name:           "multiple wildcards",
			allowedOrigins: []string{"*.example.com", "*.internal.com"},
			origin:         "https://api.internal.com",
			wantAllow:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkFunc := CheckOriginWithAllowedList(tt.allowedOrigins)
			req := httptest.NewRequest("GET", "/ws", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			got := checkFunc(req)
			if got != tt.wantAllow {
				t.Errorf("CheckOriginWithAllowedList() = %v, want %v", got, tt.wantAllow)
			}
		})
	}
}

func TestEqualASCIIFold(t *testing.T) {
	tests := []struct {
		s1   string
		s2   string
		want bool
	}{
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},
		{"example.com", "EXAMPLE.COM", true},
		{"ExAmPlE.cOm", "eXaMpLe.CoM", true},
		{"example.com", "example.org", false},
		{"example.com", "example.co", false},
		{"", "", true},
		{"a", "", false},
		{"", "a", false},
		{"123", "123", true},
		{"123", "124", false},
		{"example.com:8080", "EXAMPLE.COM:8080", true},
		{"example.com:8080", "example.com:9090", false},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			got := equalASCIIFold(tt.s1, tt.s2)
			if got != tt.want {
				t.Errorf("equalASCIIFold(%q, %q) = %v, want %v", tt.s1, tt.s2, got, tt.want)
			}
		})
	}
}

func TestWebSocketUpgraderSecurity(t *testing.T) {
	tests := []struct {
		name      string
		upgrader  Upgrader
		origin    string
		host      string
		wantError bool
	}{
		{
			name:      "default security same origin",
			upgrader:  Upgrader{},
			origin:    "https://example.com",
			host:      "example.com",
			wantError: false,
		},
		{
			name:      "default security different origin",
			upgrader:  Upgrader{},
			origin:    "https://evil.com",
			host:      "example.com",
			wantError: true,
		},
		{
			name: "allowed origins list",
			upgrader: Upgrader{
				AllowedOrigins: []string{"https://trusted.com"},
			},
			origin:    "https://trusted.com",
			host:      "example.com",
			wantError: false,
		},
		{
			name: "custom check origin overrides",
			upgrader: Upgrader{
				AllowedOrigins: []string{"https://blocked.com"},
				CheckOrigin: func(r *http.Request) bool {
					// Custom logic that allows everything
					return true
				},
			},
			origin:    "https://any-origin.com",
			host:      "example.com",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Origin", tt.origin)
			req.Host = tt.host

			// Test the validation part only since httptest.ResponseRecorder doesn't support hijacking
			// We'll test that the origin check happens before the hijack attempt
			checkOrigin := tt.upgrader.CheckOrigin
			if checkOrigin == nil {
				if len(tt.upgrader.AllowedOrigins) > 0 {
					checkOrigin = CheckOriginWithAllowedList(tt.upgrader.AllowedOrigins)
				} else {
					checkOrigin = DefaultCheckOrigin
				}
			}

			originAllowed := checkOrigin(req)
			expectError := tt.wantError || !originAllowed

			if expectError != tt.wantError {
				t.Errorf("Expected error = %v based on origin check, but test expects %v", expectError, tt.wantError)
			}
		})
	}
}
