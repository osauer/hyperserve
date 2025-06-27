// Example of how to use the auth package of Hyperserve
package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/osauer/hyperserve"
)

func main() {
	// Create server with authentication
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithAuthTokenValidator(validateToken),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Public route (no auth required)
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `
<html>
<head><title>HyperServe Auth Example</title></head>  
<body>
<h1>HyperServe Authentication Example</h1>
<p>This is a public endpoint - no authentication required.</p>
<h2>Test the API:</h2>
<ul>
<li><a href="/public">Public Endpoint</a> - No auth required</li>
<li><a href="/api/protected">Protected Endpoint</a> - Requires authentication</li>
</ul>
<h3>Authentication:</h3>
<p>To access protected endpoints, add this header:</p>
<pre>Authorization: Bearer demo-token</pre>
<p>Valid tokens: demo-token, api-key-123, secret-token</p>
</body>
</html>`)
	})

	// Public API endpoint
	srv.HandleFunc("/public", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "This is a public endpoint", "authenticated": false}`)
	})

	// Add authentication middleware to all /api routes
	srv.AddMiddleware("/api", hyperserve.AuthMiddleware(srv.Options))

	// Protected API endpoints (require authentication)
	srv.HandleFunc("/api/protected", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "This is a protected endpoint", "authenticated": true}`)
	})

	srv.HandleFunc("/api/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// In a real app, you'd extract user info from the validated token
		fmt.Fprintf(w, `{"user": "demo-user", "authenticated": true, "permissions": ["read", "write"]}`)
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("Try these endpoints:")
	fmt.Println("  http://localhost:8080/ (public)")
	fmt.Println("  http://localhost:8080/public (public API)")
	fmt.Println("  http://localhost:8080/api/protected (requires auth)")
	fmt.Println("  http://localhost:8080/api/user (requires auth)")
	fmt.Println()
	fmt.Println("For protected endpoints, use:")
	fmt.Println("  curl -H 'Authorization: Bearer demo-token' http://localhost:8080/api/protected")

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

// validateToken demonstrates different token validation strategies
func validateToken(token string) (bool, error) {
	// Simple token validation - in production, use JWT, database lookup, etc.
	validTokens := map[string]bool{
		"demo-token":   true,
		"api-key-123":  true,
		"secret-token": true,
	}

	// Example of more sophisticated validation
	if strings.HasPrefix(token, "jwt-") {
		// In real applications, you would:
		// 1. Parse the JWT token
		// 2. Verify the signature
		// 3. Check expiration
		// 4. Validate claims
		// For demo purposes, accept any JWT-prefixed token
		return len(token) > 10, nil
	}

	// Simple token lookup
	return validTokens[token], nil
}