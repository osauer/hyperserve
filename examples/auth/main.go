package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/osauer/hyperserve"
	"golang.org/x/time/rate"
)

// AuthProvider defines the interface for authentication providers
type AuthProvider interface {
	Validate(token string) (SessionInfo, error)
	Name() string
}

// SessionInfo contains user session information
type SessionInfo struct {
	UserID      string              `json:"user_id"`
	Username    string              `json:"username"`
	Roles       []string            `json:"roles"`
	Permissions []string            `json:"permissions"`
	ExpiresAt   time.Time           `json:"expires_at"`
	Metadata    map[string]string   `json:"metadata"`
}

// MultiAuthValidator supports multiple authentication methods
type MultiAuthValidator struct {
	providers    []AuthProvider
	auditLogger  *AuditLogger
	rateLimiters map[string]*rate.Limiter
	mu           sync.RWMutex
}

// NewMultiAuthValidator creates a new multi-method auth validator
func NewMultiAuthValidator(providers []AuthProvider, auditLogger *AuditLogger) *MultiAuthValidator {
	return &MultiAuthValidator{
		providers:    providers,
		auditLogger:  auditLogger,
		rateLimiters: make(map[string]*rate.Limiter),
	}
}

// ValidateToken implements the hyperserve.AuthTokenValidatorFunc
func (m *MultiAuthValidator) ValidateToken(token string) (bool, error) {
	// Extract token type and value
	parts := strings.SplitN(token, " ", 2)
	if len(parts) != 2 {
		m.auditLogger.LogFailure("", "", "invalid_token_format", nil)
		return false, fmt.Errorf("invalid token format")
	}

	tokenType := strings.ToLower(parts[0])
	tokenValue := parts[1]

	// Try each provider
	for _, provider := range m.providers {
		if !m.shouldTryProvider(provider, tokenType) {
			continue
		}

		// Check rate limit
		if !m.checkRateLimit(tokenValue) {
			m.auditLogger.LogFailure("", tokenValue, "rate_limit_exceeded", nil)
			return false, fmt.Errorf("rate limit exceeded")
		}

		session, err := provider.Validate(tokenValue)
		if err == nil {
			// Successful validation
			m.auditLogger.LogSuccess(session.UserID, session.Username, provider.Name(), session.Metadata)
			// Store session info in request context via middleware
			return true, nil
		}
	}

	m.auditLogger.LogFailure("", tokenValue, "invalid_credentials", nil)
	return false, fmt.Errorf("invalid credentials")
}

func (m *MultiAuthValidator) shouldTryProvider(provider AuthProvider, tokenType string) bool {
	switch provider.Name() {
	case "jwt":
		return tokenType == "bearer"
	case "apikey":
		return tokenType == "apikey"
	case "basic":
		return tokenType == "basic"
	default:
		return false
	}
}

func (m *MultiAuthValidator) checkRateLimit(identifier string) bool {
	m.mu.Lock()
	limiter, exists := m.rateLimiters[identifier]
	if !exists {
		// 10 requests per minute per identifier
		limiter = rate.NewLimiter(rate.Every(6*time.Second), 10)
		m.rateLimiters[identifier] = limiter
	}
	m.mu.Unlock()

	return limiter.Allow()
}

// JWTProvider handles JWT token validation
type JWTProvider struct {
	publicKey *rsa.PublicKey
	issuer    string
}

func NewJWTProvider(publicKeyPEM string, issuer string) (*JWTProvider, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return &JWTProvider{
		publicKey: publicKey,
		issuer:    issuer,
	}, nil
}

func (j *JWTProvider) Validate(token string) (SessionInfo, error) {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.publicKey, nil
	})

	if err != nil {
		return SessionInfo{}, err
	}

	if !parsedToken.Valid {
		return SessionInfo{}, fmt.Errorf("invalid token")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return SessionInfo{}, fmt.Errorf("invalid claims")
	}

	// Validate issuer
	if issuer, ok := claims["iss"].(string); !ok || issuer != j.issuer {
		return SessionInfo{}, fmt.Errorf("invalid issuer")
	}

	// Extract session info
	session := SessionInfo{
		UserID:   claims["sub"].(string),
		Username: claims["username"].(string),
		Metadata: map[string]string{"auth_method": "jwt"},
	}

	// Parse roles
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, role := range roles {
			if roleStr, ok := role.(string); ok {
				session.Roles = append(session.Roles, roleStr)
			}
		}
	}

	// Parse permissions
	if perms, ok := claims["permissions"].([]interface{}); ok {
		for _, perm := range perms {
			if permStr, ok := perm.(string); ok {
				session.Permissions = append(session.Permissions, permStr)
			}
		}
	}

	// Set expiration
	if exp, ok := claims["exp"].(float64); ok {
		session.ExpiresAt = time.Unix(int64(exp), 0)
	}

	return session, nil
}

func (j *JWTProvider) Name() string {
	return "jwt"
}

// APIKeyProvider handles API key validation
type APIKeyProvider struct {
	keys map[string]APIKey
	mu   sync.RWMutex
}

