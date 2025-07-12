# Web Worker CSP Example

This example demonstrates how to enable Web Worker support with Content Security Policy (CSP) in HyperServe.

## What this example shows

- How to enable Web Worker support using `WithCSPWebWorkerSupport()`
- How to create Web Workers using `blob:` URLs 
- How CSP headers are configured to allow Web Workers
- Simulation of real-world usage (like Tone.js audio libraries)

## The Problem

Many modern web applications use Web Workers for performance optimization. Libraries like:
- **Tone.js** - Web Audio API library for audio synthesis
- **PDF.js** - PDF rendering library
- **Custom audio/video processing libraries**

These libraries create Web Workers using `blob:` URLs, which are blocked by default by Content Security Policy (CSP) for security reasons.

## The Solution

HyperServe provides `WithCSPWebWorkerSupport()` to enable `blob:` URLs in the CSP `worker-src` and `child-src` directives.

## Running the Example

```bash
cd examples/web-worker-csp
go run main.go
```

Visit: http://localhost:8080

## What to Test

1. **Test Web Worker** - Creates a simple Web Worker using blob: URL
2. **Test Tone.js (Simulated)** - Simulates how Tone.js creates timing workers
3. **Show CSP Headers** - Displays the actual CSP headers sent by the server

## Key Code

```go
// Enable Web Worker support
srv, err := hyperserve.NewServer(
    hyperserve.WithCSPWebWorkerSupport(),
)

// Apply security headers with Web Worker support
srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))
```

## CSP Configuration

When Web Worker support is enabled, the CSP header includes:
- `worker-src 'self' blob:`
- `child-src 'self' blob:`

When disabled (default), Web Workers with blob: URLs will be blocked.

## Environment Configuration

You can also enable this via environment variable:

```bash
export HS_CSP_WEB_WORKER_SUPPORT=true
go run main.go
```

Or via JSON configuration file:

```json
{
    "csp_web_worker_support": true
}
```

## Security Note

Web Worker support is **disabled by default** for security reasons. Only enable it when your application specifically needs to use Web Workers with blob: URLs.

## Real-World Usage

This feature is essential for:
- Digital Audio Workstations (DAWs) using Tone.js
- PDF viewers using PDF.js
- Video/audio processing applications
- Any application using Web Workers for performance optimization