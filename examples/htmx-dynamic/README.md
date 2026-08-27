# HTMX + dynamic templates

A minimal server-rendered page that returns HTML partials in response to
HTMX-triggered requests. Demonstrates:

- Loading HTML templates from `./templates` via `server.WithTemplateDir`.
- Serving static assets (the HTMX `<script>` tag) from `./static`.
- Returning a partial (not a full page) for HTMX to swap in.

The pattern keeps rendering and state on the server while a small vendored
HTMX script replaces selected page fragments.

## Run

```sh
cd examples/htmx-dynamic
go run .
```

Open <http://localhost:8080> and click the button. HTMX issues a
`GET /load-content`; the server returns a template fragment for that element.

## Files

- `main.go` — server, two routes (`/` full page, `/load-content` fragment).
- `templates/` — Go `html/template` source for both shapes.
- `static/` — `htmx.min.js` plus the small example configuration scripts.
