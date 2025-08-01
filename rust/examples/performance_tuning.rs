//! Example demonstrating performance tuning options
//! 
//! This shows how to enable experimental performance features
//! WARNING: Some features may be unstable

use hyperserve::{Server, Request, Response, Status};

fn main() -> std::io::Result<()> {
    println!("Performance Tuning Example");
    println!("=========================");
    
    // Create server with experimental optimizations
    let server = Server::new("127.0.0.1:8082")?
        .with_optimized_pool(true)  // Enable experimental thread pool
        .handle_func("/", hello)
        .handle_func("/status", status);
    
    println!("\nServer configured with:");
    println!("- Experimental optimized thread pool: ENABLED");
    println!("- Note: This may improve performance but could be unstable");
    println!("\nStarting server on http://127.0.0.1:8082");
    
    server.run()
}

fn hello(_req: &Request) -> Response {
    Response::new(Status::Ok)
        .body("Hello from performance-tuned HyperServe!\n")
}

fn status(_req: &Request) -> Response {
    Response::new(Status::Ok)
        .header("Content-Type", "application/json")
        .body(r#"{"status": "ok", "mode": "performance-tuned"}"#)
}