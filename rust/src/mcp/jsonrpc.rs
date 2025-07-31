//! JSON-RPC 2.0 implementation for MCP

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use crate::json::{self, Value};

/// JSON-RPC version
pub const JSONRPC_VERSION: &str = "2.0";

/// JSON-RPC request
#[derive(Debug, Clone)]
pub struct JsonRpcRequest {
    /// JSON-RPC version
    pub jsonrpc: String,
    /// Method name
    pub method: String,
    /// Method parameters
    pub params: Option<Value>,
    /// Request ID
    pub id: Option<Value>,
}

impl JsonRpcRequest {
    /// Parse from JSON value
    pub fn from_json(value: Value) -> Result<Self, String> {
        let obj = value.as_object().ok_or("Request must be an object")?;
        
        let jsonrpc = obj.get("jsonrpc")
            .and_then(|v| v.as_str())
            .ok_or("Missing jsonrpc field")?
            .to_string();
            
        let method = obj.get("method")
            .and_then(|v| v.as_str())
            .ok_or("Missing method field")?
            .to_string();
            
        let params = obj.get("params").cloned();
        let id = obj.get("id").cloned();
        
        Ok(Self {
            jsonrpc,
            method,
            params,
            id,
        })
    }
    
    /// Convert to JSON value
    pub fn to_json(&self) -> Value {
        let mut obj = HashMap::new();
        obj.insert("jsonrpc".to_string(), Value::String(self.jsonrpc.clone()));
        obj.insert("method".to_string(), Value::String(self.method.clone()));
        
        if let Some(params) = &self.params {
            obj.insert("params".to_string(), params.clone());
        }
        
        if let Some(id) = &self.id {
            obj.insert("id".to_string(), id.clone());
        }
        
        Value::Object(obj)
    }
}

/// JSON-RPC response
#[derive(Debug, Clone)]
pub struct JsonRpcResponse {
    /// JSON-RPC version
    pub jsonrpc: String,
    /// Result (if success)
    pub result: Option<Value>,
    /// Error (if failed)
    pub error: Option<JsonRpcError>,
    /// Request ID
    pub id: Option<Value>,
}

/// JSON-RPC error
#[derive(Debug, Clone)]
pub struct JsonRpcError {
    /// Error code
    pub code: i32,
    /// Error message
    pub message: String,
    /// Additional error data
    pub data: Option<Value>,
}

impl JsonRpcResponse {
    /// Create a success response
    pub fn success(id: Option<Value>, result: Value) -> Self {
        Self {
            jsonrpc: JSONRPC_VERSION.to_string(),
            result: Some(result),
            error: None,
            id,
        }
    }
    
    /// Create an error response
    pub fn error(id: Option<Value>, error: JsonRpcError) -> Self {
        Self {
            jsonrpc: JSONRPC_VERSION.to_string(),
            result: None,
            error: Some(error),
            id,
        }
    }
    
    /// Convert to JSON value
    pub fn to_json(&self) -> Value {
        let mut obj = HashMap::new();
        obj.insert("jsonrpc".to_string(), Value::String(self.jsonrpc.clone()));
        
        if let Some(result) = &self.result {
            obj.insert("result".to_string(), result.clone());
        }
        
        if let Some(error) = &self.error {
            let mut error_obj = HashMap::new();
            error_obj.insert("code".to_string(), Value::Number(error.code as f64));
            error_obj.insert("message".to_string(), Value::String(error.message.clone()));
            if let Some(data) = &error.data {
                error_obj.insert("data".to_string(), data.clone());
            }
            obj.insert("error".to_string(), Value::Object(error_obj));
        }
        
        if let Some(id) = &self.id {
            obj.insert("id".to_string(), id.clone());
        } else {
            obj.insert("id".to_string(), Value::Null);
        }
        
        Value::Object(obj)
    }
}

/// Type alias for method handler
pub type MethodHandler = Arc<dyn Fn(Option<Value>) -> Result<Value, JsonRpcError> + Send + Sync>;

/// JSON-RPC engine for processing requests
pub struct JsonRpcEngine {
    methods: Arc<Mutex<HashMap<String, MethodHandler>>>,
}

impl JsonRpcEngine {
    /// Create a new JSON-RPC engine
    pub fn new() -> Self {
        Self {
            methods: Arc::new(Mutex::new(HashMap::new())),
        }
    }
    
    /// Register a method handler
    pub fn register_method<F>(&self, name: &str, handler: F)
    where
        F: Fn(Option<Value>) -> Result<Value, JsonRpcError> + Send + Sync + 'static,
    {
        let mut methods = self.methods.lock().unwrap();
        methods.insert(name.to_string(), Arc::new(handler));
    }
    
    /// Process a JSON-RPC request
    pub fn process_request(&self, request: &JsonRpcRequest) -> JsonRpcResponse {
        // Validate JSON-RPC version
        if request.jsonrpc != JSONRPC_VERSION {
            return JsonRpcResponse::error(
                request.id.clone(),
                JsonRpcError {
                    code: -32600,
                    message: "Invalid Request".to_string(),
                    data: Some(Value::String(format!("Expected jsonrpc version {}", JSONRPC_VERSION))),
                },
            );
        }
        
        // Find method handler
        let methods = self.methods.lock().unwrap();
        let handler = match methods.get(&request.method) {
            Some(h) => Arc::clone(h),
            None => {
                return JsonRpcResponse::error(
                    request.id.clone(),
                    JsonRpcError {
                        code: -32601,
                        message: "Method not found".to_string(),
                        data: Some(Value::String(format!("Method '{}' not found", request.method))),
                    },
                );
            }
        };
        
        // Drop the lock before calling the handler
        drop(methods);
        
        // Call method handler
        match handler(request.params.clone()) {
            Ok(result) => JsonRpcResponse::success(request.id.clone(), result),
            Err(error) => JsonRpcResponse::error(request.id.clone(), error),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_jsonrpc_request_parsing() {
        let json_str = r#"{
            "jsonrpc": "2.0",
            "method": "test",
            "params": {"foo": "bar"},
            "id": 1
        }"#;
        
        let value = json::parse(json_str).unwrap();
        let request = JsonRpcRequest::from_json(value).unwrap();
        
        assert_eq!(request.jsonrpc, "2.0");
        assert_eq!(request.method, "test");
        assert!(request.id.is_some());
    }
    
    #[test]
    fn test_jsonrpc_engine() {
        let engine = JsonRpcEngine::new();
        
        // Register a test method
        engine.register_method("echo", |params| {
            Ok(params.unwrap_or(Value::Null))
        });
        
        // Test successful call
        let request = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            method: "echo".to_string(),
            params: Some(Value::String("hello".to_string())),
            id: Some(Value::Number(1.0)),
        };
        
        let response = engine.process_request(&request);
        assert_eq!(response.result, Some(Value::String("hello".to_string())));
        assert!(response.error.is_none());
        
        // Test method not found
        let request = JsonRpcRequest {
            jsonrpc: "2.0".to_string(),
            method: "unknown".to_string(),
            params: None,
            id: Some(Value::Number(2.0)),
        };
        
        let response = engine.process_request(&request);
        assert!(response.result.is_none());
        assert!(response.error.is_some());
        assert_eq!(response.error.unwrap().code, -32601);
    }
}