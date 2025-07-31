//! Server-Sent Events (SSE) support for MCP

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::io::Write;
use std::net::TcpStream;
use crate::http::{Request, Response, Status};

/// SSE client connection
pub struct SSEClient {
    /// Client ID
    pub id: String,
    /// TCP stream
    pub stream: TcpStream,
    /// Last message ID
    pub last_message_id: u64,
}

impl SSEClient {
    /// Write an SSE message
    pub fn write_message(&mut self, event: Option<&str>, data: &str) -> std::io::Result<()> {
        self.last_message_id += 1;
        
        // Write SSE format
        write!(self.stream, "id: {}\n", self.last_message_id)?;
        if let Some(event) = event {
            write!(self.stream, "event: {}\n", event)?;
        }
        write!(self.stream, "data: {}\n\n", data)?;
        
        // Flush immediately
        self.stream.flush()
    }
}

/// SSE connection manager
pub struct SSEManager {
    clients: Arc<Mutex<HashMap<String, SSEClient>>>,
}

impl SSEManager {
    /// Create a new SSE manager
    pub fn new() -> Self {
        Self {
            clients: Arc::new(Mutex::new(HashMap::new())),
        }
    }
    
    /// Handle SSE request
    pub fn handle_sse(&self, _request: &Request) -> Response {
        // For now, return a simple response indicating SSE is not yet fully implemented
        Response::new(Status::Ok)
            .header("Content-Type", "text/event-stream")
            .header("Cache-Control", "no-cache")
            .header("Connection", "keep-alive")
            .body("event: connection\ndata: {\"message\": \"SSE endpoint active but not fully implemented yet\"}\n\n")
    }
    
    /// Add a client
    pub fn add_client(&self, id: String, stream: TcpStream) {
        let client = SSEClient {
            id: id.clone(),
            stream,
            last_message_id: 0,
        };
        
        let mut clients = self.clients.lock().unwrap();
        clients.insert(id, client);
    }
    
    /// Remove a client
    pub fn remove_client(&self, id: &str) {
        let mut clients = self.clients.lock().unwrap();
        clients.remove(id);
    }
    
    /// Send message to a specific client
    pub fn send_to_client(&self, client_id: &str, event: Option<&str>, data: &str) -> Result<(), String> {
        let mut clients = self.clients.lock().unwrap();
        
        match clients.get_mut(client_id) {
            Some(client) => {
                client.write_message(event, data)
                    .map_err(|e| format!("Failed to write message: {}", e))
            }
            None => Err(format!("Client not found: {}", client_id)),
        }
    }
    
    /// Broadcast message to all clients
    pub fn broadcast(&self, event: Option<&str>, data: &str) {
        let mut clients = self.clients.lock().unwrap();
        
        // Collect IDs of clients that fail to send
        let mut failed_clients = Vec::new();
        
        for (id, client) in clients.iter_mut() {
            if client.write_message(event, data).is_err() {
                failed_clients.push(id.clone());
            }
        }
        
        // Remove failed clients
        for id in failed_clients {
            clients.remove(&id);
        }
    }
    
    /// Get client count
    pub fn client_count(&self) -> usize {
        let clients = self.clients.lock().unwrap();
        clients.len()
    }
}