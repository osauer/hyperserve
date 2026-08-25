# Hello World Example

This is the smallest complete HyperServe application. It includes orderly
Ctrl+C shutdown rather than hiding process-signal ownership in the library.

## What This Example Shows

- Creating a basic HyperServe server
- Handling HTTP requests with a simple handler function
- Returning a text response
- Connecting server lifetime to application cancellation

## Running the Example

```bash
go run main.go
```

The server will start on http://localhost:8080

## Testing the Server

In another terminal, use curl to test:

```bash
curl http://localhost:8080/
# Output: Hello, World from HyperServe!
```

Or open http://localhost:8080 in your web browser.

## Code Breakdown

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

## What's Next?

- Try changing the response message
- Add another route like `/about`
- Move on to [static-files](../static-files/) to serve HTML files