type APIKey struct {
	Key         string
	UserID      string
	Username    string
	Roles       []string
	Permissions []string
	ExpiresAt   time.Time
	RateLimit   int // requests per minute
}

func NewAPIKeyProvider() *APIKeyProvider {
	return &APIKeyProvider{
		keys: make(map[string]APIKey),
	}
}

func (a *APIKeyProvider) AddKey(key APIKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keys[key.Key] = key
}

func (a *APIKeyProvider) Validate(token string) (SessionInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	apiKey, exists := a.keys[token]
	if !exists {
		return SessionInfo{}, fmt.Errorf("invalid API key")
	}

	if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
		return SessionInfo{}, fmt.Errorf("API key expired")
	}

	return SessionInfo{
		UserID:      apiKey.UserID,
		Username:    apiKey.Username,
		Roles:       apiKey.Roles,
		Permissions: apiKey.Permissions,
		ExpiresAt:   apiKey.ExpiresAt,
		Metadata: map[string]string{
			"auth_method": "apikey",
			"rate_limit":  fmt.Sprintf("%d/min", apiKey.RateLimit),
		},
	}, nil
}

func (a *APIKeyProvider) Name() string {
	return "apikey"
}

// BasicAuthProvider handles basic authentication
type BasicAuthProvider struct {
	users map[string]User
	mu    sync.RWMutex
}

type User struct {
	Username    string
	Password    string // In production, this should be a hashed password
	UserID      string
	Roles       []string
	Permissions []string
}

func NewBasicAuthProvider() *BasicAuthProvider {
	return &BasicAuthProvider{
		users: make(map[string]User),
	}
}

func (b *BasicAuthProvider) AddUser(user User) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.users[user.Username] = user
}

func (b *BasicAuthProvider) Validate(token string) (SessionInfo, error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("invalid base64 encoding")
	}

	// Split username:password
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return SessionInfo{}, fmt.Errorf("invalid basic auth format")
	}

	username := parts[0]
	password := parts[1]

	b.mu.RLock()
	defer b.mu.RUnlock()

	user, exists := b.users[username]
	if !exists {
		return SessionInfo{}, fmt.Errorf("invalid credentials")
	}

	// Timing-safe password comparison
	if subtle.ConstantTimeCompare([]byte(password), []byte(user.Password)) != 1 {
		return SessionInfo{}, fmt.Errorf("invalid credentials")
	}

	return SessionInfo{
		UserID:      user.UserID,
		Username:    user.Username,
		Roles:       user.Roles,
		Permissions: user.Permissions,
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Basic auth sessions expire after 24 hours
		Metadata: map[string]string{
			"auth_method": "basic",
		},
	}, nil
}

func (b *BasicAuthProvider) Name() string {
	return "basic"
}

// AuditLogger logs authentication events
type AuditLogger struct {
	mu sync.Mutex
}

func NewAuditLogger() *AuditLogger {
	return &AuditLogger{}
}

func (a *AuditLogger) LogSuccess(userID, username, method string, metadata map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	log.Printf("[AUTH SUCCESS] UserID: %s, Username: %s, Method: %s, Metadata: %v",
		userID, username, method, metadata)
}

func (a *AuditLogger) LogFailure(userID, token, reason string, metadata map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Don't log full tokens for security
	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8] + "..."
	}

	log.Printf("[AUTH FAILURE] UserID: %s, Token: %s, Reason: %s, Metadata: %v",
		userID, tokenPrefix, reason, metadata)
}

// CheckPermission verifies if the request has required permissions
func CheckPermission(w http.ResponseWriter, r *http.Request, required ...string) bool {
	// In a real app, extract permissions from JWT claims or session
	// For this example, we'll return true for demonstration
	// In production, decode the JWT and check claims
	
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	
	// TODO: Decode JWT and check permissions
	// For now, simple check based on auth method
	if strings.HasPrefix(authHeader, "APIKey dev_api_key_admin") {
		return true // Admin has all permissions
	}
	
	// Check specific permissions based on requirements
	for _, perm := range required {
		if perm == "admin" && !strings.Contains(authHeader, "admin") {
			http.Error(w, "Forbidden - admin required", http.StatusForbidden)
			return false
		}
	}
	
	return true
}

