# Static files and an API route

This example serves a selected asset directory at `/` and keeps a normal JSON
handler at `/api/status`. HyperServe opens the asset directory through
`os.Root`; startup fails if the configured root is unavailable.

## Run

The paths are relative to this example directory:

```sh
cd examples/static-files
go run .
```

Try the page, the API, and one response header:

```sh
curl http://localhost:8080/
curl http://localhost:8080/api/status
curl -I http://localhost:8080/index.html | grep X-Content-Type-Options
```

## Server setup

```go
srv, err := server.NewServer(server.WithStaticDir("./static"))
if err != nil {
    log.Fatal(err)
}

srv.Use(server.HeadersMiddleware(srv.Options()))

if err := srv.HandleStatic("/"); err != nil {
    log.Fatal(err)
}
```

`WithStaticDir` selects the filesystem capability. `HandleStatic` decides where
that capability appears in the URL space. The middleware is a separate request
pipeline decision and therefore uses `Use` after construction.

The complete program also registers `/api/status`. Go's `ServeMux` selects that
more-specific route before the `/` static fallback.
