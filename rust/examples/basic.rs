//! Basic HTTP server example for benchmarking

use hyperserve::{Method, Response, Status, Request};
use std::io::{self, Read};

fn main() -> io::Result<()> {
    // Create server on port 8081
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8081")
        .route(Method::GET, "/", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "text/plain")
                .body("Hello from HyperServe Rust!")
        })
        .route(Method::POST, "/echo", |req| {
            // Echo back the request body
            Response::new(Status::Ok)
                .header("Content-Type", "application/octet-stream")
                .header("Content-Length", &req.body.len().to_string())
                .body(std::str::from_utf8(&req.body).unwrap_or(""))
        })
        .route(Method::GET, "/json", |_| {
            let json = format!(
                r#"{{"message":"Hello from HyperServe Rust","timestamp":{},"version":"1.0.0"}}"#,
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs()
            );
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(json)
        })
        .route(Method::GET, "/health", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{"status":"ok"}"#)
        })
        .build()?;

    println!("Starting server on 127.0.0.1:8081");
    server.run()
}