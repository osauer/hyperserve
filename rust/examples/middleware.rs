//! Example showing middleware usage
//! 
//! Run with: cargo run --example middleware

use hyperserve::{ServerBuilder, Request, Response, Status, Method};
use hyperserve::{LoggingMiddleware, RecoveryMiddleware, SecurityHeadersMiddleware, RateLimitMiddleware};
use std::time::Duration;

fn main() -> std::io::Result<()> {
    // Create server with middleware
    let server = ServerBuilder::new("127.0.0.1:8080")
        // Add logging middleware
        .middleware(LoggingMiddleware)
        // Add panic recovery  
        .middleware(RecoveryMiddleware)
        // Add security headers
        .middleware(SecurityHeadersMiddleware::default())
        // Add rate limiting: 10 requests per minute
        .middleware(RateLimitMiddleware::new(10, Duration::from_secs(60)))
        // Add routes
        .route(Method::GET, "/", home_handler)
        .route(Method::GET, "/panic", panic_handler)
        .route(Method::POST, "/echo", echo_handler)
        .build()?;
    
    println!("Server with middleware running on http://127.0.0.1:8080");
    println!("Routes:");
    println!("  GET  /      - Home page");
    println!("  GET  /panic - Triggers a panic (recovered by middleware)");
    println!("  POST /echo  - Echo request body");
    println!("\nMiddleware:");
    println!("  - Request logging");
    println!("  - Panic recovery");
    println!("  - Security headers");
    println!("  - Rate limiting (10 req/min)");
    
    server.run()
}

fn home_handler(_req: &Request) -> Response {
    Response::ok()
        .body("HyperServe with Middleware!\n\nTry /panic to test recovery middleware")
}

fn panic_handler(_req: &Request) -> Response {
    panic!("This panic will be caught by RecoveryMiddleware!");
}

fn echo_handler(req: &Request) -> Response {
    Response::ok()
        .header("Content-Type", "text/plain")
        .body(req.body)
}