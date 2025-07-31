//! Simple echo server example
//! 
//! Run with: cargo run --example echo

use hyperserve::{Server, Request, Response, Status, Method};

fn main() -> std::io::Result<()> {
    // Create server
    let server = Server::new("127.0.0.1:8080")?
        .handle(Method::POST, "/echo", echo_handler)
        .handle_func("/", root_handler);
    
    println!("Echo server running on http://127.0.0.1:8080");
    println!("  GET  / - Server info");
    println!("  POST /echo - Echo back request body");
    
    server.run()
}

fn echo_handler(req: &Request) -> Response {
    Response::new(Status::Ok)
        .header("Content-Type", "text/plain")
        .body(req.body)
}

fn root_handler(_req: &Request) -> Response {
    Response::new(Status::Ok)
        .body("HyperServe Rust - Zero Dependencies!\n\nPOST to /echo to echo back data")
}