//! Optimized example for maximum performance

use hyperserve::{Method, Request};
use hyperserve::http::FastResponse;
use std::io::{self, Write};

fn main() -> io::Result<()> {
    // Pre-compute static responses
    static HELLO_RESPONSE: &str = "Hello from HyperServe Rust (Optimized)!";
    static JSON_RESPONSE: &str = r#"{"message":"Hello from HyperServe","version":"1.0.0","optimized":true}"#;
    
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8081")
        .route(Method::GET, "/", |_req: &Request| {
            // Use fast response builder
            let mut resp = FastResponse::new();
            resp.status(200)
                .header("Content-Type", "text/plain; charset=utf-8")
                .body(HELLO_RESPONSE.as_bytes());
            
            // Convert to standard response for compatibility
            hyperserve::Response::new(hyperserve::Status::Ok)
                .header("Content-Type", "text/plain")
                .body(HELLO_RESPONSE)
        })
        .route(Method::GET, "/json", |_req: &Request| {
            hyperserve::Response::new(hyperserve::Status::Ok)
                .header("Content-Type", "application/json")
                .body(JSON_RESPONSE)
        })
        .route(Method::POST, "/echo", |req: &Request| {
            // Fast echo - reuse request body
            hyperserve::Response::new(hyperserve::Status::Ok)
                .header("Content-Type", "application/octet-stream")
                .header("Content-Length", &req.body.len().to_string())
                .body(std::str::from_utf8(req.body).unwrap_or(""))
        })
        .route(Method::GET, "/health", |_req: &Request| {
            // Minimal health check
            hyperserve::Response::new(hyperserve::Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{"status":"ok"}"#)
        })
        .build()?;

    println!("Optimized server running on 127.0.0.1:8081");
    server.run()
}