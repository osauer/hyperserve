package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osauer/hyperserve/v2/ratelimit"
)

func TestMiddlewareScopes(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}

	tests := []struct {
		path       string
		wantStatus int
	}{
		{path: "/", wantStatus: http.StatusOK},
		{path: "/api/data", wantStatus: http.StatusOK},
		{path: "/api2", wantStatus: http.StatusOK},
		{path: "/api/crash", wantStatus: http.StatusInternalServerError},
	}

	handler := app.Handler()
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if got := recorder.Header().Get("X-Example-Middleware"); got != "active" {
				t.Fatalf("X-Example-Middleware = %q, want active", got)
			}
			if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestAPIRateLimitIsPrefixScoped(t *testing.T) {
	app, err := newAppWithAPIGate(ratelimit.Config{
		RequestsPerSecond: 0.000001,
		Burst:             10,
	})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	handler := app.Handler()

	for i := 0; i < 10; i++ {
		request := httptest.NewRequest(http.MethodGet, "/api/data", nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("API request %d status = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("API request over burst status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limit rejection has no Retry-After header")
	}

	publicRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	publicRecorder := httptest.NewRecorder()
	handler.ServeHTTP(publicRecorder, publicRequest)
	if publicRecorder.Code != http.StatusOK {
		t.Fatalf("public route status = %d, want %d", publicRecorder.Code, http.StatusOK)
	}
}
