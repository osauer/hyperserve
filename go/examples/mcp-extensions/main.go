// Example: Building a custom application with MCP extensions
//
// This example shows how to create a blog application that exposes
// its functionality through MCP tools and resources.
package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/osauer/hyperserve/go"
)

// BlogPost represents a blog post
type BlogPost struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
}

// BlogApp is our application
type BlogApp struct {
	posts sync.Map // map[string]*BlogPost
}

// CreateBlogExtension creates the MCP extension for our blog
func (app *BlogApp) CreateBlogExtension() hyperserve.MCPExtension {
	return hyperserve.NewMCPExtension("blog").
		WithDescription("Blog management tools and content access").
		WithTool(
			hyperserve.NewTool("manage_posts").
				WithDescription("Create, update, delete, or list blog posts").
				WithParameter("action", "string", "Action: create, update, delete, list, get", true).
				WithParameter("post_id", "string", "Post ID (for update, delete, get)", false).
				WithParameter("title", "string", "Post title (for create, update)", false).
				WithParameter("content", "string", "Post content (for create, update)", false).
				WithParameter("author", "string", "Post author (for create)", false).
				WithParameter("tags", "array", "Post tags (for create, update)", false).
				WithExecute(func(params map[string]interface{}) (interface{}, error) {
					action := params["action"].(string)
					
					switch action {
					case "create":
						post := &BlogPost{
							ID:        fmt.Sprintf("post-%d", time.Now().Unix()),
							Title:     params["title"].(string),
							Content:   params["content"].(string),
							Author:    params["author"].(string),
							CreatedAt: time.Now(),
						}
						if tags, ok := params["tags"].([]interface{}); ok {
							for _, tag := range tags {
								post.Tags = append(post.Tags, tag.(string))
							}
						}
						app.posts.Store(post.ID, post)
						return map[string]interface{}{
							"status": "created",
							"post":   post,
						}, nil

					case "list":
						posts := []*BlogPost{}
						app.posts.Range(func(key, value interface{}) bool {
							posts = append(posts, value.(*BlogPost))
							return true
						})
						return map[string]interface{}{
							"posts": posts,
							"count": len(posts),
						}, nil

					case "get":
						id := params["post_id"].(string)
						if val, ok := app.posts.Load(id); ok {
							return map[string]interface{}{
								"post": val.(*BlogPost),
							}, nil
						}
						return nil, fmt.Errorf("post not found: %s", id)

					default:
						return nil, fmt.Errorf("unknown action: %s", action)
					}
				}).
				Build(),
		).
		WithTool(
			hyperserve.NewTool("search_posts").
				WithDescription("Search blog posts by keyword or tag").
				WithParameter("query", "string", "Search query", false).
				WithParameter("tag", "string", "Filter by tag", false).
				WithExecute(func(params map[string]interface{}) (interface{}, error) {
					results := []*BlogPost{}
					query, _ := params["query"].(string)
					tag, _ := params["tag"].(string)

					app.posts.Range(func(key, value interface{}) bool {
						post := value.(*BlogPost)
						match := false

						// Search in title and content
						if query != "" {
							if contains(post.Title, query) || contains(post.Content, query) {
								match = true
							}
						}

						// Filter by tag
						if tag != "" {
							for _, t := range post.Tags {
								if t == tag {
									match = true
									break
								}
							}
						}

						if match || (query == "" && tag == "") {
							results = append(results, post)
						}
						return true
					})

					return map[string]interface{}{
						"results": results,
						"count":   len(results),
					}, nil
				}).
				Build(),
		).
		WithResource(
			hyperserve.NewResource("blog://posts/recent").
				WithName("Recent Posts").
				WithDescription("Latest blog posts").
				WithRead(func() (interface{}, error) {
					posts := []*BlogPost{}
					app.posts.Range(func(key, value interface{}) bool {
						posts = append(posts, value.(*BlogPost))
						return len(posts) < 10 // Limit to 10 recent posts
					})
					return map[string]interface{}{
						"posts": posts,
					}, nil
				}).
				Build(),
		).
		WithResource(
			hyperserve.NewResource("blog://stats/overview").
				WithName("Blog Statistics").
				WithDescription("Overview of blog statistics").
				WithRead(func() (interface{}, error) {
					count := 0
					authors := map[string]int{}
					tags := map[string]int{}

					app.posts.Range(func(key, value interface{}) bool {
						post := value.(*BlogPost)
						count++
						authors[post.Author]++
						for _, tag := range post.Tags {
							tags[tag]++
						}
						return true
					})

					return map[string]interface{}{
						"total_posts": count,
						"authors":     authors,
						"tags":        tags,
						"updated_at":  time.Now(),
					}, nil
				}).
				Build(),
		).
		Build()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

func main() {
	// Create our blog application
	app := &BlogApp{}

	// Add some sample posts
	samplePosts := []*BlogPost{
		{
			ID:        "post-1",
			Title:     "Getting Started with HyperServe",
			Content:   "HyperServe is a lightweight HTTP server framework...",
			Author:    "Alice",
			CreatedAt: time.Now().Add(-24 * time.Hour),
			Tags:      []string{"tutorial", "golang", "web"},
		},
		{
			ID:        "post-2",
			Title:     "Building MCP Extensions",
			Content:   "Learn how to extend your application with MCP tools...",
			Author:    "Bob",
			CreatedAt: time.Now().Add(-12 * time.Hour),
			Tags:      []string{"mcp", "ai", "development"},
		},
	}
	for _, post := range samplePosts {
		app.posts.Store(post.ID, post)
	}

	// Create server
	srv, err := hyperserve.NewServer(
		hyperserve.WithMCPSupport("BlogApp", "1.0.0"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Register our blog extension
	if err := srv.RegisterMCPExtension(app.CreateBlogExtension()); err != nil {
		log.Fatal("Failed to register blog extension:", err)
	}

	// Add HTTP endpoints for the blog
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Blog Application with MCP Support")
	})

	srv.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		posts := []*BlogPost{}
		app.posts.Range(func(key, value interface{}) bool {
			posts = append(posts, value.(*BlogPost))
			return true
		})
		// In real app, you'd use json.Marshal
		fmt.Fprintf(w, `{"posts": %d}`, len(posts))
	})

	log.Println("Starting Blog Application with MCP extensions")
	log.Println("Available MCP tools:")
	log.Println("  - manage_posts: Create and manage blog posts")
	log.Println("  - search_posts: Search posts by keyword or tag")
	log.Println("")
	log.Println("Available MCP resources:")
	log.Println("  - blog://posts/recent: Recent blog posts")
	log.Println("  - blog://stats/overview: Blog statistics")
	log.Println("")
	log.Println("Example usage with Claude:")
	log.Println("  'Create a new blog post about Go concurrency'")
	log.Println("  'Show me all posts tagged with \"golang\"'")
	log.Println("  'What are the blog statistics?'")

	srv.Run()
}