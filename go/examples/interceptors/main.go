package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/osauer/hyperserve/go"
	"golang.org/x/time/rate"
)

// SimpleRateLimiter implements a basic rate limiter
type SimpleRateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rps      int
	burst    int
}

func NewSimpleRateLimiter(rps, burst int) *SimpleRateLimiter {
	return &SimpleRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      rps,
		burst:    burst,
	}
}

func (srl *SimpleRateLimiter) Allow(key string) bool {
	srl.mu.Lock()
	limiter, exists := srl.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(srl.rps), srl.burst)
		srl.limiters[key] = limiter
	}
	srl.mu.Unlock()
	
	return limiter.Allow()
}

// APIKeyValidator validates API keys and enriches requests with user info
type APIKeyValidator struct {
	apiKeys map[string]string // key -> user ID
}

func NewAPIKeyValidator() *APIKeyValidator {
	return &APIKeyValidator{
		apiKeys: map[string]string{
			"demo-key-123": "user-1",
			"demo-key-456": "user-2",
			"admin-key":    "admin",
		},
	}
}

func (akv *APIKeyValidator) Name() string {
	return "APIKeyValidator"
}

func (akv *APIKeyValidator) InterceptRequest(ctx context.Context, req *hyperserve.InterceptableRequest) (*hyperserve.InterceptorResponse, error) {
	// Skip validation for public endpoints
	if strings.HasPrefix(req.URL.Path, "/public") {
		return nil, nil
	}
	
	apiKey := req.Header.Get("X-API-Key")
	if apiKey == "" {
		return &hyperserve.InterceptorResponse{
			StatusCode: http.StatusUnauthorized,
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: []byte(`{"error": "API key required"}`),
		}, nil
	}
	
	userID, valid := akv.apiKeys[apiKey]
	if !valid {
		return &hyperserve.InterceptorResponse{
			StatusCode: http.StatusUnauthorized,
			Headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: []byte(`{"error": "Invalid API key"}`),
		}, nil
	}
	
	// Store user ID in metadata for use by handlers
	req.Metadata["user_id"] = userID
	req.Header.Set("X-User-ID", userID)
	
	return nil, nil
}

func (akv *APIKeyValidator) InterceptResponse(ctx context.Context, req *hyperserve.InterceptableRequest, resp *hyperserve.InterceptableResponse) error {
	// Add user ID to response headers
	if userID, ok := req.Metadata["user_id"].(string); ok {
		resp.Headers.Set("X-User-ID", userID)
	}
	return nil
}

// JSONTransformer adds metadata to JSON responses
type JSONTransformer struct{}

func (jt *JSONTransformer) Name() string {
	return "JSONTransformer"
}

func (jt *JSONTransformer) InterceptRequest(ctx context.Context, req *hyperserve.InterceptableRequest) (*hyperserve.InterceptorResponse, error) {
	return nil, nil
}

func (jt *JSONTransformer) InterceptResponse(ctx context.Context, req *hyperserve.InterceptableRequest, resp *hyperserve.InterceptableResponse) error {
	// Only transform JSON responses
	if !strings.Contains(resp.Headers.Get("Content-Type"), "application/json") {
		return nil
	}
	
	// Parse existing JSON
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &data); err != nil {
		// If it's not valid JSON, skip transformation
		return nil
	}
	
	// Add metadata
	metadata := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"version":   "1.0",
	}
	
	// Add user info if available
	if userID, ok := req.Metadata["user_id"].(string); ok {
		metadata["user_id"] = userID
	}
	
	// Wrap response with metadata
	wrapped := map[string]interface{}{
		"data":     data,
		"metadata": metadata,
	}
	
	// Re-encode
	newBody, err := json.Marshal(wrapped)
	if err != nil {
		return err
	}
	
	resp.Body.Reset()
	resp.Body.Write(newBody)
	resp.Headers.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))
	
	return nil
}

// CORSInterceptor adds CORS headers
type CORSInterceptor struct {
	allowedOrigins []string
}

