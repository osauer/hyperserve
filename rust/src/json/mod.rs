//! Minimal JSON implementation for MCP support (zero dependencies)

use std::collections::HashMap;

pub mod builder;
pub use builder::{object, array, string, number, boolean, null};

/// JSON value type
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    /// Null value
    Null,
    /// Boolean value
    Bool(bool),
    /// Number value (stored as f64)
    Number(f64),
    /// String value
    String(String),
    /// Array value
    Array(Vec<Value>),
    /// Object value
    Object(HashMap<String, Value>),
}

impl Value {
    /// Create a null value
    pub fn null() -> Self {
        Value::Null
    }
    
    /// Get as string if possible
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Value::String(s) => Some(s),
            _ => None,
        }
    }
    
    /// Get as f64 if possible
    pub fn as_f64(&self) -> Option<f64> {
        match self {
            Value::Number(n) => Some(*n),
            _ => None,
        }
    }
    
    /// Get as bool if possible
    pub fn as_bool(&self) -> Option<bool> {
        match self {
            Value::Bool(b) => Some(*b),
            _ => None,
        }
    }
    
    /// Get as object if possible
    pub fn as_object(&self) -> Option<&HashMap<String, Value>> {
        match self {
            Value::Object(o) => Some(o),
            _ => None,
        }
    }
    
    /// Get as array if possible
    pub fn as_array(&self) -> Option<&Vec<Value>> {
        match self {
            Value::Array(a) => Some(a),
            _ => None,
        }
    }
    
    /// Index into object
    pub fn get(&self, key: &str) -> Option<&Value> {
        match self {
            Value::Object(map) => map.get(key),
            _ => None,
        }
    }
    
    /// Convert to JSON string
    pub fn to_string(&self) -> String {
        match self {
            Value::Null => "null".to_string(),
            Value::Bool(b) => b.to_string(),
            Value::Number(n) => {
                // Handle integers specially
                if n.fract() == 0.0 && *n >= i64::MIN as f64 && *n <= i64::MAX as f64 {
                    format!("{}", *n as i64)
                } else {
                    n.to_string()
                }
            }
            Value::String(s) => format!("\"{}\"", escape_json_string(s)),
            Value::Array(arr) => {
                let items: Vec<String> = arr.iter().map(|v| v.to_string()).collect();
                format!("[{}]", items.join(","))
            }
            Value::Object(obj) => {
                let items: Vec<String> = obj.iter()
                    .map(|(k, v)| format!("\"{}\":{}", escape_json_string(k), v.to_string()))
                    .collect();
                format!("{{{}}}", items.join(","))
            }
        }
    }
}

/// Escape a string for JSON
fn escape_json_string(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    for ch in s.chars() {
        match ch {
            '"' => result.push_str("\\\""),
            '\\' => result.push_str("\\\\"),
            '\n' => result.push_str("\\n"),
            '\r' => result.push_str("\\r"),
            '\t' => result.push_str("\\t"),
            ch if ch.is_control() => {
                result.push_str(&format!("\\u{:04x}", ch as u32));
            }
            ch => result.push(ch),
        }
    }
    result
}

/// Convenience macro for creating JSON objects
#[macro_export]
macro_rules! json {
    // Null
    (null) => {
        $crate::json::Value::Null
    };
    
    // Bool
    (true) => {
        $crate::json::Value::Bool(true)
    };
    (false) => {
        $crate::json::Value::Bool(false)
    };
    
    // Number
    ($n:expr) => {{
        $crate::json::Value::Number($n as f64)
    }};
    
    // String
    ($s:expr) => {{
        $crate::json::Value::String($s.to_string())
    }};
    
    // Array
    ([$($item:tt),* $(,)?]) => {{
        $crate::json::Value::Array(vec![$(json!($item)),*])
    }};
    
    // Object
    ({$($key:tt : $value:tt),* $(,)?}) => {{
        let mut map = std::collections::HashMap::new();
        $(
            map.insert($key.to_string(), json!($value));
        )*
        $crate::json::Value::Object(map)
    }};
}

/// Simple JSON parser (minimal implementation)
pub fn parse(input: &str) -> Result<Value, String> {
    Parser::new(input).parse_value()
}

struct Parser<'a> {
    input: &'a str,
    pos: usize,
}

impl<'a> Parser<'a> {
    fn new(input: &'a str) -> Self {
        Self { input, pos: 0 }
    }
    
    fn skip_whitespace(&mut self) {
        while self.pos < self.input.len() {
            match self.input.as_bytes()[self.pos] {
                b' ' | b'\t' | b'\n' | b'\r' => self.pos += 1,
                _ => break,
            }
        }
    }
    
    fn peek(&self) -> Option<u8> {
        self.input.as_bytes().get(self.pos).copied()
    }
    
    fn consume(&mut self) -> Option<u8> {
        if self.pos < self.input.len() {
            let ch = self.input.as_bytes()[self.pos];
            self.pos += 1;
            Some(ch)
        } else {
            None
        }
    }
    
