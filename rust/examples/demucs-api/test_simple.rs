//! Simple test to debug headers

use hyperserve::{Method, Response, Status, Request};
use hyperserve::middleware::LoggingMiddleware;

fn main() -> std::io::Result<()> {
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8083")
        .middleware(LoggingMiddleware)
        .route(Method::POST, "/test", |req| {
            println!("Headers:");
            for (k, v) in &req.headers {
                println!("  {}: {}", k, v);
            }
            
            let content_type = req.headers.get("content-type");
            println!("Content-Type from get: {:?}", content_type);
            
            Response::new(Status::Ok)
                .header("Content-Type", "text/plain")
                .body("OK")
        })
        .build()?;
    
    println!("Test server on http://127.0.0.1:8083");
    server.run()
}