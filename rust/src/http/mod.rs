//! Zero-dependency HTTP/1.1 implementation
//! 
//! A minimal, performant HTTP server built from scratch using Rust 2024 features.

use std::io::Read;
use std::net::TcpStream;
use std::str;

mod parser;
mod response;
mod router;
mod thread_pool;
mod concurrent_queue;
mod optimized_pool;
mod server_config;
mod fast_buffer;
mod fast_parser;
mod fast_response;

pub use parser::{Request, ParseError};
pub use response::{Response, Status};
pub use router::Router;
pub use thread_pool::ThreadPool;
pub use optimized_pool::OptimizedPool;
pub use server_config::{ThreadPoolConfig, SystemInfo};
pub use fast_response::FastResponse;

/// HTTP methods
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Method {
    GET,
    POST,
    PUT,
    DELETE,
    HEAD,
    OPTIONS,
    PATCH,
}

impl Method {
    /// Parse method from bytes
    pub fn from_bytes(bytes: &[u8]) -> Option<Self> {
        match bytes {
            b"GET" => Some(Self::GET),
            b"POST" => Some(Self::POST),
            b"PUT" => Some(Self::PUT),
            b"DELETE" => Some(Self::DELETE),
            b"HEAD" => Some(Self::HEAD),
            b"OPTIONS" => Some(Self::OPTIONS),
            b"PATCH" => Some(Self::PATCH),
            _ => None,
        }
    }
    
    /// Convert to string representation
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::GET => "GET",
            Self::POST => "POST",
            Self::PUT => "PUT",
            Self::DELETE => "DELETE",
            Self::HEAD => "HEAD",
            Self::OPTIONS => "OPTIONS",
            Self::PATCH => "PATCH",
        }
    }
}

/// Handle a single TCP connection
pub fn handle_connection(mut stream: TcpStream, router: &Router) {
    let mut buffer = vec![0u8; 8192];
    let mut total_read = 0;
    
    // Read initial data
    let n = match stream.read(&mut buffer) {
        Ok(n) if n > 0 => n,
        _ => return,
    };
    total_read = n;
    
    // Parse headers first to get Content-Length
    let (request, body_start) = match Request::parse(&buffer[..n]) {
        Ok((req, body_start)) => (req, body_start),
        Err(_) => {
            let _ = Response::new(Status::BadRequest)
                .body("Bad Request")
                .write_to(&mut stream);
            return;
        }
    };
    
    // Check if we need to read more body data
    if let Some(content_length) = request.content_length() {
        let headers_size = body_start;
        let total_size = headers_size + content_length;
        
        // Resize buffer if needed
        if total_size > buffer.len() {
            buffer.resize(total_size, 0);
        }
        
        // Read remaining body if necessary
        while total_read < total_size {
            match stream.read(&mut buffer[total_read..total_size]) {
                Ok(0) => break, // EOF
                Ok(n) => total_read += n,
                Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => continue,
                Err(_) => {
                    let _ = Response::new(Status::BadRequest)
                        .body("Failed to read request body")
                        .write_to(&mut stream);
                    return;
                }
            }
        }
    }
    
    // Re-parse with complete data
    let request = match Request::parse(&buffer[..total_read]) {
        Ok((req, _)) => req,
        Err(_) => {
            let _ = Response::new(Status::BadRequest)
                .body("Bad Request")
                .write_to(&mut stream);
            return;
        }
    };
    
    // Check for WebSocket upgrade first
    match crate::websocket::handshake(&request, stream.try_clone().unwrap()) {
        Ok(Some(ws)) => {
            // Handle WebSocket connection
            handle_websocket(ws, &request);
            return;
        }
        Ok(None) => {
            // Not a WebSocket request, handle as regular HTTP
            let response = router.handle(&request);
            let _ = response.write_to(&mut stream);
        }
        Err(e) => {
            eprintln!("WebSocket handshake error: {}", e);
            let _ = Response::internal_error().write_to(&mut stream);
        }
    }
}

/// Handle WebSocket connection
fn handle_websocket(mut ws: crate::websocket::WebSocket, request: &Request) {
    use crate::websocket::OpCode;
    
    println!("WebSocket connection established for {}", request.path);
    
    // Send welcome message
    let _ = ws.send_text("Welcome to HyperServe WebSocket!");
    
    // Echo loop
    loop {
        match ws.read_frame() {
            Ok(Some(frame)) => {
                match frame.opcode {
                    OpCode::Text => {
                        let text = String::from_utf8_lossy(&frame.payload);
                        println!("WebSocket received: {}", text);
                        
                        // Echo back
                        let response = format!("Echo: {}", text);
                        let _ = ws.send_text(&response);
                    }
                    OpCode::Binary => {
                        println!("WebSocket received {} bytes of binary data", frame.payload.len());
                        // Echo back
                        let _ = ws.send_binary(frame.payload);
                    }
                    OpCode::Close => {
                        println!("WebSocket close requested");
                        let _ = ws.close();
                        break;
                    }
                    OpCode::Ping => {
                        let _ = ws.send_pong(frame.payload);
                    }
                    _ => {}
                }
            }
            Ok(None) => break,
            Err(e) => {
                eprintln!("WebSocket error: {}", e);
                break;
            }
        }
    }
    
    println!("WebSocket connection closed");
}