package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInterceptorChain(t *testing.T) {
	// Create interceptor chain
	chain := NewInterceptorChain()

	// Test adding interceptors
	logger := NewRequestLogger(t.Logf)
	chain.Add(logger)

	if len(chain.interceptors) != 1 {
		t.Errorf("Expected 1 interceptor, got %d", len(chain.interceptors))
	}

	// Test removing interceptors
	removed := chain.Remove("RequestLogger")
	if !removed {
		t.Error("Failed to remove interceptor")
	}

	if len(chain.interceptors) != 0 {
		t.Errorf("Expected 0 interceptors after removal, got %d", len(chain.interceptors))
	}
}

func TestRequestInterception(t *testing.T) {
	// Create test interceptor
	testInterceptor := &TestInterceptor{
		shouldBlock: false,
	}

	chain := NewInterceptorChain()
	chain.Add(testInterceptor)

	// Test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "success"})
	})

	// Wrap handler with interceptor chain
	wrappedHandler := chain.WrapHandler(handler)

	// Test request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	// Verify interceptor was called
	if !testInterceptor.requestCalled {
		t.Error("Request interceptor was not called")
	}

	if !testInterceptor.responseCalled {
		t.Error("Response interceptor was not called")
	}

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestEarlyResponse(t *testing.T) {
	// Create blocking interceptor
	blockingInterceptor := &TestInterceptor{
		shouldBlock: true,
	}

	chain := NewInterceptorChain()
	chain.Add(blockingInterceptor)

	// Test handler (should not be called)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when interceptor blocks")
	})

	wrappedHandler := chain.WrapHandler(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	// Verify early response
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}

	if w.Body.String() != "Blocked" {
		t.Errorf("Expected 'Blocked', got %s", w.Body.String())
	}
}

func TestRequestLogger(t *testing.T) {
	var logMessages []string
	logger := NewRequestLogger(func(format string, args ...interface{}) {
		logMessages = append(logMessages, format)
	})

	chain := NewInterceptorChain()
	chain.Add(logger)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := chain.WrapHandler(handler)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	// Verify logging
	if len(logMessages) < 2 {
		t.Errorf("Expected at least 2 log messages, got %d", len(logMessages))
	}
}

func TestInterceptableRequest(t *testing.T) {
	// Test body reading
	body := `{"test": "data"}`
	req := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte(body)))

	ireq := &InterceptableRequest{
		Request:  req,
		Metadata: make(map[string]interface{}),
	}

	// Test GetBody
	bodyData, err := ireq.GetBody()
	if err != nil {
		t.Errorf("Failed to get body: %v", err)
	}

	if string(bodyData) != body {
		t.Errorf("Expected body %s, got %s", body, string(bodyData))
	}

	// Test SetBody
	newBody := `{"new": "data"}`
	ireq.SetBody([]byte(newBody))

	bodyData, err = ireq.GetBody()
	if err != nil {
		t.Errorf("Failed to get modified body: %v", err)
	}

	if string(bodyData) != newBody {
		t.Errorf("Expected modified body %s, got %s", newBody, string(bodyData))
	}
}

// TestInterceptor for testing purposes
type TestInterceptor struct {
	requestCalled  bool
	responseCalled bool
	shouldBlock    bool
}

func (ti *TestInterceptor) Name() string {
	return "TestInterceptor"
}

func (ti *TestInterceptor) InterceptRequest(ctx context.Context, req *InterceptableRequest) (*InterceptorResponse, error) {
	ti.requestCalled = true
	req.Metadata["test"] = "value"

	if ti.shouldBlock {
		return &InterceptorResponse{
			StatusCode: http.StatusForbidden,
			Headers:    make(http.Header),
			Body:       []byte("Blocked"),
		}, nil
	}

	return nil, nil
}

func (ti *TestInterceptor) InterceptResponse(ctx context.Context, req *InterceptableRequest, resp *InterceptableResponse) error {
	ti.responseCalled = true

	// Verify metadata was passed
	if req.Metadata["test"] != "value" {
		return http.ErrAbortHandler
	}

	return nil
}

func TestAuthTokenInjector(t *testing.T) {
	tokenProvider := func(ctx context.Context) (string, error) {
		return "test-token-123", nil
	}

	injector := NewAuthTokenInjector(tokenProvider)

	req := httptest.NewRequest("GET", "/test", nil)
	ireq := &InterceptableRequest{
		Request:  req,
		Metadata: make(map[string]interface{}),
	}

	resp, err := injector.InterceptRequest(context.Background(), ireq)
	if err != nil {
		t.Errorf("Token injection failed: %v", err)
	}

	if resp != nil {
		t.Error("Token injector should not return early response")
	}

	// Verify Authorization header was added
	authHeader := ireq.Header.Get("Authorization")
	expected := "Bearer test-token-123"
	if authHeader != expected {
		t.Errorf("Expected auth header %s, got %s", expected, authHeader)
	}
}

func TestResponseTransformer(t *testing.T) {
	transformer := NewResponseTransformer(func(body []byte, contentType string) ([]byte, error) {
		// Simple transformation: add timestamp
		if contentType == "application/json" {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err == nil {
				data["transformed_at"] = time.Now().Unix()
				return json.Marshal(data)
			}
		}
		return body, nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	ireq := &InterceptableRequest{
		Request:  req,
		Metadata: make(map[string]interface{}),
	}

	// Create response
	originalBody := `{"message": "hello"}`
	resp := &InterceptableResponse{
		Headers: make(http.Header),
		Body:    bytes.NewBufferString(originalBody),
	}
	resp.Headers.Set("Content-Type", "application/json")

	err := transformer.InterceptResponse(context.Background(), ireq, resp)
	if err != nil {
		t.Errorf("Response transformation failed: %v", err)
	}

	// Verify transformation
	var transformedData map[string]interface{}
	err = json.Unmarshal(resp.Body.Bytes(), &transformedData)
	if err != nil {
		t.Errorf("Failed to parse transformed response: %v", err)
	}

	if transformedData["message"] != "hello" {
		t.Error("Original data was lost during transformation")
	}

	if transformedData["transformed_at"] == nil {
		t.Error("Transformation timestamp was not added")
	}
}