func NewCORSInterceptor(origins []string) *CORSInterceptor {
	return &CORSInterceptor{
		allowedOrigins: origins,
	}
}

func (ci *CORSInterceptor) Name() string {
	return "CORSInterceptor"
}

func (ci *CORSInterceptor) InterceptRequest(ctx context.Context, req *hyperserve.InterceptableRequest) (*hyperserve.InterceptorResponse, error) {
	// Handle preflight requests
	if req.Method == "OPTIONS" {
		return &hyperserve.InterceptorResponse{
			StatusCode: http.StatusOK,
			Headers: http.Header{
				"Access-Control-Allow-Origin":  []string{"*"},
				"Access-Control-Allow-Methods": []string{"GET, POST, PUT, DELETE, OPTIONS"},
				"Access-Control-Allow-Headers": []string{"Content-Type, X-API-Key"},
				"Access-Control-Max-Age":       []string{"86400"},
			},
			Body: []byte(""),
		}, nil
	}
	
	return nil, nil
}

func (ci *CORSInterceptor) InterceptResponse(ctx context.Context, req *hyperserve.InterceptableRequest, resp *hyperserve.InterceptableResponse) error {
	origin := req.Header.Get("Origin")
	
	// Check if origin is allowed
	allowed := false
	for _, allowedOrigin := range ci.allowedOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			allowed = true
			break
		}
	}
	
	if allowed {
		resp.Headers.Set("Access-Control-Allow-Origin", origin)
		resp.Headers.Set("Access-Control-Allow-Credentials", "true")
	}
	
	return nil
}

func main() {
	// Create server
	srv, err := hyperserve.NewServer()
	if err != nil {
		log.Fatal(err)
	}
	srv.Options.Addr = ":8086"
	
	// Create interceptor chain
	chain := hyperserve.NewInterceptorChain()
	
	// Add CORS support
	chain.Add(NewCORSInterceptor([]string{"*"}))
	
	// Add request logging
	chain.Add(hyperserve.NewRequestLogger(log.Printf))
	
	// Add rate limiting (10 requests per second, burst of 20)
	rateLimiter := NewSimpleRateLimiter(10, 20)
	chain.Add(hyperserve.NewRateLimitInterceptor(rateLimiter))
	
	// Add API key validation
	chain.Add(NewAPIKeyValidator())
	
	// Add JSON response transformation
	chain.Add(&JSONTransformer{})
	
	// Public endpoint (no auth required)
	srv.HandleFunc("/public/health", chain.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "healthy",
			"time":   time.Now().Unix(),
		})
	})).ServeHTTP)
	
	// Protected API endpoint
	srv.HandleFunc("/api/data", chain.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user ID from request context (set by interceptor)
		userID := r.Header.Get("X-User-ID")
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": fmt.Sprintf("Hello, %s!", userID),
			"data": []map[string]interface{}{
				{"id": 1, "value": "Item 1"},
				{"id": 2, "value": "Item 2"},
				{"id": 3, "value": "Item 3"},
			},
		})
	})).ServeHTTP)
	
	// Endpoint that modifies response
	srv.HandleFunc("/api/transform", chain.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// This will be wrapped by the JSONTransformer interceptor
		json.NewEncoder(w).Encode(map[string]interface{}{
			"original": "This response will be transformed",
			"number":   42,
		})
	})).ServeHTTP)
	
	// Test page
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(testPage))
	})
	
	log.Printf("Interceptor Demo server starting on http://localhost:8086")
	log.Println("Try these endpoints:")
	log.Println("  - http://localhost:8086/ (test page)")
	log.Println("  - http://localhost:8086/public/health (no auth)")
	log.Println("  - http://localhost:8086/api/data (requires X-API-Key header)")
	log.Println("  - http://localhost:8086/api/transform (transforms JSON response)")
	log.Println("")
	log.Println("Valid API keys: demo-key-123, demo-key-456, admin-key")
	
	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

