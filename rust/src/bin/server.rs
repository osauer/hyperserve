//! HyperServe Rust server binary

use hyperserve::{Method, Response, Status};
use hyperserve::middleware::{LoggingMiddleware, SecurityHeadersMiddleware};
use std::env;

#[cfg(feature = "mcp")]
use hyperserve::mcp::{MCPHandler, MCPServerInfo};
#[cfg(feature = "mcp")]
use hyperserve::mcp::tools::{EchoTool, CalculatorTool};
#[cfg(feature = "mcp")]
use hyperserve::mcp::resources::{StaticResource, SystemInfoResource};

fn main() -> std::io::Result<()> {
    // Parse command line arguments
    let args: Vec<String> = env::args().collect();
    let mut port = 8080;
    
    // Simple argument parsing
    for i in 1..args.len() {
        if args[i] == "--port" && i + 1 < args.len() {
            port = args[i + 1].parse().unwrap_or(8080);
        }
    }
    
    // Create server using builder
    let mut builder = hyperserve::builder::ServerBuilder::new(format!("127.0.0.1:{}", port))
        .middleware(LoggingMiddleware)
        .middleware(SecurityHeadersMiddleware::default())
        .route(Method::GET, "/", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "text/html")
                .body(r#"<!DOCTYPE html>
<html>
<head><title>HyperServe Rust</title></head>
<body>
<h1>HyperServe Rust Implementation</h1>
<p>Server is running!</p>
</body>
</html>"#)
        })
        .route(Method::GET, "/health", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{"status":"healthy","service":"hyperserve-rust"}"#)
        });
    
    // Add MCP support if feature is enabled
    #[cfg(feature = "mcp")]
    {
        use std::sync::Arc;
        
        // Create MCP handler
        let mcp_handler = Arc::new(MCPHandler::new(MCPServerInfo {
            name: "HyperServe Rust".to_string(),
            version: env!("CARGO_PKG_VERSION").to_string(),
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
            "Welcome to HyperServe Rust!".to_string(),
        )));
        
        // Add MCP routes
        builder = builder
            .route(Method::POST, "/mcp", {
                let mcp_handler = Arc::clone(&mcp_handler);
                move |req| mcp_handler.handle_request(req)
            })
            .route(Method::GET, "/mcp", {
                let mcp_handler = Arc::clone(&mcp_handler);
                move |req| mcp_handler.handle_request(req)
            });
    }
    
    let server = builder.build()?;
    
    println!("HyperServe Rust server listening on 127.0.0.1:{}", port);
    
    server.run()
}