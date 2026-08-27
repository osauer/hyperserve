# Hello world

This is the smallest complete HyperServe application. It includes orderly
Ctrl+C shutdown rather than hiding process-signal ownership in the library.

It shows four pieces and nothing else:

- Creating a basic HyperServe server
- Handling HTTP requests with a simple handler function
- Returning a text response
- Connecting server lifetime to application cancellation

## Run it

```sh
go run ./examples/hello-world
```

From another terminal:

```bash
curl http://localhost:8080/
# Output: Hello, World from HyperServe!
```

The server listens on `http://localhost:8080`.

## Lifecycle shape

```go
// This executable owns Ctrl+C. HyperServe follows ctx and cleans up the
// server resources it started.
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

srv, err := server.NewServer()
if err != nil {
	log.Fatal(err)
}

srv.HandleFunc("/", hello)

if err := srv.Run(ctx); err != nil {
	log.Fatal(err)
}
```

The context lines are ordinary Go lifecycle plumbing. `signal.NotifyContext`
creates a child of `context.Background()` and cancels it when Ctrl+C arrives.
The deferred `stop` releases the signal registration when `main` returns.

Here `context.Background()` is the root because this small program owns the
whole process. If a larger application already gives you a context, pass it to
`srv.Run` or use it as the parent of `signal.NotifyContext`. This is the
server's lifetime context; handlers use `r.Context()` for each request.

Next, read [middleware basics](../middleware-basics/) to see how request policy
wraps the same standard handler shape.
