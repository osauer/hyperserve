//! HyperServe - Zero-dependency HTTP server framework
//! 
//! A lightweight, high-performance HTTP server with Model Context Protocol support.
//! Built from scratch using Rust 2024 edition features.

#![warn(missing_docs)]

pub mod http;
pub mod middleware;
pub mod builder;
pub mod websocket;
pub mod json;
#[cfg(feature = "mcp")]
pub mod mcp;

use std::net::TcpListener;
use std::sync::Arc;

pub use http::{Method, Status, Request, Response, Router};
pub use middleware::{Middleware, Chain, LoggingMiddleware, RecoveryMiddleware, 
    SecurityHeadersMiddleware, RateLimitMiddleware};
pub use builder::ServerBuilder;

/// The main HyperServe server struct
pub struct Server {
    listener: TcpListener,
    router: Arc<Router>,
    middleware_chain: Option<Chain>,
    /// Use optimized thread pool (experimental)
    use_optimized_pool: bool,
}

impl Server {
    /// Create a new server bound to the given address
    /// 
    /// # Examples
    /// ```no_run
    /// use hyperserve::Server;
    /// 
    /// let server = Server::new("127.0.0.1:8080").unwrap();
    /// ```
    pub fn new(addr: &str) -> std::io::Result<Self> {
        Ok(Self {
            listener: TcpListener::bind(addr)?,
            router: Arc::new(Router::new()),
            middleware_chain: None,
            use_optimized_pool: false,
        })
    }
    
    /// Add a handler function for GET requests
    pub fn handle_func(self, path: &str, handler: fn(&Request) -> Response) -> Self {
        self.handle(Method::GET, path, handler)
    }
    
    /// Add a handler for a specific HTTP method and path
    pub fn handle(mut self, method: Method, path: &str, handler: fn(&Request) -> Response) -> Self {
        Arc::get_mut(&mut self.router)
            .unwrap()
            .route(method, path, handler);
        self
    }
    
    /// Add middleware to the server
    pub fn with_middleware<M: Middleware + 'static>(mut self, middleware: M) -> Self {
        if self.middleware_chain.is_none() {
            self.middleware_chain = Some(Chain::new());
        }
        self.middleware_chain = Some(
            self.middleware_chain.unwrap().add(middleware)
        );
        self
    }
    
    /// Enable experimental optimized thread pool (default: false)
    /// 
    /// WARNING: This is experimental and may cause instability.
    /// Only enable if you need maximum performance and can tolerate potential issues.
    pub fn with_optimized_pool(mut self, enabled: bool) -> Self {
        self.use_optimized_pool = enabled;
        self
    }
    
    /// Run the server - blocks until shutdown
    pub fn run(mut self) -> std::io::Result<()> {
        self.print_banner();
        
        println!("Server listening on {}", self.listener.local_addr()?);
        
        // Create thread pool based on configuration
        let pool = if self.use_optimized_pool {
            println!("WARNING: Using experimental optimized thread pool");
            let config = http::ThreadPoolConfig::optimal();
            println!("Thread pool: {} min, {} target, {} max threads", 
                     config.min_threads, config.target_threads, config.max_threads);
            
            // Create optimized pool (experimental)
            // Note: This may cause segmentation faults under certain conditions
            // let pool = http::OptimizedPool::new(config.min_threads, config.max_threads);
            
            // For now, still use stable pool even when optimized is requested
            // This prevents crashes while we debug the lock-free implementation
            println!("Note: Optimized pool is temporarily disabled due to stability issues");
            http::ThreadPool::new(config.target_threads)
        } else {
            // Use stable thread pool (default)
            http::ThreadPool::new(4)
        };
        
        // Apply middleware chain to router if configured
        if let Some(chain) = self.middleware_chain {
            if let Some(router_mut) = Arc::get_mut(&mut self.router) {
                router_mut.set_middleware(chain);
            }
        }
        
        let router = self.router;
        
        for stream in self.listener.incoming() {
            let stream = stream?;
            let router = router.clone();
            
            pool.execute(move || {
                http::handle_connection(stream, &router);
            });
        }
        
        Ok(())
    }
    
    fn print_banner(&self) {
        println!(r#"
 _                                              
| |__  _   _ _ __   ___ _ __ ___  ___ _ ____   _____
| '_ \| | | | '_ \ / _ \ '__/ __|/ _ \ '__\ \ / / _ \
| | | | |_| | |_) |  __/ |  \__ \  __/ |   \ V /  __/
|_| |_|\__, | .__/ \___|_|  |___/\___|_|    \_/ \___|
       |___/|_|     Rust Edition
        "#);
    }
}

/// Re-export common HTTP methods
pub use Method::{GET, POST, PUT, DELETE};

// For tests
#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_server_creation() {
        let server = Server::new("127.0.0.1:0").unwrap();
        // Just test that we can create a server
    }
}
