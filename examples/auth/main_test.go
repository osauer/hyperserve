package main

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
)

func TestMultiAuthValidator(t *testing.T) {
	// Create test providers
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
		token     string
		wantValid bool
		wantID    string
	}{
		{
			name:      "Valid API Key",
			token:     "APIKey test_key",
			wantValid: true,
			wantID:    "test_user:testuser",
		},
		{
			name:      "Invalid API Key",
			token:     "APIKey invalid_key",
			wantValid: false,
			wantID:    "",
		},
		{
			name:      "Valid Basic Auth",
			token:     "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			wantValid: true,
			wantID:    "basic_test_user:testuser",
		},
		{
			name:      "Invalid Basic Auth",
			token:     "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass")),
			wantValid: false,
			wantID:    "",
		},
		{
			name:      "Invalid Token Format",
			token:     "InvalidToken",
			wantValid: false,
			wantID:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			gotID, gotValid := validator.ValidateToken(ctx, tt.token)

			if gotValid != tt.wantValid {
				t.Errorf("ValidateToken() valid = %v, want %v", gotValid, tt.wantValid)
			}

			if gotID != tt.wantID {
				t.Errorf("ValidateToken() ID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestAPIKeyProvider(t *testing.T) {
	provider := NewAPIKeyProvider()

	// Add test key
	testKey := APIKey{
		Key:         "test_api_key",
		UserID:      "user123",
		Username:    "testuser",
		Roles:       []string{"admin", "user"},
		Permissions: []string{"read", "write"},
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		RateLimit:   100,
	}
	provider.AddKey(testKey)

	// Add expired key
	expiredKey := APIKey{
		Key:       "expired_key",
		UserID:    "user456",
		Username:  "expireduser",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	provider.AddKey(expiredKey)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "Valid key",
			token:   "test_api_key",
			wantErr: false,
		},
		{
			name:    "Invalid key",
			token:   "invalid_key",
			wantErr: true,
		},
		{
			name:    "Expired key",
			token:   "expired_key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := provider.Validate(tt.token)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && session.UserID == "" {
				t.Error("Validate() returned empty UserID for valid key")
			}
		})
	}
}

func TestBasicAuthProvider(t *testing.T) {
	provider := NewBasicAuthProvider()

	// Add test user
	provider.AddUser(User{
		Username:    "testuser",
		Password:    "testpass123",
		UserID:      "user123",
		Roles:       []string{"user"},
		Permissions: []string{"read"},
	})

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "Valid credentials",
			token:   base64.StdEncoding.EncodeToString([]byte("testuser:testpass123")),
			wantErr: false,
		},
		{
			name:    "Invalid password",
			token:   base64.StdEncoding.EncodeToString([]byte("testuser:wrongpass")),
			wantErr: true,
		},
		{
			name:    "Invalid username",
			token:   base64.StdEncoding.EncodeToString([]byte("wronguser:testpass123")),
			wantErr: true,
		},
		{
			name:    "Invalid format",
			token:   base64.StdEncoding.EncodeToString([]byte("nouserpass")),
			wantErr: true,
		},
		{
			name:    "Invalid base64",
			token:   "not-base64!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := provider.Validate(tt.token)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && session.UserID == "" {
				t.Error("Validate() returned empty UserID for valid credentials")
			}
		})
	}
}

