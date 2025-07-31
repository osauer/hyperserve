//! JSON builder utilities

use super::Value;
use std::collections::HashMap;

/// Object builder for JSON
pub struct ObjectBuilder {
    map: HashMap<String, Value>,
}

impl ObjectBuilder {
    /// Create a new object builder
    pub fn new() -> Self {
        Self {
            map: HashMap::new(),
        }
    }
    
    /// Add a string field
    pub fn string(mut self, key: &str, value: &str) -> Self {
        self.map.insert(key.to_string(), Value::String(value.to_string()));
        self
    }
    
    /// Add a number field
    pub fn number(mut self, key: &str, value: f64) -> Self {
        self.map.insert(key.to_string(), Value::Number(value));
        self
    }
    
    /// Add a boolean field
    pub fn bool(mut self, key: &str, value: bool) -> Self {
        self.map.insert(key.to_string(), Value::Bool(value));
        self
    }
    
    /// Add a null field
    pub fn null(mut self, key: &str) -> Self {
        self.map.insert(key.to_string(), Value::Null);
        self
    }
    
    /// Add an object field
    pub fn object(mut self, key: &str, value: Value) -> Self {
        self.map.insert(key.to_string(), value);
        self
    }
    
    /// Add an array field
    pub fn array(mut self, key: &str, value: Vec<Value>) -> Self {
        self.map.insert(key.to_string(), Value::Array(value));
        self
    }
    
    /// Build the final value
    pub fn build(self) -> Value {
        Value::Object(self.map)
    }
}

/// Create a JSON object
pub fn object() -> ObjectBuilder {
    ObjectBuilder::new()
}

/// Create a JSON array
pub fn array(items: Vec<Value>) -> Value {
    Value::Array(items)
}

/// Create a JSON string
pub fn string(s: &str) -> Value {
    Value::String(s.to_string())
}

/// Create a JSON number
pub fn number(n: f64) -> Value {
    Value::Number(n)
}

/// Create a JSON boolean
pub fn boolean(b: bool) -> Value {
    Value::Bool(b)
}

/// Create a JSON null
pub fn null() -> Value {
    Value::Null
}