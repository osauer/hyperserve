# Static Files Example

This example shows how to serve static files (HTML, CSS, JavaScript) with HyperServe. It demonstrates a common use case of serving a website with both static content and API endpoints.

## What This Example Shows

- Configuring HyperServe to serve static files
- Proper directory structure for web assets
- Adding security headers for static content
- Mixing static file serving with custom API routes
- Client-side JavaScript interacting with server endpoints

## Directory Structure

```
02-static-files/
├── main.go          # Server code
├── static/          # Static files directory (default)
│   ├── index.html   # Homepage
│   ├── about.html   # About page
│   ├── css/
│   │   └── style.css
│   └── js/
│       └── app.js
```

## Running the Example

```bash
go run main.go
```

The server will start on http://localhost:8080

## Testing the Server

### Browse the Website

Open http://localhost:8080 in your browser. You'll see:
- The homepage (index.html)
- Styled with CSS
- Interactive JavaScript that calls the API

### Test with Curl

```bash
# Get the homepage
curl http://localhost:8080/

# Get a specific file
curl http://localhost:8080/about.html

# Get CSS file
curl http://localhost:8080/css/style.css

# Call the API endpoint
curl http://localhost:8080/api/status
```

### Check Security Headers

```bash
curl -I http://localhost:8080/index.html
```

You'll see security headers like:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`

## Key Concepts

### 1. Static File Serving

```go
// Serve static files from the default "static/" directory
server.HandleStatic("/")
```

HyperServe automatically serves files from the `static/` directory by default.

### 2. Automatic index.html

When you request `/`, HyperServe automatically serves `/index.html` if it exists.

### 3. Security Headers

```go
server.AddMiddleware("*", hyperserve.HeadersMiddleware(server.Options))
```

This middleware adds important security headers to all responses.

### 4. Mixed Routes

You can have both static files and custom API endpoints:
- `/` → serves static files
- `/api/status` → custom handler

## Try These Modifications

1. **Add a new page**: Create `static/contact.html`
2. **Add images**: Put images in `static/images/` and reference them in HTML
3. **Custom 404**: Add a custom 404 handler for missing files
4. **File upload**: Add an endpoint to handle file uploads

## Common Patterns

### Serving from Different Directories

```go
// Create server with custom static directory
server, err := hyperserve.NewServer(
    hyperserve.WithStaticDir("./public"),
)
```

### Adding Cache Headers

```go
server.AddMiddleware("*", func(next http.Handler) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Cache static assets for 1 hour
        if strings.HasPrefix(r.URL.Path, "/css/") || 
           strings.HasPrefix(r.URL.Path, "/js/") {
            w.Header().Set("Cache-Control", "public, max-age=3600")
        }
        next.ServeHTTP(w, r)
    }
})
```

## What's Next?

Now that you can serve static files, move on to [json-api](../json-api/) to learn about building REST APIs with JSON.