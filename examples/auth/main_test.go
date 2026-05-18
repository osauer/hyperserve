package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestMultiAuthValidator exercises validateAnyScheme — the multi-scheme
// entry point the API/Basic/Bearer routes use — across all three schemes.
// The previous (string, bool) signature was replaced with (bool, error) when
// the example was rewritten to source role/permission decisions from
// SessionInfo rather than from raw Authorization headers.
func TestMultiAuthValidator(t *testing.T) {
	apiKeyProvider := NewAPIKeyProvider()
	apiKeyProvider.AddKey(APIKey{
		Key:         "test_key",
		UserID:      "test_user",
		Username:    "testuser",
		Roles:       []string{"user"},
		Permissions: []string{"read"},
		RateLimit:   100,
	})

	basicProvider := NewBasicAuthProvider()
	basicProvider.AddUser(User{
		Username:    "testuser",
		Password:    "testpass",
		UserID:      "basic_test_user",
		Roles:       []string{"user"},
		Permissions: []string{"read"},
	})

	auditLogger := NewAuditLogger()
	validator := NewMultiAuthValidator([]AuthProvider{apiKeyProvider, basicProvider}, auditLogger)

	tests := []struct {
		name      string
		raw       string
		wantValid bool
		wantUser  string
	}{
		{"Valid API Key", "APIKey test_key", true, "testuser"},
		{"Invalid API Key", "APIKey invalid_key", false, ""},
		{
			"Valid Basic Auth",
			"Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			true,
			"testuser",
		},
		{
			"Invalid Basic Auth",
			"Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass")),
			false,
			"",
		},
		{"Invalid Token Format", "InvalidToken", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.SplitN(tt.raw, " ", 2)
			var scheme, value string
			if len(parts) == 2 {
				scheme = strings.ToLower(parts[0])
				value = parts[1]
			}
			session, ok := validator.validateAnyScheme(scheme, value)
			if ok != tt.wantValid {
				t.Errorf("validateAnyScheme valid = %v, want %v", ok, tt.wantValid)
			}
			if ok && session.Username != tt.wantUser {
				t.Errorf("session.Username = %q, want %q", session.Username, tt.wantUser)
			}
		})
	}
}

func TestAPIKeyProvider(t *testing.T) {
	provider := NewAPIKeyProvider()
	provider.AddKey(APIKey{
		Key:         "test_api_key",
		UserID:      "user123",
		Username:    "testuser",
		Roles:       []string{"admin", "user"},
		Permissions: []string{"read", "write"},
		RateLimit:   100,
	})

	t.Run("Valid key", func(t *testing.T) {
		s, err := provider.Validate("test_api_key")
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}
		if s.Username != "testuser" {
			t.Errorf("Username = %q, want %q", s.Username, "testuser")
		}
	})

	t.Run("Invalid key", func(t *testing.T) {
		if _, err := provider.Validate("missing"); err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("Expired key", func(t *testing.T) {
		expired := APIKey{
			Key:       "expired",
			UserID:    "u",
			Username:  "u",
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		provider.AddKey(expired)
		if _, err := provider.Validate("expired"); err == nil {
			t.Error("expected error for expired key")
		}
	})
}

func TestRateLimiter(t *testing.T) {
	apiKeyProvider := NewAPIKeyProvider()
	apiKeyProvider.AddKey(APIKey{
		Key:         "rate_test_key",
		UserID:      "rate_user",
		Username:    "rate_user",
		Roles:       []string{"user"},
		Permissions: []string{"read"},
		RateLimit:   5,
	})

	auditLogger := NewAuditLogger()
	validator := NewMultiAuthValidator([]AuthProvider{apiKeyProvider}, auditLogger)

	// Initial 10 should pass under the per-token rate cap of 10/min.
	for i := 0; i < 10; i++ {
		if _, ok := validator.validateAnyScheme("apikey", "rate_test_key"); !ok {
			t.Errorf("request %d unexpectedly denied", i)
		}
	}

	// 11th should be rate-limited (limiter is 10/min, capacity 10).
	if _, ok := validator.validateAnyScheme("apikey", "rate_test_key"); ok {
		t.Error("expected rate limit refusal")
	}
}

func BenchmarkMultiAuthValidator(b *testing.B) {
	apiKeyProvider := NewAPIKeyProvider()
	for i := 0; i < 100; i++ {
		apiKeyProvider.AddKey(APIKey{
			Key:         "test_key_" + string(rune(i)),
			UserID:      "user_" + string(rune(i)),
			Username:    "user_" + string(rune(i)),
			Roles:       []string{"user"},
			Permissions: []string{"read"},
			RateLimit:   1000,
		})
	}
	auditLogger := NewAuditLogger()
	validator := NewMultiAuthValidator([]AuthProvider{apiKeyProvider}, auditLogger)
	for b.Loop() {
		_, _ = validator.validateAnyScheme("apikey", "test_key_50")
	}
}

// BenchmarkJWTValidation is kept as a compilation guard for the JWTProvider
// path; it produces real signed tokens, decodes them, and checks claims.
func BenchmarkJWTValidation(b *testing.B) {
	publicKeyPEM := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0Z3VS6JJcds6IYwR+OO5
p3dqNisZGPHPL1+x23qJa+7qOaHrLCrYGjcLsHH1sQ0L7jxP4F6grDdG0Yu5bqWO
U4D+qnVJdCQHDTGhtZ3+DS8iu5oy2MB3SZmixu5ByZGEkZEYPSYXlOLbRAIQ1SQ9
WjeFqM3KYYdXWpvyhJguDMYZXKCG3vK1YlXUhMzpDhD8YnNxqIv96Ff4bOqIEC2b
DF3aTM7GmAEJPvWdAK1CRotcAHRfMDSuRaahvQXBKn16CfRIPbVNhgoysBEyFM9M
q5CmbYup5VlF1g5x25wKGPv7MWsGgQKNcBL1pqQj7h+aSUZFELFJoHLv7W+qQYVA
7QIDAQAB
-----END PUBLIC KEY-----`
	_, err := NewJWTProvider(publicKeyPEM, "test-issuer")
	if err != nil {
		b.Fatalf("NewJWTProvider: %v", err)
	}
	// Generate a token signed with HS256 (won't validate against the RSA
	// pubkey above — the bench just exercises the parse path).
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "x"})
	signed, _ := tok.SignedString([]byte("k"))
	b.ResetTimer()
	for b.Loop() {
		_, _ = jwt.Parse(signed, func(*jwt.Token) (any, error) { return []byte("k"), nil })
	}
}
