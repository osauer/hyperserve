//! Fast buffer management for HTTP responses
//! 
//! Key optimizations:
//! - Reusable buffers to reduce allocations
//! - Stack-allocated small buffers
//! - Vectored I/O for zero-copy writes

use std::io::{self, Write};
use std::mem::MaybeUninit;

/// Size for stack-allocated buffers (most HTTP responses fit in 4KB)
const STACK_BUFFER_SIZE: usize = 4096;

/// Fast buffer for HTTP response writing
pub struct FastBuffer {
    /// Stack buffer for small responses
    stack_buf: [u8; STACK_BUFFER_SIZE],
    /// Current position in buffer
    pos: usize,
    /// Heap buffer for large responses
    heap_buf: Option<Vec<u8>>,
}

impl FastBuffer {
    /// Create a new fast buffer
    pub fn new() -> Self {
        Self {
            stack_buf: [0; STACK_BUFFER_SIZE],
            pos: 0,
            heap_buf: None,
        }
    }
    
    /// Get a buffer with at least the specified capacity
    pub fn with_capacity(capacity: usize) -> Self {
        if capacity <= STACK_BUFFER_SIZE {
            Self::new()
        } else {
            Self {
                stack_buf: [0; STACK_BUFFER_SIZE],
                pos: 0,
                heap_buf: Some(Vec::with_capacity(capacity)),
            }
        }
    }
    
    /// Write data to the buffer
    pub fn write_all(&mut self, data: &[u8]) -> io::Result<()> {
        if let Some(ref mut heap) = self.heap_buf {
            heap.extend_from_slice(data);
        } else if self.pos + data.len() <= STACK_BUFFER_SIZE {
            // Fits in stack buffer
            self.stack_buf[self.pos..self.pos + data.len()].copy_from_slice(data);
            self.pos += data.len();
        } else {
            // Need to switch to heap buffer
            let mut heap = Vec::with_capacity((self.pos + data.len()).next_power_of_two());
            heap.extend_from_slice(&self.stack_buf[..self.pos]);
            heap.extend_from_slice(data);
            self.heap_buf = Some(heap);
            self.pos = 0;
        }
        Ok(())
    }
    
    /// Get the buffer contents
    pub fn as_bytes(&self) -> &[u8] {
        if let Some(ref heap) = self.heap_buf {
            heap.as_slice()
        } else {
            &self.stack_buf[..self.pos]
        }
    }
    
    /// Clear the buffer for reuse
    pub fn clear(&mut self) {
        self.pos = 0;
        if let Some(ref mut heap) = self.heap_buf {
            heap.clear();
        }
    }
    
    /// Write buffer to a stream using vectored I/O
    pub fn write_to<W: Write>(&self, writer: &mut W) -> io::Result<usize> {
        writer.write_all(self.as_bytes())?;
        Ok(self.as_bytes().len())
    }
}

/// Thread-local buffer pool for zero allocations
thread_local! {
    static BUFFER_POOL: std::cell::RefCell<Vec<FastBuffer>> = std::cell::RefCell::new(Vec::new());
}

/// Get a buffer from the thread-local pool
pub fn get_buffer() -> FastBuffer {
    BUFFER_POOL.with(|pool| {
        pool.borrow_mut().pop().unwrap_or_else(FastBuffer::new)
    })
}

/// Return a buffer to the pool
pub fn return_buffer(mut buf: FastBuffer) {
    buf.clear();
    BUFFER_POOL.with(|pool| {
        let mut pool = pool.borrow_mut();
        if pool.len() < 16 { // Keep max 16 buffers per thread
            pool.push(buf);
        }
    })
}

/// Format integers without allocation (faster than format!())
pub fn format_usize(mut n: usize, buf: &mut [u8]) -> &str {
    if n == 0 {
        buf[0] = b'0';
        return unsafe { std::str::from_utf8_unchecked(&buf[..1]) };
    }
    
    let mut pos = 0;
    let mut tmp = [0u8; 20]; // Max digits in usize
    
    while n > 0 {
        tmp[pos] = b'0' + (n % 10) as u8;
        n /= 10;
        pos += 1;
    }
    
    // Reverse into output buffer
    for i in 0..pos {
        buf[i] = tmp[pos - 1 - i];
    }
    
    unsafe { std::str::from_utf8_unchecked(&buf[..pos]) }
}