//! WebSocket implementation for HyperServe
//! 
//! Zero-dependency WebSocket support following RFC 6455

use std::io::{Read, Write};
use std::net::TcpStream;

/// WebSocket opcodes
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum OpCode {
    /// Continuation frame
    Continuation = 0x0,
    /// Text frame
    Text = 0x1,
    /// Binary frame
    Binary = 0x2,
    /// Connection close
    Close = 0x8,
    /// Ping
    Ping = 0x9,
    /// Pong
    Pong = 0xA,
}

/// WebSocket frame
pub struct Frame {
    /// Is this the final frame?
    pub fin: bool,
    /// Operation code
    pub opcode: OpCode,
    /// Payload data
    pub payload: Vec<u8>,
}

impl Frame {
    /// Create a text frame
    pub fn text(text: &str) -> Self {
        Self {
            fin: true,
            opcode: OpCode::Text,
            payload: text.as_bytes().to_vec(),
        }
    }
    
    /// Create a binary frame
    pub fn binary(data: Vec<u8>) -> Self {
        Self {
            fin: true,
            opcode: OpCode::Binary,
            payload: data,
        }
    }
    
    /// Create a close frame
    pub fn close() -> Self {
        Self {
            fin: true,
            opcode: OpCode::Close,
            payload: Vec::new(),
        }
    }
    
    /// Create a ping frame
    pub fn ping(data: Vec<u8>) -> Self {
        Self {
            fin: true,
            opcode: OpCode::Ping,
            payload: data,
        }
    }
    
    /// Create a pong frame
    pub fn pong(data: Vec<u8>) -> Self {
        Self {
            fin: true,
            opcode: OpCode::Pong,
            payload: data,
        }
    }
    
    /// Write frame to stream
    pub fn write_to<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
        let mut header = vec![];
        
        // First byte: FIN + opcode
        let first = if self.fin { 0x80 } else { 0x00 } | (self.opcode as u8);
        header.push(first);
        
        // Payload length
        let len = self.payload.len();
        if len < 126 {
            header.push(len as u8);
        } else if len < 65536 {
            header.push(126);
            header.extend_from_slice(&(len as u16).to_be_bytes());
        } else {
            header.push(127);
            header.extend_from_slice(&(len as u64).to_be_bytes());
        }
        
        // Write header and payload
        writer.write_all(&header)?;
        writer.write_all(&self.payload)?;
        writer.flush()
    }
    
    /// Read frame from stream
    pub fn read_from<R: Read>(reader: &mut R) -> std::io::Result<Option<Self>> {
        let mut header = [0u8; 2];
        if reader.read_exact(&mut header).is_err() {
            return Ok(None);
        }
        
        let fin = (header[0] & 0x80) != 0;
        let opcode = match header[0] & 0x0F {
            0x0 => OpCode::Continuation,
            0x1 => OpCode::Text,
            0x2 => OpCode::Binary,
            0x8 => OpCode::Close,
            0x9 => OpCode::Ping,
            0xA => OpCode::Pong,
            _ => return Ok(None), // Invalid opcode
        };
        
        let masked = (header[1] & 0x80) != 0;
        let mut payload_len = (header[1] & 0x7F) as usize;
        
        // Extended payload length
        if payload_len == 126 {
            let mut len_bytes = [0u8; 2];
            reader.read_exact(&mut len_bytes)?;
            payload_len = u16::from_be_bytes(len_bytes) as usize;
        } else if payload_len == 127 {
            let mut len_bytes = [0u8; 8];
            reader.read_exact(&mut len_bytes)?;
            payload_len = u64::from_be_bytes(len_bytes) as usize;
        }
        
        // Read mask if present
        let mask = if masked {
            let mut mask_bytes = [0u8; 4];
            reader.read_exact(&mut mask_bytes)?;
            Some(mask_bytes)
        } else {
            None
        };
        
        // Read payload
        let mut payload = vec![0u8; payload_len];
        reader.read_exact(&mut payload)?;
        
        // Unmask payload if needed
        if let Some(mask) = mask {
            for (i, byte) in payload.iter_mut().enumerate() {
                *byte ^= mask[i % 4];
            }
        }
        
        Ok(Some(Frame {
            fin,
            opcode,
            payload,
        }))
    }
}

/// WebSocket connection
pub struct WebSocket {
    stream: TcpStream,
}

impl WebSocket {
    /// Create WebSocket from upgraded connection
    pub fn from_stream(stream: TcpStream) -> Self {
        Self { stream }
    }
    
    /// Send a text message
    pub fn send_text(&mut self, text: &str) -> std::io::Result<()> {
        Frame::text(text).write_to(&mut self.stream)
    }
    
    /// Send binary data
    pub fn send_binary(&mut self, data: Vec<u8>) -> std::io::Result<()> {
        Frame::binary(data).write_to(&mut self.stream)
    }
    
    /// Send ping
    pub fn send_ping(&mut self, data: Vec<u8>) -> std::io::Result<()> {
        Frame::ping(data).write_to(&mut self.stream)
    }
    
    /// Send pong
    pub fn send_pong(&mut self, data: Vec<u8>) -> std::io::Result<()> {
        Frame::pong(data).write_to(&mut self.stream)
    }
    
    /// Close connection
    pub fn close(&mut self) -> std::io::Result<()> {
        Frame::close().write_to(&mut self.stream)
    }
    
    /// Read next frame
    pub fn read_frame(&mut self) -> std::io::Result<Option<Frame>> {
        Frame::read_from(&mut self.stream)
    }
}

