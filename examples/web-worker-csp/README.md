# Web Workers under Content Security Policy

HyperServe's browser-header middleware blocks `blob:` worker sources by
default. This example opts into them for an application that deliberately
creates Web Workers from blob URLs.

## Run

The assets are relative to this directory:

```sh
cd examples/web-worker-csp
go run .
```

Open <http://localhost:8080>, start the worker, and inspect the response's
`Content-Security-Policy` header.

## Configuration

```go
srv, err := server.NewServer(server.WithCSPWebWorkerSupport())
if err != nil {
    log.Fatal(err)
}

srv.Use(server.HeadersMiddleware(srv.Options()))
```

`WithCSPWebWorkerSupport` changes the configured CSP value; it does not attach
middleware by itself. `HeadersMiddleware` reads the finalized snapshot and adds
`worker-src 'self' blob:` and `child-src 'self' blob:` to responses.

Leave the option off unless the application needs blob-backed workers. It
widens the set of script execution sources allowed by the browser policy.
