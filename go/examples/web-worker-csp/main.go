package main

import (
	"fmt"
	"net/http"

	"github.com/osauer/hyperserve/go"
)

func main() {
	// Create server with Web Worker support enabled
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithCSPWebWorkerSupport(), // Enable blob: URLs for Web Workers
	)
	if err != nil {
		panic(err)
	}

	// Apply security headers with Web Worker support
	srv.AddMiddleware("*", hyperserve.HeadersMiddleware(srv.Options))

	// Serve static files (HTML, JS, CSS)
	srv.HandleStatic("/static/")

	// Main page
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Web Worker CSP Example</title>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .container { max-width: 800px; margin: 0 auto; }
        button { padding: 10px 20px; margin: 10px 0; font-size: 16px; cursor: pointer; }
        .success { color: green; }
        .error { color: red; }
        .info { color: blue; }
        pre { background: #f4f4f4; padding: 10px; border-radius: 4px; overflow-x: auto; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Web Worker CSP Example</h1>
        
        <h2>Test Web Worker with blob: URLs</h2>
        <p>This example demonstrates how HyperServe's CSP Web Worker support enables modern web applications to use Web Workers with blob: URLs.</p>
        
        <button onclick="testWebWorker()">Test Web Worker</button>
        <button onclick="testToneJS()">Test Tone.js (Simulated)</button>
        <button onclick="showCSPHeaders()">Show CSP Headers</button>
        
        <div id="results"></div>
        
        <h3>How it works:</h3>
        <ul>
            <li>Server started with <code>hyperserve.WithCSPWebWorkerSupport()</code></li>
            <li>CSP header includes: <code>worker-src 'self' blob:</code></li>
            <li>Web Workers can be created using blob: URLs</li>
            <li>Required for libraries like Tone.js, PDF.js, etc.</li>
        </ul>
        
        <h3>CSP Configuration:</h3>
        <pre id="csp-info">Loading CSP information...</pre>
    </div>

    <script>
        function log(message, type = 'info') {
            const results = document.getElementById('results');
            const div = document.createElement('div');
            div.className = type;
            div.innerHTML = '<strong>' + new Date().toLocaleTimeString() + '</strong>: ' + message;
            results.appendChild(div);
        }

        function testWebWorker() {
            log('Testing Web Worker with blob: URL...', 'info');
            
            try {
                // Create a Web Worker using blob: URL
                const workerCode = ` + "`" + `
                    self.onmessage = function(e) {
                        const result = e.data.a + e.data.b;
                        self.postMessage({
                            result: result,
                            message: 'Web Worker executed successfully!'
                        });
                    };
                ` + "`" + `;
                
                const blob = new Blob([workerCode], { type: 'application/javascript' });
                const workerUrl = URL.createObjectURL(blob);
                
                log('Created blob URL: ' + workerUrl, 'info');
                
                const worker = new Worker(workerUrl);
                
                worker.onmessage = function(e) {
                    log('✅ Web Worker Success: ' + e.data.message + ' (Result: ' + e.data.result + ')', 'success');
                    worker.terminate();
                };
                
                worker.onerror = function(error) {
                    log('❌ Web Worker Error: ' + error.message, 'error');
                };
                
                // Send test data to worker
                worker.postMessage({ a: 10, b: 20 });
                
            } catch (error) {
                log('❌ Failed to create Web Worker: ' + error.message, 'error');
            }
        }

        function testToneJS() {
            log('Simulating Tone.js Web Worker usage...', 'info');
            
            // Simulate what Tone.js does - create a worker for its timing engine
            try {
                const timingWorkerCode = ` + "`" + `
                    // Simulated Tone.js timing worker
                    let interval;
                    
                    self.onmessage = function(e) {
                        if (e.data.command === 'start') {
                            interval = setInterval(() => {
                                self.postMessage({ type: 'tick', time: Date.now() });
                            }, e.data.intervalMs || 100);
                        } else if (e.data.command === 'stop') {
                            clearInterval(interval);
                            self.postMessage({ type: 'stopped' });
                        }
                    };
                ` + "`" + `;
                
                const blob = new Blob([timingWorkerCode], { type: 'application/javascript' });
                const workerUrl = URL.createObjectURL(blob);
                
                log('Created Tone.js-style timing worker at: ' + workerUrl, 'info');
                
                const worker = new Worker(workerUrl);
                let tickCount = 0;
                
                worker.onmessage = function(e) {
                    if (e.data.type === 'tick') {
                        tickCount++;
                        if (tickCount <= 3) {
                            log('⏱️ Timing tick #' + tickCount + ' at ' + new Date(e.data.time).toLocaleTimeString(), 'info');
                        }
                        if (tickCount === 3) {
                            worker.postMessage({ command: 'stop' });
                        }
                    } else if (e.data.type === 'stopped') {
                        log('✅ Simulated Tone.js timing worker completed successfully!', 'success');
                        worker.terminate();
                    }
                };
                
                worker.onerror = function(error) {
                    log('❌ Simulated Tone.js Error: ' + error.message, 'error');
                };
                
                // Start the timing worker
                worker.postMessage({ command: 'start', intervalMs: 500 });
                
            } catch (error) {
                log('❌ Failed to create Tone.js-style worker: ' + error.message, 'error');
            }
        }

        function showCSPHeaders() {
            log('Fetching CSP headers...', 'info');
            
            fetch('/csp-info')
                .then(response => {
                    const csp = response.headers.get('Content-Security-Policy');
                    const permissionsPolicy = response.headers.get('Permissions-Policy');
                    
                    document.getElementById('csp-info').innerHTML = 
                        'Content-Security-Policy: ' + (csp || 'Not found') + '\n\n' +
                        'Permissions-Policy: ' + (permissionsPolicy || 'Not found');
                    
                    log('✅ CSP Headers retrieved and displayed below', 'success');
                })
                .catch(error => {
                    log('❌ Failed to fetch CSP headers: ' + error.message, 'error');
                });
        }

        // Load CSP info on page load
        window.onload = function() {
            showCSPHeaders();
        };
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	})

	// API endpoint to show CSP headers
	srv.HandleFunc("/csp-info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"message": "CSP headers are sent with this response"}`)
	})

	fmt.Println("🚀 Web Worker CSP Example")
	fmt.Println("📖 Server running at http://localhost:8080")
	fmt.Println("🔒 Web Worker support enabled (blob: URLs allowed)")
	fmt.Println("🧪 Visit http://localhost:8080 to test Web Workers")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop the server")

	srv.Run()
}