func main() {
	// Load configuration based on environment
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	log.Printf("Starting auth example in %s mode", env)

	// Create audit logger
	auditLogger := NewAuditLogger()

	// Initialize auth providers
	providers := []AuthProvider{}

	// JWT Provider (production-ready)
	if env == "production" {
		// In production, load from secure key management
		publicKeyPEM := os.Getenv("JWT_PUBLIC_KEY")
		if publicKeyPEM != "" {
			jwtProvider, err := NewJWTProvider(publicKeyPEM, "hyperserve-auth")
			if err != nil {
				log.Printf("Failed to initialize JWT provider: %v", err)
			} else {
				providers = append(providers, jwtProvider)
			}
		}
	} else {
		// Development JWT provider with test keys
		testPublicKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0Z3VS6JJcds6IYwR+OO5
p3dqNisZGPHPL1+x23qJa+7qOaHrLCrYGjcLsHH1sQ0L7jxP4F6grDdG0Yu5bqWO
U4D+qnVJdCQHDTGhtZ3+DS8iu5oy2MB3SZmixu5ByZGEkZEYPSYXlOLbRAIQ1SQ9
WjeFqM3KYYdXWpvyhJguDMYZXKCG3vK1YlXUhMzpDhD8YnNxqIv96Ff4bOqIEC2b
DF3aTM7GmAEJPvWdAK1CRotcAHRfMDSuRaahvQXBKn16CfRIPbVNhgoysBEyFM9M
q5CmbYup5VlF1g5x25wKGPv7MWsGgQKNcBL1pqQj7h+aSUZFELFJoHLv7W+qQYVA
7QIDAQAB
-----END PUBLIC KEY-----`
		jwtProvider, _ := NewJWTProvider(testPublicKey, "hyperserve-auth")
		providers = append(providers, jwtProvider)
	}

	// API Key Provider
	apiKeyProvider := NewAPIKeyProvider()
	if env == "development" {
		// Add development API keys
		apiKeyProvider.AddKey(APIKey{
			Key:         "dev_api_key_admin",
			UserID:      "dev_admin",
			Username:    "dev_admin",
			Roles:       []string{"admin", "user"},
			Permissions: []string{"read", "write", "delete"},
			RateLimit:   100,
		})
		apiKeyProvider.AddKey(APIKey{
			Key:         "dev_api_key_user",
			UserID:      "dev_user",
			Username:    "dev_user",
			Roles:       []string{"user"},
			Permissions: []string{"read"},
			RateLimit:   50,
		})
	}
	providers = append(providers, apiKeyProvider)

	// Basic Auth Provider (for development/emergency access)
	if env != "production" {
		basicProvider := NewBasicAuthProvider()
		basicProvider.AddUser(User{
			Username:    "admin",
			Password:    "admin123", // In production, use bcrypt
			UserID:      "basic_admin",
			Roles:       []string{"admin"},
			Permissions: []string{"read", "write", "delete"},
		})
		providers = append(providers, basicProvider)
	}

	// Create multi-auth validator
	authValidator := NewMultiAuthValidator(providers, auditLogger)

	// Create server with auth
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8090"),
		hyperserve.WithAuthTokenValidator(authValidator.ValidateToken),
		hyperserve.WithRateLimit(60, 120), // 60 requests/minute, burst of 120
	)
	if err != nil {
		log.Fatal(err)
	}

	// Public routes (no auth required)
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"message": "Welcome to HyperServe Auth Example",
			"endpoints": map[string]string{
				"GET /":              "This help message",
				"GET /health":        "Health check (public)",
				"POST /login":        "Login endpoint (returns JWT)",
				"GET /api/profile":   "Get user profile (requires auth)",
				"GET /api/users":     "List users (requires admin)",
				"POST /api/resource": "Create resource (requires write permission)",
			},
			"auth_methods": []string{"Bearer (JWT)", "APIKey", "Basic"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	srv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// Login endpoint (generates JWT in dev mode)
	srv.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var loginReq struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// In production, validate against real user database
		if env == "development" && loginReq.Username == "testuser" && loginReq.Password == "testpass" {
			// Generate development JWT
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub":         "test_user_id",
				"username":    "testuser",
				"roles":       []string{"user"},
				"permissions": []string{"read"},
				"iss":         "hyperserve-auth",
				"exp":         time.Now().Add(24 * time.Hour).Unix(),
			})

			// In dev, use a simple secret
			tokenString, _ := token.SignedString([]byte("dev_secret"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"token": tokenString,
				"type":  "Bearer",
			})
			return
		}

		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	})

	// Apply auth middleware to all /api routes
	srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))

	// User profile (any authenticated user)
	srv.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		// In a real app, extract user info from token claims
		token := r.Header.Get("Authorization")
		parts := strings.SplitN(token, " ", 2)

		profile := map[string]interface{}{
			"message":  "This is your profile",
			"auth_method": parts[0],
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	})

	// Admin-only route
	srv.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		// Check admin permission
		if !CheckPermission(w, r, "admin") {
			return
		}
		
		users := []map[string]string{
			{"id": "1", "username": "admin", "role": "admin"},
			{"id": "2", "username": "user1", "role": "user"},
			{"id": "3", "username": "user2", "role": "user"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	// Write permission required
	srv.HandleFunc("/api/resource", func(w http.ResponseWriter, r *http.Request) {
		// Check write permission
		if !CheckPermission(w, r, "write") {
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Resource created",
			"id":      "res_" + generateID(),
		})
	})

	// Start server
	log.Printf("Auth example server starting on :8090")
	log.Printf("Environment: %s", env)
	log.Printf("Try these commands:")
	log.Printf("  curl http://localhost:8090/")
	log.Printf("  curl http://localhost:8090/health")
	log.Printf("  curl -H 'Authorization: APIKey dev_api_key_admin' http://localhost:8090/api/profile")
	log.Printf("  curl -u admin:admin123 http://localhost:8090/api/profile")

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:11]
}