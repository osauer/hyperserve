package main

import (
	"log"
	"net/http"

	"github.com/osauer/hyperserve"
)

func main() {
	// Create a new server with CSP configured to allow blob: URLs for Web Workers
	// This configuration is required for modern web applications that use:
	// - Tone.js for audio synthesis
	// - PDF.js for rendering PDFs
	// - Any library that creates Web Workers using blob: URLs
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithCSPWebWorkerSupport(), // Enables blob: URLs for both worker-src and child-src
		// Alternative: Use individual options for fine-grained control
		// hyperserve.WithCSPWorkerBlob(),  // For worker-src blob: URLs
		// hyperserve.WithCSPChildBlob(),   // For child-src blob: URLs
		// hyperserve.WithCSPScriptBlob(),  // For script-src blob: URLs
		// hyperserve.WithCSPMediaBlob(),   // For media-src blob: URLs
	)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Add middleware for secure web endpoints
	srv.AddMiddlewareStack("*", hyperserve.SecureWeb(srv.Options))

	// Serve static files (index.html, tone.js, etc.)
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(indexHTML))
		} else {
			http.NotFound(w, r)
		}
	})

	// Add a simple API endpoint
	srv.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Web Worker CSP test endpoint", "worker_support": true}`))
	})

	log.Println("Starting server on :8080")
	log.Println("Visit http://localhost:8080 to test Web Workers with blob: URLs")
	log.Fatal(srv.Run())
}

const indexHTML = `
<!DOCTYPE html>
<html>
<head>
    <title>Web Worker CSP Test</title>
    <meta charset="UTF-8">
</head>
<body>
    <h1>Web Worker CSP Test</h1>
    <p>This page tests that Web Workers can be created using blob: URLs.</p>
    <button onclick="testWebWorker()">Test Web Worker</button>
    <div id="output"></div>

    <script>
        function testWebWorker() {
            const output = document.getElementById('output');
            output.innerHTML = '<p>Testing Web Worker creation...</p>';

            // Create a Web Worker using a blob: URL (this is what Tone.js does)
            const workerScript = ` + "`" + `
                self.onmessage = function(e) {
                    console.log('Worker received:', e.data);
                    self.postMessage('Hello from Web Worker! Received: ' + e.data);
                };
            ` + "`" + `;

            try {
                const blob = new Blob([workerScript], { type: 'application/javascript' });
                const workerUrl = URL.createObjectURL(blob);
                const worker = new Worker(workerUrl);

                worker.onmessage = function(e) {
                    output.innerHTML += '<p style="color: green;">✓ Success: ' + e.data + '</p>';
                };

                worker.onerror = function(error) {
                    output.innerHTML += '<p style="color: red;">✗ Worker Error: ' + error.message + '</p>';
                };

                worker.postMessage('Hello from main thread!');
                
                // Clean up
                setTimeout(() => {
                    worker.terminate();
                    URL.revokeObjectURL(workerUrl);
                }, 1000);

            } catch (error) {
                output.innerHTML += '<p style="color: red;">✗ Error creating worker: ' + error.message + '</p>';
            }
        }

        // Test on page load
        window.onload = function() {
            testWebWorker();
        };
    </script>
</body>
</html>
`