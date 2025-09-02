# ADR-0007: Optional Template System Integration

**Status:** Accepted  
**Date:** 2024-12-01  
**Deciders:** hyperserve team  

## Context

Many web applications need HTML templating for dynamic content:
- Server-side rendered pages
- Email templates
- Dynamic content generation

However, not all HTTP servers need templating:
- APIs return JSON, not HTML
- Static file servers don't need templates
- Microservices often skip HTML entirely

The challenge is supporting templates without forcing them on users who don't need them.

## Decision

Provide optional template integration using Go's `html/template`:
- Templates are discovered automatically from a configured directory
- No templates are loaded if directory isn't specified
- Zero overhead when templates aren't used
- Use standard Go template syntax

Templates are configured via:
```go
hyperserve.WithTemplateDir("./templates")
```

## Consequences

### Positive
- **Zero overhead**: No performance impact when not used
- **Familiar syntax**: Standard Go templates
- **Auto-discovery**: Templates found automatically
- **Security**: html/template provides XSS protection
- **Simplicity**: No custom template language to learn

### Negative
- **No hot reload**: Must restart server for template changes
- **Limited syntax**: Only Go template features available
- **No template inheritance**: Unlike Jinja2 or similar
- **Performance**: Template parsing happens at startup

### Mitigation
- Clear documentation of template features
- Examples of common patterns
- Recommend template development workflow
- Consider hot reload in development mode (future)

## Implementation Details

- Templates are parsed recursively from the specified directory
- Template names are relative paths from template directory
- Templates are parsed once at server startup
- Template execution errors return 500 status

```go
// Internal implementation
if opts.TemplateDir != "" {
    templates, err = template.ParseGlob(
        filepath.Join(opts.TemplateDir, "**/*.html")
    )
}

// Handler usage
srv.TemplateHandler("/", "index.html", dataFunc)
```

## Examples

Directory structure:
```
templates/
├── index.html
├── layouts/
│   └── base.html
└── partials/
    ├── header.html
    └── footer.html
```

Server configuration:
```go
srv, _ := hyperserve.NewServer(
    hyperserve.WithTemplateDir("./templates"),
)

// Render template with data
srv.TemplateHandler("/", "index.html", func(r *http.Request) any {
    return map[string]any{
        "Title": "Welcome",
        "User":  getCurrentUser(r),
    }
})

// Or use in custom handler
srv.HandleFunc("/profile", func(w http.ResponseWriter, r *http.Request) {
    data := getUserProfile(r)
    srv.RenderTemplate(w, "profile.html", data)
})
```

Template example:
```html
{{/* index.html */}}
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
</head>
<body>
    <h1>Welcome {{.User.Name}}</h1>
</body>
</html>
```