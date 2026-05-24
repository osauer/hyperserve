// Example: typed MCP tools — one tool per verb.
//
// Each tool is a thin wrapper around a method on the blog store. The args
// struct says exactly what the tool needs (no "optional unless action=…"
// gymnastics), the return type is a concrete domain object that gets
// JSON-marshaled and advertised via `outputSchema` on tools/list, and
// every handler reads like normal Go — no map[string]any assertions.
//
// One tool is kept on the older builder API (search_posts) so the two
// shapes stay visible side by side. Use the typed shape for new tools and
// the builder when you need to hand-tune a schema the typed generator
// doesn't emit yet.
//
// The example also registers a subscribable resource template:
// blog://posts/{id}. `resources/templates/list` advertises the family,
// `resources/read` resolves concrete post IDs, and SSE/stdio clients can
// subscribe for standard `notifications/resources/updated` invalidations.
//
// Try it:
//
//	go run ./examples/mcp-extensions &
//
//	# tools/list now carries inputSchema AND outputSchema for typed tools.
//	curl -s -X POST localhost:8080/mcp -H 'Content-Type: application/json' \
//	     -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | jq .
//
//	curl -s -X POST localhost:8080/mcp -H 'Content-Type: application/json' \
//	     -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{
//	            "name":"create_post",
//	            "arguments":{"title":"hello","author":"ada","content":"first","tags":["intro"]}
//	          }}'
//
//	# Validation: required field missing surfaces through the MCP error envelope.
//	curl -s -X POST localhost:8080/mcp -H 'Content-Type: application/json' \
//	     -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{
//	            "name":"create_post","arguments":{"title":"hello"}
//	          }}'
package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/osauer/hyperserve/pkg/mcp"
	"github.com/osauer/hyperserve/pkg/server"
)

// Post is the domain object every tool works with.
type Post struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

type blog struct {
	mu    sync.Mutex
	posts map[string]*Post
}

func newBlog() *blog { return &blog{posts: map[string]*Post{}} }

// =============================================================================
// Typed tools — one verb each
// =============================================================================
//
// The args struct + the return type drive both `inputSchema` and
// `outputSchema` on the wire. Validation runs before the handler does.

type CreatePostArgs struct {
	Title   string   `json:"title"   validate:"required,max=200" mcp:"desc=Post title"`
	Author  string   `json:"author"  validate:"required"         mcp:"desc=Author handle"`
	Content string   `json:"content"                             mcp:"desc=Post body"`
	Tags    []string `json:"tags,omitempty" validate:"max=10"    mcp:"desc=Tags (max 10)"`
}

func (b *blog) Create(_ context.Context, args CreatePostArgs) (Post, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := Post{
		ID:        fmt.Sprintf("post-%d", time.Now().UnixNano()),
		Title:     args.Title,
		Author:    args.Author,
		Content:   args.Content,
		Tags:      append([]string(nil), args.Tags...),
		CreatedAt: time.Now().UTC(),
	}
	b.posts[p.ID] = &p
	return p, nil
}

type GetPostArgs struct {
	ID string `json:"id" validate:"required" mcp:"desc=Post identifier"`
}

func (b *blog) Get(_ context.Context, args GetPostArgs) (Post, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.posts[args.ID]
	if !ok {
		return Post{}, mcp.ToolErrorf("post %q not found", args.ID)
	}
	return *p, nil
}

// List takes no arguments. The args type is an empty struct.
func (b *blog) List(_ context.Context, _ struct{}) ([]Post, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Post, 0, len(b.posts))
	for _, p := range b.posts {
		out = append(out, *p)
	}
	return out, nil
}

type DeletePostArgs struct {
	ID string `json:"id" validate:"required" mcp:"desc=Post identifier"`
}

// Delete returns no payload. `struct{}` on the return side suppresses
// outputSchema and makes the wire response a JSON-encoded empty object.
func (b *blog) Delete(_ context.Context, args DeletePostArgs) (struct{}, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.posts[args.ID]; !ok {
		return struct{}{}, mcp.ToolErrorf("post %q not found", args.ID)
	}
	delete(b.posts, args.ID)
	return struct{}{}, nil
}

// =============================================================================
// Builder tool — kept for contrast
// =============================================================================
//
// search_posts uses the older builder API. The handler reads params out of
// a map[string]any with unchecked assertions. Use this shape when you want
// hand-tuned schemas (e.g. unions the typed generator doesn't emit yet).

func (b *blog) searchTool() mcp.Tool {
	return mcp.NewTool("search_posts").
		WithDescription("Search posts by query string and/or tag.").
		WithParameter("query", "string", "Substring matched against title and content", false).
		WithParameter("tag", "string", "Exact tag filter", false).
		WithExecute(func(params map[string]any) (any, error) {
			query, _ := params["query"].(string)
			tag, _ := params["tag"].(string)
			query = strings.ToLower(query)

			b.mu.Lock()
			defer b.mu.Unlock()
			results := make([]Post, 0, len(b.posts))
			for _, p := range b.posts {
				if query != "" {
					hay := strings.ToLower(p.Title) + " " + strings.ToLower(p.Content)
					if !strings.Contains(hay, query) {
						continue
					}
				}
				if tag != "" && !slices.Contains(p.Tags, tag) {
					continue
				}
				results = append(results, *p)
			}
			return map[string]any{"count": len(results), "posts": results}, nil
		}).
		Build()
}

// =============================================================================
// Resource template — concrete post URIs + live invalidation notifications
// =============================================================================

type postResourceTemplate struct {
	store *blog
}

func (t postResourceTemplate) URITemplate() string { return "blog://posts/{id}" }
func (t postResourceTemplate) Name() string        { return "Blog Post" }
func (t postResourceTemplate) Description() string { return "Read one blog post by ID." }
func (t postResourceTemplate) MimeType() string    { return "application/json" }

func (t postResourceTemplate) Match(uri string) (map[string]string, bool) {
	id, ok := strings.CutPrefix(uri, "blog://posts/")
	if !ok || id == "" || strings.Contains(id, "/") {
		return nil, false
	}
	return map[string]string{"id": id}, true
}

func (t postResourceTemplate) Read(ctx context.Context, _ string, params map[string]string) (any, error) {
	return t.store.Get(ctx, GetPostArgs{ID: params["id"]})
}

func (t postResourceTemplate) Subscribe(ctx context.Context, uri string, _ map[string]string, emit mcp.ResourceEmitter) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := emit.Update(uri); err != nil {
				return err
			}
		}
	}
}

// =============================================================================
// Wiring
// =============================================================================

func main() {
	srv, err := server.NewServer(
		server.WithAddr(":8080"),
		server.WithMCPSupport("blog-mcp", "0.1.0"),
	)
	if err != nil {
		log.Fatal(err)
	}

	store := newBlog()

	ext := mcp.NewExtension("blog").
		WithDescription("Blog tools — typed verbs + one builder tool for contrast.").
		WithTool(mcp.NewTypedTool("create_post", "Create a new blog post.", store.Create)).
		WithTool(mcp.NewTypedTool("get_post", "Fetch a single post by ID.", store.Get)).
		WithTool(mcp.NewTypedTool("list_posts", "List every post in the store.", store.List)).
		WithTool(mcp.NewTypedTool("delete_post", "Delete a post by ID.", store.Delete)).
		WithTool(store.searchTool()).
		WithResourceTemplate(postResourceTemplate{store: store}).
		Build()

	if err := srv.RegisterMCPExtension(ext); err != nil {
		log.Fatal(err)
	}

	log.Println("MCP server on :8080, endpoint /mcp")
	log.Fatal(srv.Run())
}
