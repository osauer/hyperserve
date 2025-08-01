//! Optimized HTTP parser for maximum single-request performance
//! 
//! Optimizations:
//! - SIMD for finding delimiters (when available)
//! - Branch prediction hints
//! - Minimal allocations
//! - Fast path for common cases

use std::hint;

/// Find a byte in a slice (optimized)
#[inline(always)]
pub fn find_byte(haystack: &[u8], needle: u8) -> Option<usize> {
    // For small slices, use simple loop
    if haystack.len() < 32 {
        return haystack.iter().position(|&b| b == needle);
    }
    
    // For larger slices, use memchr-style optimization
    let mut pos = 0;
    let len = haystack.len();
    
    // Process 8 bytes at a time using bit manipulation
    while pos + 8 <= len {
        let chunk = unsafe { *(haystack.as_ptr().add(pos) as *const u64) };
        let mask = 0x0101010101010101u64 * (needle as u64);
        let xor = chunk ^ mask;
        let has_zero = (xor.wrapping_sub(0x0101010101010101) & !xor & 0x8080808080808080);
        
        if has_zero != 0 {
            // Found potential match, check each byte
            for i in 0..8 {
                if haystack[pos + i] == needle {
                    return Some(pos + i);
                }
            }
        }
        pos += 8;
    }
    
    // Check remaining bytes
    haystack[pos..].iter().position(|&b| b == needle).map(|i| pos + i)
}

/// Fast HTTP method parsing
#[inline(always)]
pub fn parse_method_fast(bytes: &[u8]) -> Option<super::Method> {
    if bytes.len() < 3 {
        return None;
    }
    
    // Use first 3 bytes for fast discrimination
    let key = (bytes[0] as u32) << 16 | (bytes[1] as u32) << 8 | (bytes[2] as u32);
    
    match key {
        0x474554 => { // "GET"
            hint::black_box(Some(super::Method::GET))
        }
        0x504F53 => { // "POS"
            if bytes.len() >= 4 && bytes[3] == b'T' {
                Some(super::Method::POST)
            } else {
                None
            }
        }
        0x505554 => { // "PUT"
            Some(super::Method::PUT)
        }
        0x44454C => { // "DEL"
            if bytes.len() >= 6 && &bytes[3..6] == b"ETE" {
                Some(super::Method::DELETE)
            } else {
                None
            }
        }
        0x484541 => { // "HEA"
            if bytes.len() >= 4 && bytes[3] == b'D' {
                Some(super::Method::HEAD)
            } else {
                None
            }
        }
        _ => None,
    }
}

/// Fast path for common HTTP versions
#[inline(always)]
pub fn is_http_11(bytes: &[u8]) -> bool {
    bytes.len() >= 8 && 
    bytes[0] == b'H' &&
    bytes[1] == b'T' &&
    bytes[2] == b'T' &&
    bytes[3] == b'P' &&
    bytes[4] == b'/' &&
    bytes[5] == b'1' &&
    bytes[6] == b'.' &&
    bytes[7] == b'1'
}

/// Fast header name normalization (to lowercase)
#[inline(always)]
pub fn normalize_header_name(bytes: &mut [u8]) {
    for b in bytes.iter_mut() {
        // Branchless ASCII uppercase to lowercase
        *b |= (*b >= b'A' && *b <= b'Z') as u8 * 0x20;
    }
}

/// Parse Content-Length header value quickly
#[inline(always)]
pub fn parse_content_length(value: &[u8]) -> Option<usize> {
    let mut result = 0usize;
    
    for &b in value {
        if b >= b'0' && b <= b'9' {
            result = result.saturating_mul(10).saturating_add((b - b'0') as usize);
        } else if b != b' ' {
            return None;
        }
    }
    
    Some(result)
}

/// Check if connection should be kept alive (fast path)
#[inline(always)]
pub fn should_keep_alive(version: &[u8], connection_header: Option<&[u8]>) -> bool {
    let is_http_11 = version.len() >= 3 && version[2] == b'1';
    
    match connection_header {
        Some(value) => {
            // Fast check for "close"
            !(value.len() >= 5 && 
              (value[0] | 0x20) == b'c' &&
              (value[1] | 0x20) == b'l' &&
              (value[2] | 0x20) == b'o' &&
              (value[3] | 0x20) == b's' &&
              (value[4] | 0x20) == b'e')
        }
        None => is_http_11, // HTTP/1.1 defaults to keep-alive
    }
}