//! Server builder with middleware support

use crate::{Server, Router, Middleware, Chain};
use std::net::TcpListener;
use std::sync::Arc;

/// Server builder
pub struct ServerBuilder {
    addr: String,
    router: Router,
    middleware_chain: Chain,
    use_optimized_pool: bool,
}

impl ServerBuilder {
    /// Create new server builder
    pub fn new(addr: impl Into<String>) -> Self {
        Self {
            addr: addr.into(),
            router: Router::new(),
            middleware_chain: Chain::new(),
            use_optimized_pool: false,
        }
    }
    
    /// Add middleware
    pub fn middleware<M: Middleware + 'static>(mut self, middleware: M) -> Self {
        self.middleware_chain = self.middleware_chain.add(middleware);
        self
    }
    
    /// Add route handler
    pub fn route(mut self, method: crate::Method, path: &str, handler: impl Fn(&crate::Request) -> crate::Response + Send + Sync + 'static) -> Self {
        self.router.route(method, path, handler);
        self
    }
    
    /// Enable experimental optimized thread pool (default: false)
    /// 
    /// WARNING: This is experimental and may cause instability.
    pub fn with_optimized_pool(mut self, enabled: bool) -> Self {
        self.use_optimized_pool = enabled;
        self
    }
    
    /// Build the server
    pub fn build(mut self) -> std::io::Result<Server> {
        // Apply middleware to router
        self.router.set_middleware(self.middleware_chain);
        
        Ok(Server {
            listener: TcpListener::bind(&self.addr)?,
            router: Arc::new(self.router),
            middleware_chain: None,
            use_optimized_pool: self.use_optimized_pool,
        })
    }
}