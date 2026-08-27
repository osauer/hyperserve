package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareScopes(t *testing.T) {
	srv, err := newServer()
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	tests := []struct {
		path          string
		wantStatus    int
		wantRateLimit bool
	}{
		{path: "/", wantStatus: http.StatusOK},
		{path: "/api/data", wantStatus: http.StatusOK, wantRateLimit: true},
		{path: "/api2", wantStatus: http.StatusOK},
		{path: "/api/crash", wantStatus: http.StatusInternalServerError, wantRateLimit: true},
	}

	handler := srv.Handler()
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
			gotRateLimit := recorder.Header().Get("X-RateLimit-Limit") != ""
			if gotRateLimit != test.wantRateLimit {
				t.Fatalf("rate-limit header present = %t, want %t", gotRateLimit, test.wantRateLimit)
			}
		})
	}
}
