# Document catalogue

One in-memory collection is exposed through HTML, a JSON API, an MCP search
tool, and authenticated SSE. The application owns data, identity and budgets;
HyperServe owns the HTTP boundary and transport lifecycle. No new dependencies.

Run from the repository root:

```sh
go run ./examples/catalogue
```

[policy.json](policy.json) is embedded at build time and is the only source of
storage limits: 10 MiB per file, 50 MiB of file content in total, 100 documents.
Oliver approved these limits and memory-only storage on 2026-10-31 07:51 CET.
The UI reads this policy from the server; no browser or library defaults copy it.
Stored file content and the single admitted upload are bounded separately;
HTTP buffers and filename metadata also consume memory.

Open http://127.0.0.1:8080. Upload a sample document or choose **Add a sample
note**, search its filename, and open a second view to see live changes.
The JSON/MCP inspector runs real read requests using the current search; its
terminal commands include the public token and discovered MCP headers. Click
a filename to download it. Storage is process-local and clears
on restart. This teaching app uses the public token `catalogue-demo`; keep it
on loopback. Startup rejects a non-loopback address. A deployed application supplies a real verifier and access policy.

## Configuration and lifetime

The application explicitly reads HyperServe environment options. For example,
`SERVER_ADDR=127.0.0.1:8081 HS_MCP_ENDPOINT=/rpc go run ./examples/catalogue`
changes the listener and MCP endpoint while retaining authentication. Use a
numeric loopback address, not the all-interface `HS_PORT` shortcut.

Health runs separately on `127.0.0.1:9080`: `/healthz`, `/livez`, `/readyz`.
The application owns signals and the root context. Stream writes carry their
own deadlines, so the HTTP server has no whole-response write timeout.
Cancelling the application context stops both listeners and active streams.

Templates and rate limiting are covered by [HTMX](../htmx-dynamic/) and
[middleware basics](../middleware-basics/); this app does not need an extra
renderer or a second admission policy. It replaces `complete` and
`best-practices`, including their hardcoded feature cards, simulated CPU/Memory
chart, random-error action and duplicate About page.

## Shared data

```sh
curl -H 'Authorization: Bearer catalogue-demo' \
  http://127.0.0.1:8080/api/documents
curl -H 'Authorization: Bearer catalogue-demo' \
  -H 'Content-Type: application/octet-stream' -H 'X-Document-Name: notes.txt' \
  --data-binary @notes.txt http://127.0.0.1:8080/api/documents
```

`X-Document-Name` uses percent-encoded UTF-8 for non-ASCII filenames. Downloads
are attachments. File bytes never become HTML, filenames are inserted as text,
and uploaded files are not executed or persisted. Uploads are serialized by
admission: a concurrent upload receives 429 instead of waiting and retaining
another body. The store enforces file, total-byte and document-count limits.

Use `tools/list` on the configured MCP endpoint to discover `search_documents`,
then call it with `{"query":"notes"}`. The tool reads the same collection as
`GET /api/documents?q=notes`. Authentication follows `app.Options().MCPEndpoint`,
including an endpoint relocated by `HS_MCP_ENDPOINT`.

The browser uses authenticated fetch streaming. Each SSE frame is a complete
catalogue snapshot; reconnect does not require replay or start application work.
Writes have a five-second transport deadline. The application context owns the
server lifetime; a browser disconnect owns only that reader's stream.

## Tests

`go test ./examples/catalogue` from the repository root exercises authentication,
HTTP/MCP data agreement, upload limits, admission, downloads, asset delivery,
live updates and cancellation using synthetic policies and data.