func TestJWTProvider(t *testing.T) {
	// Test RSA key pair
	privateKeyPEM := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS6JJcds6IYwR+OO5p3dqNisZGPHPL1+x23qJa+7qOaHr
LCrYGjcLsHH1sQ0L7jxP4F6grDdG0Yu5bqWOU4D+qnVJdCQHDTGhtZ3+DS8iu5oy
2MB3SZmixu5ByZGEkZEYPSYXlOLbRAIQ1SQ9WjeFqM3KYYdXWpvyhJguDMYZXKCG
3vK1YlXUhMzpDhD8YnNxqIv96Ff4bOqIEC2bDF3aTM7GmAEJPvWdAK1CRotcAHRf
MDSuRaahvQXBKn16CfRIPbVNhgoysBEyFM9Mq5CmbYup5VlF1g5x25wKGPv7MWsG
gQKNcBL1pqQj7h+aSUZFELFJoHLv7W+qQYVA7QIDAQABAoIBAQCZmGrk8BK5x/jJ
k+5xwq738KKC8e0WXzqjLAkIEaPi5X5Ib8/Zc5RaADLZWLhLQE1Kzl1xSCHfSqJL
8F9QhKPuwMn3b2afvKXqPJconSbG6TEkGXZY3WKmGb+Hzp3aDCN5c3kIbgCbFLxv
5se8yZV4MzyaMbdzB9L/KQwFjqWYqB7riCs6NnLMqaJdT0tMHr2CZVRJNxFDFmRy
T6PDXYhmrj8n/F5QFX8yuKt5l+HofJANYZ5jmoF9XGbWMQMI7E3NIAYIcP1zF1L5
FKDvqk2Y7PxATL7QQVlBuVRCGUxB3ePGwvPTfBF4BKIpQfRYjGNj7GnhKMoGqLCZ
I/MBWdgBAoGBAPO1bOg4aCXLEQBjXQcnGrF3QidQNn7s6RkZL6ILyHqBDzbLvXJh
7ZRuTcWi4yGYxl8r5foXR8TwpGEDsoM1p7qjQXMrVwEXPvD8N6z2CZGPaZKNiNNT
vG7gJFKcp14c8NHLH/4HxNR4GO6t0EBScLcawK9h7Tk0glDmBqhJT+mtAoGBANyr
oQNVqdIhX1cU5daIX6ef7y7FHGKGof0vJgHNvafTtLo5nGSdWPKC8TRLQVB8Z1vr
8PlQa4cFCTbWPHMcdBdL29kEK9/+KBVRQvQFPQPB3kYYXBbIYEL0zRf/2kFMOLNm
A0CKGTJccOqGLTaLYJT0gNEVK5WOaSWAqLBEhMKBAoGAesR9tlVGvUDFOdYvR7MJ
K0jVqMzk5NIIYmz5Zwjcy8nNY8P6QLcTMI7fDfFCSZd2K6LLGPPuCVAU2ngD5FDA
h/0IjEPUIt7nezy0HwHBcCeXz7kAQ8h+oTgGP0hFCFdsGX3GVLT3dVqksyYbUOXU
IWOYXCmPUZDP1nLx7VclpYECgYEAoOqCnrIKm+U8vqK3VBQw9RfCbeJ8LSgfRPP8
c2wSZAXuzx0m3mE5pFfdrfLvUY0d0PNnmQPZI9lAYmJ6QmghtD4NMhhusaAMt2Yg
A1ojFDddJPPQ/8bBCIuRd6AS5d6uQMEEgsJnALKVE5ndV7CnE1TmGQKtFbs9WLKS
78PGIAECgYBNetTusHuMbrDGC/pQlCmw0B3YuIuMdwISEZ/LIGgpvQE6RUkcKXvJ
zDKKr8uHY4C7fJRnrYLso5//PD0K8nY2K1qMWWNkqQvBWGPKxALnYFpEcr+ELAT1
wKKwOdC6/PYBEqEuqIGoclB1YYVTNZChiNwD7A3NyJYN8K3SgSJzdA==
-----END RSA PRIVATE KEY-----`

	publicKeyPEM := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0Z3VS6JJcds6IYwR+OO5
p3dqNisZGPHPL1+x23qJa+7qOaHrLCrYGjcLsHH1sQ0L7jxP4F6grDdG0Yu5bqWO
U4D+qnVJdCQHDTGhtZ3+DS8iu5oy2MB3SZmixu5ByZGEkZEYPSYXlOLbRAIQ1SQ9
WjeFqM3KYYdXWpvyhJguDMYZXKCG3vK1YlXUhMzpDhD8YnNxqIv96Ff4bOqIEC2b
DF3aTM7GmAEJPvWdAK1CRotcAHRfMDSuRaahvQXBKn16CfRIPbVNhgoysBEyFM9M
q5CmbYup5VlF1g5x25wKGPv7MWsGgQKNcBL1pqQj7h+aSUZFELFJoHLv7W+qQYVA
7QIDAQAB
-----END PUBLIC KEY-----`

	provider, err := NewJWTProvider(publicKeyPEM, "test-issuer")
	if err != nil {
		t.Fatalf("Failed to create JWT provider: %v", err)
	}

	// Parse private key for signing
	block, _ := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))

	// Create valid token
	validClaims := jwt.MapClaims{
		"sub":         "user123",
		"username":    "testuser",
		"roles":       []string{"admin", "user"},
		"permissions": []string{"read", "write"},
		"iss":         "test-issuer",
		"exp":         time.Now().Add(1 * time.Hour).Unix(),
	}
	validToken := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims)
	validTokenString, _ := validToken.SignedString(block)

	// Create token with wrong issuer
	wrongIssuerClaims := jwt.MapClaims{
		"sub":      "user123",
		"username": "testuser",
		"iss":      "wrong-issuer",
		"exp":      time.Now().Add(1 * time.Hour).Unix(),
	}
	wrongIssuerToken := jwt.NewWithClaims(jwt.SigningMethodRS256, wrongIssuerClaims)
	wrongIssuerTokenString, _ := wrongIssuerToken.SignedString(block)

	// Create expired token
	expiredClaims := jwt.MapClaims{
		"sub":      "user123",
		"username": "testuser",
		"iss":      "test-issuer",
		"exp":      time.Now().Add(-1 * time.Hour).Unix(),
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodRS256, expiredClaims)
	expiredTokenString, _ := expiredToken.SignedString(block)

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "Valid token",
			token:   validTokenString,
			wantErr: false,
		},
		{
			name:    "Wrong issuer",
			token:   wrongIssuerTokenString,
			wantErr: true,
		},
		{
			name:    "Expired token",
			token:   expiredTokenString,
			wantErr: true,
		},
		{
			name:    "Invalid token",
			token:   "invalid.token.here",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := provider.Validate(tt.token)

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && session.UserID == "" {
				t.Error("Validate() returned empty UserID for valid token")
			}
		})
	}
}

