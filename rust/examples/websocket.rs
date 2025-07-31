//! WebSocket server example
//! 
//! Run with: cargo run --example websocket

use hyperserve::{Server, Request, Response, Method};

fn main() -> std::io::Result<()> {
    // Create server
    let server = Server::new("127.0.0.1:8080")?
        .handle(Method::GET, "/", home_handler)
        .handle(Method::GET, "/ws", websocket_handler);
    
    println!("WebSocket server running on http://127.0.0.1:8080");
    println!("Routes:");
    println!("  GET  /    - WebSocket client UI");
    println!("  GET  /ws  - WebSocket endpoint");
    
    server.run()
}

fn home_handler(_req: &Request) -> Response {
    Response::ok()
        .header("Content-Type", "text/html; charset=utf-8")
        .body(r#"<!DOCTYPE html>
<html>
<head>
    <title>HyperServe WebSocket Demo</title>
    <style>
        body {
            font-family: monospace;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
            background: #1a1a1a;
            color: #0f0;
        }
        h1 { color: #0f0; }
        #messages {
            height: 300px;
            overflow-y: auto;
            border: 1px solid #0f0;
            padding: 10px;
            margin: 20px 0;
            background: #000;
        }
        .message { margin: 5px 0; }
        .sent { color: #0ff; }
        .received { color: #0f0; }
        .system { color: #ff0; }
        input, button {
            font-family: monospace;
            background: #000;
            color: #0f0;
            border: 1px solid #0f0;
            padding: 5px;
        }
        input { width: 400px; }
        button { cursor: pointer; }
        button:hover { background: #0f0; color: #000; }
    </style>
</head>
<body>
    <h1>HyperServe WebSocket Demo</h1>
    <div id="messages"></div>
    <input type="text" id="messageInput" placeholder="Type a message..." />
    <button onclick="sendMessage()">Send</button>
    <button onclick="connect()">Connect</button>
    <button onclick="disconnect()">Disconnect</button>
    
    <script>
        let ws = null;
        const messages = document.getElementById('messages');
        const input = document.getElementById('messageInput');
        
        function log(message, className) {
            const div = document.createElement('div');
            div.className = 'message ' + className;
            div.textContent = new Date().toLocaleTimeString() + ' - ' + message;
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }
        
        function connect() {
            if (ws) {
                log('Already connected', 'system');
                return;
            }
            
            ws = new WebSocket('ws://127.0.0.1:8080/ws');
            
            ws.onopen = () => {
                log('Connected to WebSocket server', 'system');
            };
            
            ws.onmessage = (event) => {
                log('Received: ' + event.data, 'received');
            };
            
            ws.onclose = () => {
                log('Disconnected from server', 'system');
                ws = null;
            };
            
            ws.onerror = (error) => {
                log('Error: ' + error, 'system');
            };
        }
        
        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
        }
        
        function sendMessage() {
            if (!ws || ws.readyState !== WebSocket.OPEN) {
                log('Not connected', 'system');
                return;
            }
            
            const message = input.value.trim();
            if (message) {
                ws.send(message);
                log('Sent: ' + message, 'sent');
                input.value = '';
            }
        }
        
        input.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                sendMessage();
            }
        });
        
        // Auto-connect on load
        connect();
    </script>
</body>
</html>"#)
}

fn websocket_handler(_req: &Request) -> Response {
    // This is a special handler that will upgrade to WebSocket
    // In a real implementation, this would be handled by the server
    Response::ok().body("WebSocket endpoint - upgrade required")
}