//! HTTP request parser - zero allocation design

use super::Method;
use std::collections::HashMap;
use std::str;

/// HTTP request representation
#[derive(Debug)]
pub struct Request<'a> {
    pub method: Method,
    pub path: &'a str,
    pub version: &'a str,
    pub headers: HashMap<&'a str, &'a str>,
    pub body: &'a [u8],
}

/// Parse errors
#[derive(Debug)]
pub enum ParseError {
    InvalidRequest,
    InvalidMethod,
    InvalidPath,
    InvalidVersion,
    InvalidHeader,
}

impl<'a> Request<'a> {
    /// Parse HTTP request from bytes
    /// Returns the request and number of bytes consumed
    pub fn parse(buffer: &'a [u8]) -> Result<(Self, usize), ParseError> {
        let mut headers = HashMap::new();
        let mut lines = buffer.split(|&b| b == b'\n');
        
        // Parse request line
        let request_line = lines.next()
            .ok_or(ParseError::InvalidRequest)?;
            
        let mut parts = request_line
            .trim_ascii_end()  // Remove \r if present
            .split(|&b| b == b' ');
        
        // Method
        let method = parts.next()
            .and_then(Method::from_bytes)
            .ok_or(ParseError::InvalidMethod)?;
            
        // Path
        let path = parts.next()
            .and_then(|p| str::from_utf8(p).ok())
            .ok_or(ParseError::InvalidPath)?;
            
        // Version
        let version = parts.next()
            .and_then(|v| str::from_utf8(v).ok())
            .ok_or(ParseError::InvalidVersion)?;
        
        // Parse headers
        let mut body_start = 0;
        for line in lines {
            // Empty line marks end of headers (handles both \n and \r\n)
            if line.is_empty() || (line.len() == 1 && line[0] == b'\r') {
                body_start = unsafe { 
                    line.as_ptr().offset_from(buffer.as_ptr()) 
                } as usize + line.len() + 1; // Skip the line ending
                break;
            }
            
            // Parse header
            let line = line.trim_ascii_end();
            if let Some(colon_pos) = line.iter().position(|&b| b == b':') {
                let key = str::from_utf8(&line[..colon_pos])
                    .map_err(|_| ParseError::InvalidHeader)?;
                let value = str::from_utf8(&line[colon_pos + 1..])
                    .map_err(|_| ParseError::InvalidHeader)?
                    .trim();
                headers.insert(key, value);
            }
        }
        
        let body = &buffer[body_start..];
        
        Ok((Request {
            method,
            path,
            version,
            headers,
            body,
        }, body_start))
    }
    
    /// Get content length from headers
    pub fn content_length(&self) -> Option<usize> {
        self.headers.get("Content-Length")
            .and_then(|v| v.parse().ok())
    }
}

// Rust 2024 feature: improved trait implementations
impl std::fmt::Display for ParseError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::InvalidRequest => write!(f, "Invalid HTTP request"),
            Self::InvalidMethod => write!(f, "Invalid HTTP method"),
            Self::InvalidPath => write!(f, "Invalid request path"),
            Self::InvalidVersion => write!(f, "Invalid HTTP version"),
            Self::InvalidHeader => write!(f, "Invalid HTTP header"),
        }
    }
}

impl std::error::Error for ParseError {}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_parse_get_request() {
        let request = b"GET /index.html HTTP/1.1\r\nHost: localhost\r\n\r\n";
        let (req, _) = Request::parse(request).unwrap();
        
        assert_eq!(req.method, Method::GET);
        assert_eq!(req.path, "/index.html");
        assert_eq!(req.version, "HTTP/1.1");
        assert_eq!(req.headers.get("Host"), Some(&"localhost"));
    }
    
    #[test]
    fn test_parse_post_with_body() {
        let request = b"POST /api/users HTTP/1.1\r\nContent-Length: 13\r\n\r\nHello, World!";
        let (req, _) = Request::parse(request).unwrap();
        
        assert_eq!(req.method, Method::POST);
        assert_eq!(req.body, b"Hello, World!");
        assert_eq!(req.content_length(), Some(13));
    }
}