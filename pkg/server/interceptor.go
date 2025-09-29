package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// InterceptorChain manages a chain of request/response interceptors
type InterceptorChain struct {
	interceptors []Interceptor
	mu           sync.RWMutex
}

// Interceptor defines the interface for request/response interceptors
type Interceptor interface {
	// InterceptRequest is called before the request is processed
	// It can modify the request or return an early response
	InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error)

	// InterceptResponse is called after the response is generated
	// It can modify the response before it's sent to the client
	InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error

	// Name returns the name of the interceptor for debugging
	Name() string
}

// InterceptableRequest wraps http.Request with additional functionality
type InterceptableRequest struct {
	*http.Request

	// Metadata can be used to pass data between interceptors
	Metadata map[string]interface{}

	// Body buffer for reading/modifying request body
	bodyBuffer []byte
	bodyRead   bool
	mu         sync.Mutex
}

// InterceptableResponse wraps http.ResponseWriter with buffering capability
type InterceptableResponse struct {
	http.ResponseWriter

	// Response data that can be modified
	StatusCode int
	Headers    http.Header
	Body       *bytes.Buffer

	// Metadata from the request
	Metadata map[string]interface{}

	// Track if response has been written
	written bool
	mu      sync.Mutex
}

// InterceptorResponse allows interceptors to return early responses
type InterceptorResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// NewInterceptorChain creates a new interceptor chain
func NewInterceptorChain() *InterceptorChain {
	return &InterceptorChain{
		interceptors: make([]Interceptor, 0),
	}
}

// Add adds an interceptor to the chain
func (ic *InterceptorChain) Add(interceptor Interceptor) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.interceptors = append(ic.interceptors, interceptor)
}

// Remove removes an interceptor by name
func (ic *InterceptorChain) Remove(name string) bool {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	for i, interceptor := range ic.interceptors {
		if interceptor.Name() == name {
			ic.interceptors = append(ic.interceptors[:i], ic.interceptors[i+1:]...)
			return true
		}
	}
	return false
}

// WrapHandler wraps an http.Handler with the interceptor chain
func (ic *InterceptorChain) WrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create interceptable request
		ireq := &InterceptableRequest{
			Request:  r,
			Metadata: make(map[string]interface{}),
		}

		// Create interceptable response
		iresp := &InterceptableResponse{
			ResponseWriter: w,
			StatusCode:     http.StatusOK,
			Headers:        make(http.Header),
			Body:           new(bytes.Buffer),
			Metadata:       ireq.Metadata,
		}

		// Run request interceptors
		ic.mu.RLock()
		interceptors := make([]Interceptor, len(ic.interceptors))
		copy(interceptors, ic.interceptors)
		ic.mu.RUnlock()

		for _, interceptor := range interceptors {
			resp, err := interceptor.InterceptRequest(r.Context(), ireq)
			if err != nil {
				// Interceptor error - return 500
				http.Error(w, "Interceptor error", http.StatusInternalServerError)
				return
			}

			if resp != nil {
				// Early response from interceptor
				for k, v := range resp.Headers {
					w.Header()[k] = v
				}
				w.WriteHeader(resp.StatusCode)
				w.Write(resp.Body)
				return
			}
		}

		// Create response recorder to capture the response
		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			headers:        make(http.Header),
			body:           new(bytes.Buffer),
		}

		// Call the next handler
		next.ServeHTTP(recorder, ireq.Request)

		// Copy recorded response to interceptable response
		iresp.StatusCode = recorder.statusCode
		iresp.Headers = recorder.headers
		iresp.Body = recorder.body

		// Run response interceptors in reverse order
		for i := len(interceptors) - 1; i >= 0; i-- {
			err := interceptors[i].InterceptResponse(r.Context(), ireq, iresp)
			if err != nil {
				// Log error but continue
				// In production, you might want to handle this differently
				continue
			}
		}

		// Write the final response
		for k, v := range iresp.Headers {
			w.Header()[k] = v
		}
		w.WriteHeader(iresp.StatusCode)
		io.Copy(w, iresp.Body)
	})
}

// GetBody reads and buffers the request body
func (ir *InterceptableRequest) GetBody() ([]byte, error) {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	if ir.bodyRead {
		return ir.bodyBuffer, nil
	}

	if ir.Request.Body != nil {
		body, err := io.ReadAll(ir.Request.Body)
		if err != nil {
			return nil, err
		}
		ir.Request.Body.Close()

		// Store the body and create a new reader
		ir.bodyBuffer = body
		ir.Request.Body = io.NopCloser(bytes.NewReader(body))
		ir.bodyRead = true

		return body, nil
	}

	return nil, nil
}

