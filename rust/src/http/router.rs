//! HTTP request router

use super::{Method, Request, Response};
use crate::middleware::Chain;
use std::collections::HashMap;
use std::sync::Arc;

/// Handler function type
pub type Handler = Arc<dyn Fn(&Request) -> Response + Send + Sync + 'static>;

/// Simple HTTP router
pub struct Router {
    routes: HashMap<(Method, String), Handler>,
    middleware_chain: Option<Chain>,
}

impl Router {
    /// Create new router
    pub fn new() -> Self {
        Self {
            routes: HashMap::new(),
            middleware_chain: None,
        }
    }
    
    /// Set middleware chain
    pub fn set_middleware(&mut self, chain: Chain) {
        self.middleware_chain = Some(chain);
    }
    
    /// Add route handler
    pub fn route(&mut self, method: Method, path: &str, handler: impl Fn(&Request) -> Response + Send + Sync + 'static) {
        self.routes.insert(
            (method, path.to_string()), 
            Arc::new(handler)
        );
    }
    
    /// Handle request
    pub fn handle(&self, request: &Request) -> Response {
        // Find handler
        let handler = if let Some(handler) = self.routes.get(&(request.method, request.path.to_string())) {
            handler.clone()
        } else {
            // Not found handler
            Arc::new(|_: &Request| Response::not_found()) as Handler
        };
        
        // Apply middleware if configured
        if let Some(ref chain) = self.middleware_chain {
            let wrapped = chain.clone().wrap(handler);
            wrapped(request)
        } else {
            handler(request)
        }
    }
}

// Convenience methods using Rust 2024 features
impl Router {
    /// Add GET route
    pub fn get(&mut self, path: &str, handler: impl Fn(&Request) -> Response + Send + Sync + 'static) {
        self.route(Method::GET, path, handler);
    }
    
    /// Add POST route
    pub fn post(&mut self, path: &str, handler: impl Fn(&Request) -> Response + Send + Sync + 'static) {
        self.route(Method::POST, path, handler);
    }
    
    /// Add PUT route
    pub fn put(&mut self, path: &str, handler: impl Fn(&Request) -> Response + Send + Sync + 'static) {
        self.route(Method::PUT, path, handler);
    }
    
    /// Add DELETE route
    pub fn delete(&mut self, path: &str, handler: impl Fn(&Request) -> Response + Send + Sync + 'static) {
        self.route(Method::DELETE, path, handler);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_router_exact_match() {
        let mut router = Router::new();
        router.get("/", |_| Response::ok().body("Home"));
        router.post("/users", |_| Response::ok().body("Create user"));
        
        // Test GET /
        let req = Request {
            method: Method::GET,
            path: "/",
            version: "HTTP/1.1",
            headers: HashMap::new(),
            body: b"",
        };
        
        let _resp = router.handle(&req);
        // Response created successfully
    }
}