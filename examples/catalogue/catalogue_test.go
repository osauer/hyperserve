package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osauer/hyperserve"
)

func testCatalogue(t *testing.T) (*hyperserve.Server, *catalogue) {
	t.Helper()
	t.Setenv("HS_MCP_ENDPOINT", "/rpc")
	app, c, err := newCatalogue(Policy{MaxFileBytes: 16, MaxTotalBytes: 24, MaxDocuments: 2}, hyperserve.WithEnvironment(), hyperserve.WithMCPSupport("catalogue", "test"), hyperserve.WithLogLevel("ERROR"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return app, c
}
func call(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/octet-stream")
	r.Header.Set("X-Document-Name", "notes.txt")
	if path == "/rpc" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestCatalogueSharedHTTPAndMCPData(t *testing.T) {
	app, _ := testCatalogue(t)
	h := app.Handler()
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/documents", ""}, {"POST", "/api/documents", "hello"}, {"GET", "/api/events", ""}, {"POST", "/rpc", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`}} {
		for _, token := range []string{"", "wrong"} {
			if w := call(h, tc.method, tc.path, tc.body, token); w.Code != 401 {
				t.Fatalf("%s unauthorized: %d", tc.path, w.Code)
			}
		}
	}
	if w := call(h, "POST", "/api/documents", "hello", "catalogue-demo"); w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	w := call(h, "GET", "/api/documents", "", "catalogue-demo")
	var snapshot Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil || len(snapshot.Documents) != 1 || snapshot.Documents[0].Name != "notes.txt" {
		t.Fatalf("HTTP data: %s %v", w.Body, err)
	}
	// Discover the registered name: namespaces remain a protocol concern.
	w = call(h, "POST", "/rpc", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "catalogue-demo")
	var list struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Result.Tools) != 1 {
		t.Fatalf("tools: %s %v", w.Body, err)
	}
	body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + list.Result.Tools[0].Name + `","arguments":{"query":"notes"}}}`
	w = call(h, "POST", "/rpc", body, "catalogue-demo")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "notes.txt") {
		t.Fatalf("MCP data: %d %s", w.Code, w.Body)
	}
	if w := call(h, "GET", "/api/documents/1", "", "catalogue-demo"); w.Code != 200 || w.Body.String() != "hello" {
		t.Fatalf("download: %d %s", w.Code, w.Body)
	}
}
func TestCatalogueLimitsAreEnforced(t *testing.T) {
	app, c := testCatalogue(t)
	h := app.Handler()
	if w := call(h, "POST", "/api/documents", strings.Repeat("a", 17), "catalogue-demo"); w.Code != 413 {
		t.Fatalf("oversized body: %d", w.Code)
	}
	if c.snapshot("").Bytes != 0 {
		t.Fatal("rejected upload changed data")
	}
	for _, tc := range []struct {
		body   string
		status int
	}{{strings.Repeat("a", 16), 201}, {strings.Repeat("a", 9), 409}, {"b", 201}, {"c", 409}} {
		if w := call(h, "POST", "/api/documents", tc.body, "catalogue-demo"); w.Code != tc.status {
			t.Fatalf("limit: %d %s", w.Code, w.Body)
		}
	}
	c.upload.Lock()
	w := call(h, "POST", "/api/documents", "d", "catalogue-demo")
	c.upload.Unlock()
	if w.Code != 429 {
		t.Fatal("concurrent upload not rejected")
	}
}
func TestCatalogueStreamDeliversChangesAndCancels(t *testing.T) {
	app, c := testCatalogue(t)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer catalogue-demo")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	frames := make(chan string, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		scan := bufio.NewScanner(response.Body)
		for scan.Scan() {
			if strings.HasPrefix(scan.Text(), "data: ") {
				frames <- scan.Text()
			}
		}
	}()
	read := func() string {
		t.Helper()
		select {
		case frame := <-frames:
			return frame
		case <-time.After(2 * time.Second):
			t.Fatal("stream did not deliver snapshot")
			return ""
		}
	}
	if !strings.Contains(read(), `"revision":0`) {
		t.Fatal("initial snapshot missing")
	}
	if _, err := c.add("live.txt", []byte("live")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), "live.txt") {
		t.Fatal("updated snapshot missing")
	}
	cancel()
	response.Body.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not cancel")
	}
}
func TestCatalogueAssets(t *testing.T) {
	app, _ := testCatalogue(t)
	h := app.Handler()
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		w := call(h, "GET", path, "", "")
		b, _ := io.ReadAll(w.Result().Body)
		if w.Code != 200 || len(b) == 0 {
			t.Fatalf("asset %s: %d", path, w.Code)
		}
	}
}

func TestCatalogueRejectsInvalidFilenames(t *testing.T) {
	for _, name := range []string{"%00bad.txt", "%FF.txt", "hello%09world.txt", "../outside.txt", "..", "."} {
		t.Run(name, func(t *testing.T) {
			app, c := testCatalogue(t)
			r := httptest.NewRequest("POST", "/api/documents", strings.NewReader("hello"))
			r.Header.Set("Authorization", "Bearer catalogue-demo")
			r.Header.Set("Content-Type", "application/octet-stream")
			r.Header.Set("X-Document-Name", name)
			w := httptest.NewRecorder()
			app.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || c.snapshot("").Bytes != 0 {
				t.Fatalf("invalid filename accepted: status=%d bytes=%d", w.Code, c.snapshot("").Bytes)
			}
		})
	}
}

func TestCatalogueExecutableConfiguration(t *testing.T) {
	t.Setenv("HS_MCP_ENDPOINT", "/rpc")
	for _, address := range []string{"127.0.0.1:0", "[::1]:0", ":8080", "0.0.0.0:8080"} {
		t.Run(address, func(t *testing.T) {
			t.Setenv("SERVER_ADDR", address)
			app, err := newApp()
			local := strings.HasPrefix(address, "127.") || strings.HasPrefix(address, "[::1]")
			if !local {
				if err == nil {
					app.Shutdown(context.Background())
					t.Fatal("public-token application accepted a public listener")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer app.Shutdown(context.Background())
			var expected Policy
			if err := json.Unmarshal(policyJSON, &expected); err != nil {
				t.Fatal(err)
			}
			w := call(app.Handler(), "GET", "/api/documents", "", "catalogue-demo")
			var got Snapshot
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Policy != expected {
				t.Fatalf("executable did not apply embedded policy: %s (%v)", w.Body, err)
			}
			if w := call(app.Handler(), "POST", "/rpc", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, ""); w.Code != http.StatusUnauthorized {
				t.Fatalf("relocated endpoint lost authentication: %d", w.Code)
			}
		})
	}
}