    fn parse_value(&mut self) -> Result<Value, String> {
        self.skip_whitespace();
        
        match self.peek() {
            Some(b'"') => self.parse_string(),
            Some(b'{') => self.parse_object(),
            Some(b'[') => self.parse_array(),
            Some(b't') | Some(b'f') => self.parse_bool(),
            Some(b'n') => self.parse_null(),
            Some(ch) if (ch as char).is_numeric() || ch == b'-' => self.parse_number(),
            _ => Err("Invalid JSON value".to_string()),
        }
    }
    
    fn parse_string(&mut self) -> Result<Value, String> {
        self.consume(); // Skip opening quote
        let start = self.pos;
        
        while let Some(ch) = self.peek() {
            if ch == b'"' && self.input.as_bytes().get(self.pos - 1) != Some(&b'\\') {
                let s = self.input[start..self.pos].to_string();
                self.consume(); // Skip closing quote
                return Ok(Value::String(s));
            }
            self.consume();
        }
        
        Err("Unterminated string".to_string())
    }
    
    fn parse_number(&mut self) -> Result<Value, String> {
        let start = self.pos;
        
        // Handle negative sign
        if self.peek() == Some(b'-') {
            self.consume();
        }
        
        // Parse digits
        while let Some(ch) = self.peek() {
            if (ch as char).is_numeric() || ch == b'.' || ch == b'e' || ch == b'E' || ch == b'+' || ch == b'-' {
                self.consume();
            } else {
                break;
            }
        }
        
        let num_str = &self.input[start..self.pos];
        num_str.parse::<f64>()
            .map(Value::Number)
            .map_err(|_| "Invalid number".to_string())
    }
    
    fn parse_bool(&mut self) -> Result<Value, String> {
        if self.input[self.pos..].starts_with("true") {
            self.pos += 4;
            Ok(Value::Bool(true))
        } else if self.input[self.pos..].starts_with("false") {
            self.pos += 5;
            Ok(Value::Bool(false))
        } else {
            Err("Invalid boolean".to_string())
        }
    }
    
    fn parse_null(&mut self) -> Result<Value, String> {
        if self.input[self.pos..].starts_with("null") {
            self.pos += 4;
            Ok(Value::Null)
        } else {
            Err("Invalid null".to_string())
        }
    }
    
    fn parse_array(&mut self) -> Result<Value, String> {
        self.consume(); // Skip [
        let mut items = Vec::new();
        
        self.skip_whitespace();
        
        // Empty array
        if self.peek() == Some(b']') {
            self.consume();
            return Ok(Value::Array(items));
        }
        
        loop {
            items.push(self.parse_value()?);
            self.skip_whitespace();
            
            match self.consume() {
                Some(b',') => {
                    self.skip_whitespace();
                    continue;
                }
                Some(b']') => break,
                _ => return Err("Expected ',' or ']' in array".to_string()),
            }
        }
        
        Ok(Value::Array(items))
    }
    
    fn parse_object(&mut self) -> Result<Value, String> {
        self.consume(); // Skip {
        let mut map = HashMap::new();
        
        self.skip_whitespace();
        
        // Empty object
        if self.peek() == Some(b'}') {
            self.consume();
            return Ok(Value::Object(map));
        }
        
        loop {
            self.skip_whitespace();
            
            // Parse key
            let key = match self.parse_string()? {
                Value::String(s) => s,
                _ => return Err("Expected string key".to_string()),
            };
            
            self.skip_whitespace();
            
            // Expect colon
            if self.consume() != Some(b':') {
                return Err("Expected ':' after key".to_string());
            }
            
            self.skip_whitespace();
            
            // Parse value
            let value = self.parse_value()?;
            map.insert(key, value);
            
            self.skip_whitespace();
            
            match self.consume() {
                Some(b',') => continue,
                Some(b'}') => break,
                _ => return Err("Expected ',' or '}' in object".to_string()),
            }
        }
        
        Ok(Value::Object(map))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_json_macro() {
        let obj = json!({
            "name": "test",
            "age": 42,
            "active": true,
            "items": ["a", "b", "c"]
        });
        
        assert_eq!(obj.get("name").and_then(|v| v.as_str()), Some("test"));
        assert_eq!(obj.get("age").and_then(|v| v.as_f64()), Some(42.0));
        assert_eq!(obj.get("active").and_then(|v| v.as_bool()), Some(true));
    }
    
    #[test]
    fn test_json_parse() {
        let input = r#"{"name": "test", "value": 123, "items": [1, 2, 3]}"#;
        let parsed = parse(input).unwrap();
        
        assert_eq!(parsed.get("name").and_then(|v| v.as_str()), Some("test"));
        assert_eq!(parsed.get("value").and_then(|v| v.as_f64()), Some(123.0));
        
        let items = parsed.get("items").and_then(|v| v.as_array()).unwrap();
        assert_eq!(items.len(), 3);
    }
    
    #[test]
    fn test_json_to_string() {
        let obj = json!({
            "message": "Hello, \"world\"!",
            "number": 42,
            "float": 3.14,
            "null": null,
            "bool": true
        });
        
        let s = obj.to_string();
        assert!(s.contains("\"message\":\"Hello, \\\"world\\\"!\""));
        assert!(s.contains("\"number\":42"));
        assert!(s.contains("\"bool\":true"));
    }
}