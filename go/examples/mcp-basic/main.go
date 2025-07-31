// Package main demonstrates MCP with Server-Sent Events (SSE) support.
// This example shows how to use hyperserve's MCP implementation with real-time SSE connections.
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/osauer/hyperserve"
)

func main() {
	// Create server with MCP support
	srv, err := hyperserve.NewServer(
		hyperserve.WithAddr(":8080"),
		hyperserve.WithMCPSupport("MCP-SSE-Demo", "1.0.0"),
		hyperserve.WithMCPBuiltinTools(true),
		hyperserve.WithMCPBuiltinResources(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Add a demo endpoint
	srv.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>MCP SSE Demo</title>
    <style>
        body { font-family: monospace; margin: 20px; }
        .log { background: #f0f0f0; padding: 10px; margin: 10px 0; white-space: pre-wrap; }
        button { padding: 10px; margin: 5px; }
    </style>
</head>
<body>
    <h1>MCP SSE Demo</h1>
    
    <div>
        <button onclick="connectSSE()">Connect SSE</button>
        <button onclick="disconnect()">Disconnect</button>
        <button onclick="sendInitialize()">Initialize</button>
        <button onclick="sendListTools()">List Tools</button>
    </div>
    
    <h2>SSE Messages:</h2>
    <div id="messages"></div>
    
    <script>
    let eventSource = null;
    let clientId = null;
    
    function log(message) {
        const div = document.createElement('div');
        div.className = 'log';
        div.textContent = new Date().toISOString() + ' - ' + message;
        document.getElementById('messages').appendChild(div);
    }
    
    function connectSSE() {
        if (eventSource) {
            log('Already connected');
            return;
        }
        
        eventSource = new EventSource('/mcp/sse');
        
        eventSource.addEventListener('connection', function(e) {
            const data = JSON.parse(e.data);
            clientId = data.clientId;
            log('Connected: ' + JSON.stringify(data, null, 2));
        });
        
        eventSource.addEventListener('message', function(e) {
            log('Message: ' + e.data);
        });
        
        eventSource.addEventListener('notification', function(e) {
            log('Notification: ' + e.data);
        });
        
        eventSource.addEventListener('ping', function(e) {
            log('Ping: ' + e.data);
        });
        
        eventSource.onerror = function(e) {
            log('Error: ' + e);
        };
    }
    
    function disconnect() {
        if (eventSource) {
            eventSource.close();
            eventSource = null;
            log('Disconnected');
        }
    }
    
    async function sendRequest(method, params = {}) {
        if (!clientId) {
            log('Not connected to SSE');
            return;
        }
        
        const request = {
            jsonrpc: "2.0",
            method: method,
            params: params,
            id: Date.now()
        };
        
        try {
            const response = await fetch('/mcp', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-SSE-Client-ID': clientId
                },
                body: JSON.stringify(request)
            });
            
            const result = await response.text();
            log('Request sent: ' + method + ', Response: ' + result);
        } catch (err) {
            log('Request error: ' + err);
        }
    }
    
    function sendInitialize() {
        sendRequest('initialize', {
            protocolVersion: "2024-11-05",
            capabilities: {},
            clientInfo: {
                name: "web-demo",
                version: "1.0.0"
            }
        });
    }
    
    function sendListTools() {
        sendRequest('tools/list');
    }
    </script>
</body>
</html>`)
	})

	// Add a demo endpoint that shows SSE in action
	srv.HandleFunc("/demo/time", func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}
		
		// Send time updates every second
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-r.Context().Done():
				return
			case t := <-ticker.C:
				fmt.Fprintf(w, "data: %s\n\n", t.Format(time.RFC3339))
				flusher.Flush()
			}
		}
	})

	log.Println("MCP SSE Demo server starting on http://localhost:8080")
	log.Println("Visit http://localhost:8080 for the interactive demo")
	log.Println("MCP endpoint: http://localhost:8080/mcp")
	log.Println("MCP SSE endpoint: http://localhost:8080/mcp/sse")
	
	if err := srv.Run(); err != nil {
		log.Fatal("Server failed:", err)
	}
}