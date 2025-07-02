# Hello World Example

This is the simplest possible HyperServe application. It demonstrates the absolute minimum code needed to create a working HTTP server.

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
// Create a server with default settings
server, err := hyperserve.NewServer()

// Register a handler for the root path
server.HandleFunc("/", handler)

// Start the server (blocks until stopped)
server.Run()
```

That's it! Just three function calls to get a working server.

## What's Next?

- Try changing the response message
- Add another route like `/about`
- Move on to [static-files](../static-files/) to serve HTML files