# HTMX + dynamic templates

A minimal server-rendered page that returns HTML partials in response to
HTMX-triggered requests. Demonstrates:

- Loading HTML templates from `./templates` via `server.WithTemplateDir`.
- Serving static assets (the HTMX `<script>` tag) from `./static`.
- Returning a partial (not a full page) for HTMX to swap in.

The pattern is for teams that want HTMX-style interactivity without a
JavaScript bundler — server-rendered HTML, server-owned state, plus the
small `htmx.js` script loaded once.

## Run

```bash
go run ./examples/htmx-dynamic &
open http://localhost:8080
```

Click the button; HTMX issues a `GET /load-content`; the server returns a
template fragment; HTMX swaps it into the DOM. No SPA, no bundler.

## Files

- `main.go` — server, two routes (`/` full page, `/load-content` fragment).
- `templates/` — Go `html/template` source for both shapes.
- `static/` — `htmx.js` (vendored so the example runs offline).