const testPage = `
<!DOCTYPE html>
<html>
<head>
    <title>Interceptor Demo</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; }
        .endpoint { background: #f9f9f9; padding: 15px; margin: 15px 0; border-radius: 5px; border-left: 4px solid #4CAF50; }
        button { background: #4CAF50; color: white; padding: 10px 20px; border: none; border-radius: 5px; cursor: pointer; margin: 5px; }
        button:hover { background: #45a049; }
        input { padding: 8px; margin: 5px; border: 1px solid #ddd; border-radius: 4px; width: 200px; }
        pre { background: #f4f4f4; padding: 10px; border-radius: 5px; overflow-x: auto; }
        .error { color: red; }
        .success { color: green; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔒 Request/Response Interceptor Demo</h1>
        
        <div class="endpoint">
            <h3>Public Health Check (No Auth)</h3>
            <button onclick="testPublicHealth()">Test Public Endpoint</button>
            <pre id="publicResult"></pre>
        </div>
        
        <div class="endpoint">
            <h3>Protected API (Requires API Key)</h3>
            <input type="text" id="apiKey" placeholder="Enter API key" value="demo-key-123">
            <button onclick="testProtectedAPI()">Test with API Key</button>
            <button onclick="testWithoutKey()">Test without Key</button>
            <pre id="protectedResult"></pre>
        </div>
        
        <div class="endpoint">
            <h3>Response Transformation</h3>
            <input type="text" id="transformKey" placeholder="Enter API key" value="demo-key-456">
            <button onclick="testTransform()">Test Transform</button>
            <pre id="transformResult"></pre>
        </div>
        
        <div class="endpoint">
            <h3>Rate Limiting Test</h3>
            <button onclick="testRateLimit()">Send 15 Rapid Requests</button>
            <pre id="rateLimitResult"></pre>
        </div>
    </div>
    
    <script>
        async function testPublicHealth() {
            try {
                const response = await fetch('/public/health');
                const data = await response.json();
                document.getElementById('publicResult').textContent = 
                    'Status: ' + response.status + '\n' +
                    JSON.stringify(data, null, 2);
            } catch (error) {
                document.getElementById('publicResult').textContent = 'Error: ' + error;
            }
        }
        
        async function testProtectedAPI() {
            const apiKey = document.getElementById('apiKey').value;
            try {
                const response = await fetch('/api/data', {
                    headers: {
                        'X-API-Key': apiKey
                    }
                });
                const data = await response.json();
                document.getElementById('protectedResult').textContent = 
                    'Status: ' + response.status + '\n' +
                    'User ID: ' + response.headers.get('X-User-ID') + '\n\n' +
                    JSON.stringify(data, null, 2);
            } catch (error) {
                document.getElementById('protectedResult').textContent = 'Error: ' + error;
            }
        }
        
        async function testWithoutKey() {
            try {
                const response = await fetch('/api/data');
                const data = await response.json();
                document.getElementById('protectedResult').textContent = 
                    'Status: ' + response.status + '\n' +
                    JSON.stringify(data, null, 2);
            } catch (error) {
                document.getElementById('protectedResult').textContent = 'Error: ' + error;
            }
        }
        
        async function testTransform() {
            const apiKey = document.getElementById('transformKey').value;
            try {
                const response = await fetch('/api/transform', {
                    headers: {
                        'X-API-Key': apiKey
                    }
                });
                const data = await response.json();
                document.getElementById('transformResult').textContent = 
                    'Status: ' + response.status + '\n' +
                    'Response transformed with metadata:\n\n' +
                    JSON.stringify(data, null, 2);
            } catch (error) {
                document.getElementById('transformResult').textContent = 'Error: ' + error;
            }
        }
        
        async function testRateLimit() {
            const results = [];
            const apiKey = 'demo-key-123';
            
            for (let i = 0; i < 15; i++) {
                try {
                    const response = await fetch('/api/data', {
                        headers: {
                            'X-API-Key': apiKey
                        }
                    });
                    results.push('Request ' + (i+1) + ': Status ' + response.status);
                } catch (error) {
                    results.push('Request ' + (i+1) + ': Error - ' + error);
                }
            }
            
            document.getElementById('rateLimitResult').textContent = results.join('\n');
        }
    </script>
</body>
</html>
`