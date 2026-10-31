package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osauer/hyperserve/v2"
	"github.com/osauer/hyperserve/v2/ratelimit"
)

func TestReusedGateChargesOnceAcrossOverlappingHyperServePrefixes(t *testing.T) {
	app, err := hyperserve.New()
	if err != nil {
		t.Fatalf("hyperserve.New() error = %v", err)
	}
	gate, err := ratelimit.New(ratelimit.Config{
		RequestsPerSecond: 0.001,
		Burst:             1,
	})
	if err != nil {
		t.Fatalf("ratelimit.New() error = %v", err)
	}

	app.UsePrefix("/api", gate)
	app.UsePrefix("/api/admin", gate)
	app.HandleFunc("/api/admin/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := app.Handler()

	first := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	first.RemoteAddr = "192.0.2.70:1000"
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, first)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d; overlapping prefixes charged twice", firstRecorder.Code, http.StatusOK)
	}

	second := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	second.RemoteAddr = "192.0.2.70:2000"
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRecorder.Code, http.StatusTooManyRequests)
	}
}
