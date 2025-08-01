//! Fast HTTP response generation
//! 
//! Optimizations:
//! - Pre-computed common responses
//! - Stack-allocated buffers
//! - Vectored I/O
//! - Branch-free status line generation

use std::io::{self, Write};
use super::fast_buffer::{FastBuffer, get_buffer, return_buffer};

/// Pre-computed response lines for common status codes
static STATUS_LINES: &[(&str, &[u8])] = &[
    ("200", b"HTTP/1.1 200 OK\r\n"),
    ("201", b"HTTP/1.1 201 Created\r\n"),
    ("204", b"HTTP/1.1 204 No Content\r\n"),
    ("400", b"HTTP/1.1 400 Bad Request\r\n"),
    ("401", b"HTTP/1.1 401 Unauthorized\r\n"),
    ("403", b"HTTP/1.1 403 Forbidden\r\n"),
    ("404", b"HTTP/1.1 404 Not Found\r\n"),
    ("405", b"HTTP/1.1 405 Method Not Allowed\r\n"),
    ("500", b"HTTP/1.1 500 Internal Server Error\r\n"),
];

/// Common headers pre-formatted
static COMMON_HEADERS: &[(&str, &[u8])] = &[
    ("content-type-html", b"Content-Type: text/html; charset=utf-8\r\n"),
    ("content-type-json", b"Content-Type: application/json\r\n"),
    ("content-type-text", b"Content-Type: text/plain; charset=utf-8\r\n"),
    ("connection-close", b"Connection: close\r\n"),
    ("connection-keep-alive", b"Connection: keep-alive\r\n"),
];

/// Fast response builder
pub struct FastResponse {
    buffer: FastBuffer,
    status_written: bool,
    headers_done: bool,
}

impl FastResponse {
    /// Create new response
    #[inline]
    pub fn new() -> Self {
        Self {
            buffer: get_buffer(),
            status_written: false,
            headers_done: false,
        }
    }
    
    /// Set status (optimized for common codes)
    #[inline]
    pub fn status(&mut self, code: u16) -> &mut Self {
        if !self.status_written {
            // Fast path for common status codes
            let status_line = match code {
                200 => &STATUS_LINES[0].1,
                201 => &STATUS_LINES[1].1,
                204 => &STATUS_LINES[2].1,
                400 => &STATUS_LINES[3].1,
                401 => &STATUS_LINES[4].1,
                403 => &STATUS_LINES[5].1,
                404 => &STATUS_LINES[6].1,
                405 => &STATUS_LINES[7].1,
                500 => &STATUS_LINES[8].1,
                _ => {
                    // Fallback for other codes
                    self.buffer.write_all(b"HTTP/1.1 ").unwrap();
                    let mut num_buf = [0u8; 8];
                    let num_str = super::fast_buffer::format_usize(code as usize, &mut num_buf);
                    self.buffer.write_all(num_str.as_bytes()).unwrap();
                    self.buffer.write_all(b" ").unwrap();
                    self.buffer.write_all(status_text(code).as_bytes()).unwrap();
                    self.buffer.write_all(b"\r\n").unwrap();
                    self.status_written = true;
                    return self;
                }
            };
            
            self.buffer.write_all(status_line).unwrap();
            self.status_written = true;
        }
        self
    }
    
    /// Add header (optimized for common headers)
    #[inline]
    pub fn header(&mut self, name: &str, value: &str) -> &mut Self {
        if self.headers_done {
            return self;
        }
        
        // Fast path for common headers
        match (name, value) {
            ("Content-Type", "text/html; charset=utf-8") => {
                self.buffer.write_all(COMMON_HEADERS[0].1).unwrap();
            }
            ("Content-Type", "application/json") => {
                self.buffer.write_all(COMMON_HEADERS[1].1).unwrap();
            }
            ("Content-Type", "text/plain; charset=utf-8") => {
                self.buffer.write_all(COMMON_HEADERS[2].1).unwrap();
            }
            ("Connection", "close") => {
                self.buffer.write_all(COMMON_HEADERS[3].1).unwrap();
            }
            ("Connection", "keep-alive") => {
                self.buffer.write_all(COMMON_HEADERS[4].1).unwrap();
            }
            _ => {
                // Generic path
                self.buffer.write_all(name.as_bytes()).unwrap();
                self.buffer.write_all(b": ").unwrap();
                self.buffer.write_all(value.as_bytes()).unwrap();
                self.buffer.write_all(b"\r\n").unwrap();
            }
        }
        self
    }
    
    /// Set Content-Length header with zero-allocation formatting
    #[inline]
    pub fn content_length(&mut self, len: usize) -> &mut Self {
        if !self.headers_done {
            self.buffer.write_all(b"Content-Length: ").unwrap();
            let mut num_buf = [0u8; 20];
            let num_str = super::fast_buffer::format_usize(len, &mut num_buf);
            self.buffer.write_all(num_str.as_bytes()).unwrap();
            self.buffer.write_all(b"\r\n").unwrap();
        }
        self
    }
    
    /// Add body
    #[inline]
    pub fn body(&mut self, body: &[u8]) -> &mut Self {
        if !self.headers_done {
            self.content_length(body.len());
            self.buffer.write_all(b"\r\n").unwrap();
            self.headers_done = true;
        }
        self.buffer.write_all(body).unwrap();
        self
    }
    
    /// Write response to stream
    #[inline]
    pub fn write_to<W: Write>(mut self, writer: &mut W) -> io::Result<()> {
        if !self.status_written {
            self.status(200);
        }
        if !self.headers_done {
            self.buffer.write_all(b"\r\n").unwrap();
        }
        
        self.buffer.write_to(writer)?;
        
        // Return buffer to pool
        return_buffer(self.buffer);
        
        Ok(())
    }
}

/// Get status text for code
#[inline]
fn status_text(code: u16) -> &'static str {
    match code {
        200 => "OK",
        201 => "Created",
        204 => "No Content",
        400 => "Bad Request",
        401 => "Unauthorized",
        403 => "Forbidden",
        404 => "Not Found",
        405 => "Method Not Allowed",
        429 => "Too Many Requests",
        500 => "Internal Server Error",
        _ => "Unknown",
    }
}