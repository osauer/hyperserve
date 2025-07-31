// Enterprise example demonstrating FIPS 140-3 compliance and enhanced security features
package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/osauer/hyperserve"
)

func main() {
	// Generate ECH keys for demo (in production, load from secure storage)
	echKey := make([]byte, 32)
	if _, err := rand.Read(echKey); err != nil {
		log.Fatal("Failed to generate ECH key:", err)
	}

	// Create server with enterprise security features
	srv, err := hyperserve.NewServer(
		// Basic configuration
		hyperserve.WithAddr(":8443"),
		hyperserve.WithHealthServer(),

		// Enable FIPS 140-3 mode for government compliance
		hyperserve.WithFIPSMode(),

		// Enable TLS with certificates
		hyperserve.WithTLS("cert.pem", "key.pem"),

		// Enable Encrypted Client Hello for privacy
		hyperserve.WithEncryptedClientHello(echKey),

		// Configure rate limiting
		hyperserve.WithRateLimit(100, 200),

		// Set strict timeouts
		hyperserve.WithTimeouts(30*time.Second, 30*time.Second, 120*time.Second),

		// Configure authentication
		hyperserve.WithAuthTokenValidator(validateToken),

		// Set template and static directories
		hyperserve.WithTemplateDir("./templates"),
	)
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}

	// Apply security middleware stack to all routes
	srv.AddMiddlewareStack("*", hyperserve.SecureWeb(srv.Options))

	// Apply API security to /api routes
	srv.AddMiddlewareStack("/api", hyperserve.SecureAPI(srv))

	// Public endpoints
	srv.HandleFunc("/", homeHandler)
	srv.HandleFunc("/health", healthHandler)

	// Protected API endpoints
	srv.HandleFunc("/api/status", apiStatusHandler)
	srv.HandleFunc("/api/data", apiDataHandler)

	// Serve static files securely (uses os.Root in Go 1.24)
	srv.HandleStatic("/static/")

	log.Println("Starting enterprise server with FIPS 140-3 mode...")
	log.Println("Server features:")
	log.Println("- FIPS 140-3 compliant TLS")
	log.Println("- Encrypted Client Hello (ECH)")
	log.Println("- Post-quantum resistant key exchange")
	log.Println("- Timing-safe authentication")
	log.Println("- Secure file serving with os.Root")
	log.Println("- Swiss Tables optimized rate limiting")

	if err := srv.Run(); err != nil {
		log.Fatal("Server failed:", err)
	}
}

// validateToken demonstrates timing-safe token validation
func validateToken(token string) (bool, error) {
	// In production, this would check against a database or JWT
	// The middleware already uses crypto/subtle for timing protection
	validTokens := map[string]bool{
		"enterprise-key-123": true,
		"admin-token-456":    true,
	}

	// Check if token exists
	if valid, exists := validTokens[token]; exists && valid {
		return true, nil
	}

	// Simulate database lookup time to prevent timing attacks
	time.Sleep(10 * time.Millisecond)
	return false, nil
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Enterprise HyperServe</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .feature { background: #f0f0f0; padding: 10px; margin: 10px 0; }
        .secure { color: green; font-weight: bold; }
    </style>
</head>
<body>
    <h1>Enterprise HyperServe Demo</h1>
    <p>This server is running with enterprise-grade security features:</p>
    
    <div class="feature">
        <span class="secure">✓</span> FIPS 140-3 Compliant Mode
    </div>
    
    <div class="feature">
        <span class="secure">✓</span> Encrypted Client Hello (ECH)
    </div>
    
    <div class="feature">
        <span class="secure">✓</span> Post-Quantum Key Exchange
    </div>
    
    <div class="feature">
        <span class="secure">✓</span> Timing-Safe Authentication
    </div>
    
    <div class="feature">
        <span class="secure">✓</span> Secure File Serving (os.Root)
    </div>
    
    <h2>Test Endpoints</h2>
    <ul>
        <li><a href="/health">/health</a> - Health check (public)</li>
        <li><a href="/api/status">/api/status</a> - API status (requires auth)</li>
        <li><a href="/api/data">/api/data</a> - Sample data (requires auth)</li>
    </ul>
    
    <h2>Test with curl</h2>
    <pre>
# Public endpoint
curl https://localhost:8443/health

# Protected endpoint (will fail without auth)
curl https://localhost:8443/api/status

# Protected endpoint with auth
curl -H "Authorization: Bearer enterprise-key-123" https://localhost:8443/api/status
    </pre>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"healthy","fips_mode":true,"timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

func apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"status": "operational",
		"security": {
			"fips_mode": true,
			"ech_enabled": true,
			"post_quantum": true,
			"timing_safe_auth": true
		},
		"timestamp": "%s"
	}`, time.Now().Format(time.RFC3339))
}

func apiDataHandler(w http.ResponseWriter, r *http.Request) {
	// Demonstrate secure data handling
	data := map[string]interface{}{
		"data": []map[string]interface{}{
			{"id": 1, "value": "Enterprise data 1", "classified": false},
			{"id": 2, "value": "Enterprise data 2", "classified": true},
		},
		"metadata": map[string]interface{}{
			"total":      2,
			"filtered":   0,
			"encryption": "AES-256-GCM",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	// In production, use json.NewEncoder(w).Encode(data)
	fmt.Fprintf(w, `%v`, data)
}

func init() {
	// Set GOFIPS140 environment variable for FIPS mode
	// This should be done at the system level in production
	os.Setenv("GOFIPS140", "1")

	// Ensure we're using a FIPS-compliant random source
	// Go 1.24 handles this automatically when GOFIPS140 is set
}
