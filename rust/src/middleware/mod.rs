//! Middleware system for HyperServe
//! 
//! Provides similar middleware to the Go version without external dependencies.

use crate::http::{Request, Response, Status};
use std::sync::Arc;

/// Handler function type
pub type Handler = Arc<dyn Fn(&Request) -> Response + Send + Sync>;

/// Middleware trait
pub trait Middleware: Send + Sync {
    /// Process request, potentially modifying it or the response
    fn process(&self, req: &Request, next: Handler) -> Response;
}

/// Logging middleware
pub struct LoggingMiddleware;

impl Middleware for LoggingMiddleware {
    fn process(&self, req: &Request, next: Handler) -> Response {
        let start = std::time::Instant::now();
        println!("{} {}", req.method.as_str(), req.path);
        
        let response = next(req);
        
        println!("  -> {} in {:?}", 
            response.status as u16, 
            start.elapsed()
        );
        
        response
    }
}

/// Recovery middleware - catches panics
pub struct RecoveryMiddleware;

impl Middleware for RecoveryMiddleware {
    fn process(&self, req: &Request, next: Handler) -> Response {
        use std::panic::{self, AssertUnwindSafe};
        
        // Set a custom panic hook to log the panic
        let method = req.method.as_str();
        let path = req.path;
        
        // Try to catch the panic
        let result = panic::catch_unwind(AssertUnwindSafe(|| {
            // Call the handler - if it panics, we'll catch it
            let handler = AssertUnwindSafe(&next);
            let request = AssertUnwindSafe(req);
            (handler.0)(request.0)
        }));
        
        match result {
            Ok(response) => response,
            Err(_) => {
                eprintln!("Panic recovered while handling {} {}", method, path);
                Response::internal_error()
            }
        }
    }
}

/// Security headers middleware
pub struct SecurityHeadersMiddleware {
    pub csp: Option<String>,
    pub frame_options: &'static str,
}

impl Default for SecurityHeadersMiddleware {
    fn default() -> Self {
        Self {
            csp: None,
            frame_options: "DENY",
        }
    }
}

impl Middleware for SecurityHeadersMiddleware {
    fn process(&self, req: &Request, next: Handler) -> Response {
        let mut response = next(req);
        
        // Add security headers
        response.add_header("X-Content-Type-Options", "nosniff");
        response.add_header("X-Frame-Options", self.frame_options);
        response.add_header("X-XSS-Protection", "1; mode=block");
        response.add_header("Referrer-Policy", "strict-origin-when-cross-origin");
            
        if let Some(csp) = &self.csp {
            response.add_header("Content-Security-Policy", csp.clone());
        }
        
        response
    }
}

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};

/// Middleware chain builder
#[derive(Clone)]
pub struct Chain {
    middlewares: Vec<Arc<dyn Middleware>>,
}

impl Chain {
    pub fn new() -> Self {
        Self {
            middlewares: Vec::new(),
        }
    }
    
    pub fn add<M: Middleware + 'static>(mut self, middleware: M) -> Self {
        self.middlewares.push(Arc::new(middleware));
        self
    }
    
    pub fn wrap(self, handler: Handler) -> Handler {
        let mut final_handler = handler;
        
        // Apply middleware in reverse order
        for middleware in self.middlewares.into_iter().rev() {
            let next = final_handler.clone();
            let mw = middleware.clone();
            final_handler = Arc::new(move |req: &Request| {
                mw.process(req, next.clone())
            });
        }
        
        final_handler
    }
}

/// Rate limiting middleware

pub struct RateLimitMiddleware {
    requests: Arc<Mutex<HashMap<String, Vec<Instant>>>>,
    max_requests: usize,
    window: Duration,
}

impl RateLimitMiddleware {
    pub fn new(max_requests: usize, window: Duration) -> Self {
        Self {
            requests: Arc::new(Mutex::new(HashMap::new())),
            max_requests,
            window,
        }
    }
    
    fn get_client_key(req: &Request) -> String {
        // Simple IP-based key from X-Forwarded-For or remote addr
        req.headers.get("x-forwarded-for")
            .or_else(|| req.headers.get("x-real-ip"))
            .map(|s| s.to_string())
            .unwrap_or_else(|| "unknown".to_string())
    }
}

impl Middleware for RateLimitMiddleware {
    fn process(&self, req: &Request, next: Handler) -> Response {
        let key = Self::get_client_key(req);
        let now = Instant::now();
        
        let mut requests = self.requests.lock().unwrap();
        let client_requests = requests.entry(key).or_insert_with(Vec::new);
        
        // Remove old requests outside window
        client_requests.retain(|&t| now.duration_since(t) < self.window);
        
        // Check rate limit
        if client_requests.len() >= self.max_requests {
            return Response::new(Status::TooManyRequests)
                .header("Retry-After", "60")
                .body("Rate limit exceeded");
        }
        
        // Record this request
        client_requests.push(now);
        drop(requests);
        
        next(req)
    }
}

