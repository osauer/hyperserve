//! MCP (Model Context Protocol) example
//! 
//! Demonstrates HyperServe's MCP capabilities for AI assistant integration

use hyperserve::{Method, Response, Status};
use hyperserve::middleware::{LoggingMiddleware, SecurityHeadersMiddleware};

#[cfg(feature = "mcp")]
use hyperserve::mcp::{MCPHandler, MCPServerInfo};
#[cfg(feature = "mcp")]
use hyperserve::mcp::tools::{EchoTool, CalculatorTool};
#[cfg(feature = "mcp")]
use hyperserve::mcp::resources::{StaticResource, SystemInfoResource};

fn main() -> std::io::Result<()> {
    #[cfg(not(feature = "mcp"))]
    {
        eprintln!("This example requires the 'mcp' feature. Run with:");
        eprintln!("  cargo run --example mcp --features mcp");
        std::process::exit(1);
    }
    
    #[cfg(feature = "mcp")]
    {
        use std::sync::Arc;
        
        // Create MCP handler
        let mcp_handler = Arc::new(MCPHandler::new(MCPServerInfo {
            name: "HyperServe MCP".to_string(),
            version: "0.1.0".to_string(),
        }));
        
        // Register tools
        mcp_handler.register_tool(Box::new(EchoTool));
        mcp_handler.register_tool(Box::new(CalculatorTool));
        
        // Register resources
        mcp_handler.register_resource(Box::new(SystemInfoResource));
        mcp_handler.register_resource(Box::new(StaticResource::new(
            "welcome://message".to_string(),
            "Welcome Message".to_string(),
            "A friendly welcome message".to_string(),
            "Welcome to HyperServe MCP!".to_string(),
        )));
        
        // Create server using builder
        let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8080")
            .middleware(LoggingMiddleware)
            .middleware(SecurityHeadersMiddleware::default())
            .route(Method::GET, "/", |_| {
                Response::new(Status::Ok)
                    .header("Content-Type", "text/html")
                    .body(r#"
<!DOCTYPE html>
<html>
<head>
    <title>HyperServe MCP Example</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        pre { background: #f0f0f0; padding: 10px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>HyperServe MCP Example</h1>
    <p>This server implements the Model Context Protocol (MCP) for AI assistant integration.</p>
    
    <h2>Available Endpoints</h2>
    <ul>
        <li><strong>/mcp</strong> - MCP protocol endpoint</li>
        <li><strong>/health</strong> - Health check</li>
    </ul>
    
    <h2>Test MCP</h2>
    <p>Use curl to test the MCP endpoint:</p>
    
    <h3>1. Initialize</h3>
    <pre>curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {"name": "test", "version": "1.0"}
    },
    "id": 1
  }'</pre>
    
    <h3>2. List Tools</h3>
    <pre>curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "tools/list", "id": 2}'</pre>
    
    <h3>3. Call Echo Tool</h3>
    <pre>curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "echo",
      "arguments": {"message": "Hello, MCP!"}
    },
    "id": 3
  }'</pre>
    
    <h3>4. Use Calculator</h3>
    <pre>curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "calculator",
      "arguments": {"operation": "add", "a": 5, "b": 3}
    },
    "id": 4
  }'</pre>
</body>
</html>
                    "#)
            })
            .route(Method::GET, "/health", |_| {
                Response::new(Status::Ok)
                    .header("Content-Type", "application/json")
                    .body(r#"{"status":"healthy","service":"hyperserve-mcp"}"#)
            })
            .route(Method::POST, "/mcp", {
                let mcp_handler = Arc::clone(&mcp_handler);
                move |req| mcp_handler.handle_request(req)
            })
            .route(Method::GET, "/mcp", {
                let mcp_handler = Arc::clone(&mcp_handler);
                move |req| mcp_handler.handle_request(req)
            })
            .build()?;
        
        println!("MCP server running on http://127.0.0.1:8080");
        println!("Visit http://127.0.0.1:8080 for instructions");
        
        server.run()
    }
}