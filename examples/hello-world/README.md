# Hello World Example

This is the smallest complete HyperServe application. It includes orderly
Ctrl+C shutdown rather than hiding process-signal ownership in the library.

## What This Example Shows

- Creating a basic HyperServe server
- Handling HTTP requests with a simple handler function
- Returning a text response

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
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

srv, err := server.NewServer()
if err != nil {
	log.Fatal(err)
}

// Register a handler for the root path
srv.HandleFunc("/", handler)

// Run blocks until Ctrl+C cancels ctx.
if err := srv.Run(ctx); err != nil {
	log.Fatal(err)
}
```

The extra context lines are ordinary Go lifecycle plumbing. They make it clear
that the application owns process signals while HyperServe owns cleanup of the
listeners and workers it starts.

## What's Next?

- Try changing the response message
- Add another route like `/about`
- Move on to [static-files](../static-files/) to serve HTML files