// SetBody sets a new request body
func (ir *InterceptableRequest) SetBody(body []byte) {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	ir.bodyBuffer = body
	ir.bodyRead = true
	ir.Request.Body = io.NopCloser(bytes.NewReader(body))
	ir.Request.ContentLength = int64(len(body))
}

// responseRecorder captures the response for modification
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	headers    http.Header
	body       *bytes.Buffer
	written    bool
}

func (rr *responseRecorder) Header() http.Header {
	if rr.headers == nil {
		rr.headers = make(http.Header)
	}
	return rr.headers
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.written {
		rr.statusCode = code
		rr.written = true

		// Copy headers to underlying writer
		for k, v := range rr.headers {
			rr.ResponseWriter.Header()[k] = v
		}
	}
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.written {
		rr.WriteHeader(http.StatusOK)
	}
	return rr.body.Write(b)
}

// Built-in Interceptors

// AuthTokenInjector adds authentication tokens to requests
type AuthTokenInjector struct {
	tokenProvider func(context.Context) (string, error)
}

func NewAuthTokenInjector(provider func(context.Context) (string, error)) *AuthTokenInjector {
	return &AuthTokenInjector{
		tokenProvider: provider,
	}
}

func (ati *AuthTokenInjector) Name() string {
	return "AuthTokenInjector"
}

func (ati *AuthTokenInjector) InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error) {
	token, err := ati.tokenProvider(ctx)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil, nil
}

func (ati *AuthTokenInjector) InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error {
	return nil
}

// RequestLogger logs all requests and responses
type RequestLogger struct {
	logger func(format string, args ...interface{})
}

func NewRequestLogger(logger func(format string, args ...interface{})) *RequestLogger {
	return &RequestLogger{
		logger: logger,
	}
}

func (rl *RequestLogger) Name() string {
	return "RequestLogger"
}

func (rl *RequestLogger) InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error) {
	rl.logger("Request: %s %s", req.Method, req.URL.Path)

	// Store request time in metadata
	req.Metadata["request_time"] = time.Now()

	return nil, nil
}

func (rl *RequestLogger) InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error {
	if startTime, ok := req.Metadata["request_time"].(time.Time); ok {
		duration := time.Since(startTime)
		rl.logger("Response: %s %s - Status: %d - Duration: %v",
			req.Method, req.URL.Path, resp.StatusCode, duration)
	}

	return nil
}

// ResponseTransformer modifies response bodies
type ResponseTransformer struct {
	transformer func([]byte, string) ([]byte, error)
}

func NewResponseTransformer(transformer func([]byte, string) ([]byte, error)) *ResponseTransformer {
	return &ResponseTransformer{
		transformer: transformer,
	}
}

func (rt *ResponseTransformer) Name() string {
	return "ResponseTransformer"
}

func (rt *ResponseTransformer) InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error) {
	return nil, nil
}

func (rt *ResponseTransformer) InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error {
	contentType := resp.Headers.Get("Content-Type")

	originalBody := resp.Body.Bytes()
	transformedBody, err := rt.transformer(originalBody, contentType)
	if err != nil {
		return err
	}

	resp.Body = bytes.NewBuffer(transformedBody)
	resp.Headers.Set("Content-Length", fmt.Sprintf("%d", len(transformedBody)))

	return nil
}

// RateLimitInterceptor enforces rate limits per client
type RateLimitInterceptor struct {
	limiter RateLimiter
}

// RateLimiter interface for rate limiting
type RateLimiter interface {
	Allow(key string) bool
}

func NewRateLimitInterceptor(limiter RateLimiter) *RateLimitInterceptor {
	return &RateLimitInterceptor{
		limiter: limiter,
	}
}

func (rli *RateLimitInterceptor) Name() string {
	return "RateLimitInterceptor"
}

func (rli *RateLimitInterceptor) InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error) {
	// Use client IP as rate limit key
	clientIP := req.RemoteAddr
	if xForwardedFor := req.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		clientIP = xForwardedFor
	}

	if !rli.limiter.Allow(clientIP) {
		return &InterceptorResponse{
			StatusCode: http.StatusTooManyRequests,
			Headers: http.Header{
				"Content-Type": []string{"text/plain"},
				"Retry-After":  []string{"60"},
			},
			Body: []byte("Rate limit exceeded"),
		}, nil
	}

	return nil, nil
}

func (rli *RateLimitInterceptor) InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error {
	return nil
}
