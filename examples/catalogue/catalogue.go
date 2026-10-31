package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/osauer/hyperserve"
	"github.com/osauer/hyperserve/auth"
	"github.com/osauer/hyperserve/mcp"
	"github.com/osauer/hyperserve/sse"
)

//go:embed web/*
var assets embed.FS

// Policy belongs to this example application, not HyperServe. Required fields
// deliberately have no source defaults: deployment chooses finite budgets.
type Policy struct {
	MaxFileBytes  int64 `json:"max_file_bytes"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
	MaxDocuments  int   `json:"max_documents"`
}

func (p Policy) validate() error {
	if p.MaxFileBytes <= 0 || p.MaxTotalBytes < p.MaxFileBytes || p.MaxDocuments <= 0 {
		return errors.New("policy requires positive file/document limits and total bytes >= file bytes")
	}
	return nil
}

type Document struct {
	ID      int       `json:"id"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	content []byte
}
type Snapshot struct {
	Documents []Document `json:"documents"`
	Bytes     int64      `json:"bytes"`
	Revision  int        `json:"revision"`
	Policy    Policy     `json:"policy"`
}
type catalogue struct {
	mu        sync.Mutex
	policy    Policy
	documents []Document
	bytes     int64
	revision  int
	changed   chan struct{}
	// One upload at a time bounds in-flight allocations as well as stored data.
	upload sync.Mutex
}

func (c *catalogue) snapshot(query string) Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := Snapshot{Documents: []Document{}, Bytes: c.bytes, Revision: c.revision, Policy: c.policy}
	for _, d := range c.documents {
		if strings.Contains(strings.ToLower(d.Name), strings.ToLower(query)) {
			d.content = nil
			result.Documents = append(result.Documents, d)
		}
	}
	return result
}
func (c *catalogue) add(name string, content []byte) (Document, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if int64(len(content)) > c.policy.MaxFileBytes || c.bytes+int64(len(content)) > c.policy.MaxTotalBytes || len(c.documents) >= c.policy.MaxDocuments {
		return Document{}, errors.New("catalogue capacity reached")
	}
	c.revision++
	d := Document{ID: c.revision, Name: name, Size: int64(len(content)), Created: time.Now().UTC(), content: content}
	c.documents = append(c.documents, d)
	c.bytes += d.Size
	close(c.changed)
	c.changed = make(chan struct{})
	d.content = nil
	return d, nil
}
func newCatalogue(policy Policy, options ...hyperserve.Option) (*hyperserve.Server, *catalogue, error) {
	if err := policy.validate(); err != nil {
		return nil, nil, err
	}
	app, err := hyperserve.New(options...)
	if err != nil {
		return nil, nil, err
	}
	c := &catalogue{policy: policy, changed: make(chan struct{})}
	app.Use(hyperserve.HeadersMiddleware(app.Options()))
	identity := auth.Require(auth.Bearer(auth.TokenVerifierFunc(func(_ context.Context, token string) (auth.Principal, error) {
		// Public fixture, suitable only for a loopback teaching application.
		if token != "catalogue-demo" {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return auth.Principal{Issuer: "catalogue-example", Subject: "reader"}, nil
	})))
	app.UsePrefix("/api", identity)
	if app.MCPEnabled() {
		app.UsePrefix(app.Options().MCPEndpoint, identity)
		tool := mcp.NewTypedTool("search_documents", "Search the catalogue by filename.", func(_ context.Context, in struct {
			Query string `json:"query"`
		}) (Snapshot, error) {
			return c.snapshot(in.Query), nil
		})
		if err := app.RegisterMCPTool(tool); err != nil {
			app.Shutdown(context.Background())
			return nil, nil, err
		}
	}
	app.GET("/api/documents", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, c.snapshot(r.URL.Query().Get("q"))) })
	app.POST("/api/documents", c.receive)
	app.GET("/api/documents/{id}", c.download)
	app.GET("/api/events", c.events)
	web, err := fs.Sub(assets, "web")
	if err != nil {
		app.Shutdown(context.Background())
		return nil, nil, err
	}
	app.GET("/", http.FileServerFS(web).ServeHTTP)
	return app, c, nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (c *catalogue) receive(w http.ResponseWriter, r *http.Request) {
	// Reject concurrent uploads instead of retaining unbounded waiting requests.
	if !c.upload.TryLock() {
		http.Error(w, "Another upload is in progress", http.StatusTooManyRequests)
		return
	}
	defer c.upload.Unlock()
	if !strings.EqualFold(r.Header.Get("Content-Type"), "application/octet-stream") {
		http.Error(w, "Send the file as application/octet-stream", http.StatusUnsupportedMediaType)
		return
	}
	name, decodeErr := url.PathUnescape(r.Header.Get("X-Document-Name"))
	name = strings.TrimSpace(name)
	if decodeErr != nil {
		http.Error(w, "Invalid filename encoding", http.StatusBadRequest)
		return
	}
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) ||
		name != filepath.Base(name) || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) {
		http.Error(w, "A plain filename is required", http.StatusBadRequest)
		return
	}
	// A raw body makes the request limit equal the file limit: no multipart overhead.
	r.Body = http.MaxBytesReader(w, r.Body, c.policy.MaxFileBytes)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "File exceeds the catalogue limit", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Could not read file", http.StatusBadRequest)
		}
		return
	}
	d, err := c.add(name, content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, 201, d)
}
func (c *catalogue) download(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	i := slices.IndexFunc(c.documents, func(d Document) bool { return fmt.Sprint(d.ID) == r.PathValue("id") })
	if i < 0 {
		c.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	d := c.documents[i]
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Name}))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(d.content)
}
func (c *catalogue) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	controller := http.NewResponseController(w)
	defer func() { _ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second)) }()
	stream, err := sse.NewWriter(w, 5*time.Second)
	if err != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		c.mu.Lock()
		changed := c.changed
		c.mu.Unlock()
		if stream.SendJSON("catalogue", "", c.snapshot("")) != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		case <-ticker.C:
		}
	}
}
