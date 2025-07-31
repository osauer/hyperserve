//! HTTP response builder

use std::io::{Write, Result as IoResult};

/// HTTP status codes
#[derive(Debug, Clone, Copy, PartialEq)]
#[repr(u16)]
pub enum Status {
    Ok = 200,
    Created = 201,
    NoContent = 204,
    BadRequest = 400,
    Unauthorized = 401,
    Forbidden = 403,
    NotFound = 404,
    MethodNotAllowed = 405,
    TooManyRequests = 429,
    InternalServerError = 500,
}

impl Status {
    /// Get status text
    pub fn text(&self) -> &'static str {
        match self {
            Self::Ok => "OK",
            Self::Created => "Created",
            Self::NoContent => "No Content",
            Self::BadRequest => "Bad Request",
            Self::Unauthorized => "Unauthorized",
            Self::Forbidden => "Forbidden",
            Self::NotFound => "Not Found",
            Self::MethodNotAllowed => "Method Not Allowed",
            Self::TooManyRequests => "Too Many Requests",
            Self::InternalServerError => "Internal Server Error",
        }
    }
}

/// HTTP response builder
pub struct Response {
    pub(crate) status: Status,
    pub(crate) headers: Vec<(&'static str, String)>,
    pub(crate) body: Vec<u8>,
}

impl Response {
    /// Create new response with status
    pub fn new(status: Status) -> Self {
        Self {
            status,
            headers: vec![
                ("Server", "HyperServe-Rust/0.1.0".to_string()),
                ("Content-Type", "text/plain; charset=utf-8".to_string()),
            ],
            body: Vec::new(),
        }
    }
    
    /// Add header
    pub fn header(mut self, key: &'static str, value: impl Into<String>) -> Self {
        // Remove existing header if present
        self.headers.retain(|(k, _)| *k != key);
        self.headers.push((key, value.into()));
        self
    }
    
    /// Add header to existing response (for middleware)
    pub fn add_header(&mut self, key: &'static str, value: impl Into<String>) {
        self.headers.push((key, value.into()));
    }
    
    /// Set body
    pub fn body(mut self, body: impl Into<Vec<u8>>) -> Self {
        self.body = body.into();
        self
    }
    
    /// Set JSON body (requires serde feature)
    #[cfg(feature = "json")]
    pub fn json<T: serde::Serialize>(mut self, value: &T) -> Self {
        match serde_json::to_vec(value) {
            Ok(json) => {
                self.body = json;
                self.headers.retain(|(k, _)| *k != "Content-Type");
                self.headers.push(("Content-Type", "application/json".to_string()));
            }
            Err(_) => {
                self.status = Status::InternalServerError;
                self.body = b"JSON serialization error".to_vec();
            }
        }
        self
    }
    
    /// Write response to stream
    pub fn write_to<W: Write>(&self, writer: &mut W) -> IoResult<()> {
        // Status line
        write!(writer, "HTTP/1.1 {} {}\r\n", 
            self.status as u16, 
            self.status.text()
        )?;
        
        // Headers
        for (key, value) in &self.headers {
            write!(writer, "{}: {}\r\n", key, value)?;
        }
        
        // Content-Length
        write!(writer, "Content-Length: {}\r\n", self.body.len())?;
        
        // Connection header
        write!(writer, "Connection: close\r\n")?;
        
        // End headers
        write!(writer, "\r\n")?;
        
        // Body
        writer.write_all(&self.body)?;
        writer.flush()
    }
}

// Convenience constructors
impl Response {
    /// 200 OK response
    pub fn ok() -> Self {
        Self::new(Status::Ok)
    }
    
    /// 404 Not Found response
    pub fn not_found() -> Self {
        Self::new(Status::NotFound)
            .body("404 Not Found")
    }
    
    /// 500 Internal Server Error response
    pub fn internal_error() -> Self {
        Self::new(Status::InternalServerError)
            .body("Internal Server Error")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_response_builder() {
        let response = Response::new(Status::Ok)
            .header("X-Custom", "test")
            .body("Hello, World!");
            
        let mut output = Vec::new();
        response.write_to(&mut output).unwrap();
        
        let response_str = String::from_utf8(output).unwrap();
        assert!(response_str.contains("HTTP/1.1 200 OK"));
        assert!(response_str.contains("X-Custom: test"));
        assert!(response_str.contains("Hello, World!"));
    }
}