func TestRateLimiting(t *testing.T) {
	apiKeyProvider := NewAPIKeyProvider()
	apiKeyProvider.AddKey(APIKey{
		Key:         "rate_test_key",
		UserID:      "test_user",
		Username:    "testuser",
		Roles:       []string{"user"},
		Permissions: []string{"read"},
		RateLimit:   5, // Low limit for testing
	})

	auditLogger := NewAuditLogger()
	validator := NewMultiAuthValidator([]AuthProvider{apiKeyProvider}, auditLogger)

	ctx := context.Background()
	token := "APIKey rate_test_key"

	// Should allow initial requests
	for i := 0; i < 10; i++ {
		_, valid := validator.ValidateToken(ctx, token)
		if i < 10 && !valid {
			t.Errorf("Request %d should have been allowed", i)
		}
	}

	// Should be rate limited now
	_, valid := validator.ValidateToken(ctx, token)
	if valid {
		t.Error("Request should have been rate limited")
	}
}

func BenchmarkMultiAuthValidator(b *testing.B) {
	// Setup providers
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

	ctx := context.Background()
	token := "APIKey test_key_50"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateToken(ctx, token)
	}
}

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

	provider, _ := NewJWTProvider(publicKeyPEM, "test-issuer")

	// Create a valid token for benchmarking
	token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMTIzIiwidXNlcm5hbWUiOiJ0ZXN0dXNlciIsInJvbGVzIjpbImFkbWluIiwidXNlciJdLCJwZXJtaXNzaW9ucyI6WyJyZWFkIiwid3JpdGUiXSwiaXNzIjoidGVzdC1pc3N1ZXIiLCJleHAiOjk5OTk5OTk5OTl9.example"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.Validate(token)
	}
}