/// Perform WebSocket handshake
pub fn handshake(request: &crate::http::Request, mut stream: TcpStream) -> std::io::Result<Option<WebSocket>> {
    // Only handle WebSocket on specific paths
    if request.path != "/ws" {
        return Ok(None);
    }
    
    // Check for WebSocket upgrade headers (case-sensitive)
    let upgrade = request.headers.get("Upgrade")
        .map(|v| v.to_lowercase())
        .unwrap_or_default();
    let connection = request.headers.get("Connection")
        .map(|v| v.to_lowercase())
        .unwrap_or_default();
    let version = request.headers.get("Sec-WebSocket-Version")
        .copied()
        .unwrap_or("");
    let key = request.headers.get("Sec-WebSocket-Key");
    
    if upgrade != "websocket" || !connection.contains("upgrade") || version != "13" || key.is_none() {
        return Ok(None);
    }
    
    // Calculate accept key
    let accept = calculate_accept_key(key.unwrap());
    
    // Send handshake response
    let response = format!(
        "HTTP/1.1 101 Switching Protocols\r\n\
         Upgrade: websocket\r\n\
         Connection: Upgrade\r\n\
         Sec-WebSocket-Accept: {}\r\n\
         \r\n",
        accept
    );
    
    stream.write_all(response.as_bytes())?;
    stream.flush()?;
    
    Ok(Some(WebSocket::from_stream(stream)))
}

/// Calculate WebSocket accept key
fn calculate_accept_key(key: &str) -> String {
        
    const WS_GUID: &str = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";
    let combined = format!("{}{}", key, WS_GUID);
    
    // SHA-1 hash (simple implementation for zero deps)
    let hash = sha1(&combined.as_bytes());
    
    // Base64 encode
    base64_encode(&hash)
}

/// Simple SHA-1 implementation
fn sha1(data: &[u8]) -> [u8; 20] {
    // This is a simplified SHA-1 for the WebSocket handshake
    // In production, you'd want to use a proper crypto library
    
    let mut h0: u32 = 0x67452301;
    let mut h1: u32 = 0xEFCDAB89;
    let mut h2: u32 = 0x98BADCFE;
    let mut h3: u32 = 0x10325476;
    let mut h4: u32 = 0xC3D2E1F0;
    
    // Pre-processing
    let mut msg = data.to_vec();
    let original_len = msg.len();
    msg.push(0x80);
    
    while (msg.len() % 64) != 56 {
        msg.push(0x00);
    }
    
    let bit_len = (original_len as u64) * 8;
    msg.extend_from_slice(&bit_len.to_be_bytes());
    
    // Process chunks
    for chunk in msg.chunks(64) {
        let mut w = [0u32; 80];
        
        // Copy chunk into first 16 words
        for i in 0..16 {
            w[i] = u32::from_be_bytes([
                chunk[i * 4],
                chunk[i * 4 + 1],
                chunk[i * 4 + 2],
                chunk[i * 4 + 3],
            ]);
        }
        
        // Extend the sixteen 32-bit words into eighty 32-bit words
        for i in 16..80 {
            w[i] = (w[i-3] ^ w[i-8] ^ w[i-14] ^ w[i-16]).rotate_left(1);
        }
        
        let mut a = h0;
        let mut b = h1;
        let mut c = h2;
        let mut d = h3;
        let mut e = h4;
        
        // Main loop
        for i in 0..80 {
            let (f, k) = if i < 20 {
                ((b & c) | ((!b) & d), 0x5A827999)
            } else if i < 40 {
                (b ^ c ^ d, 0x6ED9EBA1)
            } else if i < 60 {
                ((b & c) | (b & d) | (c & d), 0x8F1BBCDC)
            } else {
                (b ^ c ^ d, 0xCA62C1D6)
            };
            
            let temp = a.rotate_left(5)
                .wrapping_add(f)
                .wrapping_add(e)
                .wrapping_add(k)
                .wrapping_add(w[i]);
            e = d;
            d = c;
            c = b.rotate_left(30);
            b = a;
            a = temp;
        }
        
        h0 = h0.wrapping_add(a);
        h1 = h1.wrapping_add(b);
        h2 = h2.wrapping_add(c);
        h3 = h3.wrapping_add(d);
        h4 = h4.wrapping_add(e);
    }
    
    let mut result = [0u8; 20];
    result[0..4].copy_from_slice(&h0.to_be_bytes());
    result[4..8].copy_from_slice(&h1.to_be_bytes());
    result[8..12].copy_from_slice(&h2.to_be_bytes());
    result[12..16].copy_from_slice(&h3.to_be_bytes());
    result[16..20].copy_from_slice(&h4.to_be_bytes());
    
    result
}

/// Simple base64 encoder
fn base64_encode(data: &[u8]) -> String {
    const CHARS: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut result = String::new();
    
    for chunk in data.chunks(3) {
        let mut buf = [0u8; 3];
        for (i, &byte) in chunk.iter().enumerate() {
            buf[i] = byte;
        }
        
        let b1 = (buf[0] >> 2) as usize;
        let b2 = (((buf[0] & 0x03) << 4) | (buf[1] >> 4)) as usize;
        let b3 = (((buf[1] & 0x0F) << 2) | (buf[2] >> 6)) as usize;
        let b4 = (buf[2] & 0x3F) as usize;
        
        result.push(CHARS[b1] as char);
        result.push(CHARS[b2] as char);
        
        if chunk.len() > 1 {
            result.push(CHARS[b3] as char);
        } else {
            result.push('=');
        }
        
        if chunk.len() > 2 {
            result.push(CHARS[b4] as char);
        } else {
            result.push('=');
        }
    }
    
    result
}