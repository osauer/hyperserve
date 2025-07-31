//! MCP tools interface and implementations

use crate::json::{Value, object};
use std::error::Error;

/// MCP tool information
#[derive(Debug, Clone)]
pub struct MCPToolInfo {
    /// Tool name
    pub name: String,
    /// Tool description
    pub description: String,
    /// Input schema
    pub input_schema: Value,
}

/// Trait for MCP tools
pub trait MCPTool: Send + Sync {
    /// Get tool name
    fn name(&self) -> &str;
    
    /// Get tool description
    fn description(&self) -> &str;
    
    /// Get input schema
    fn schema(&self) -> Value;
    
    /// Execute the tool
    fn execute(&self, params: Value) -> Result<Value, Box<dyn Error>>;
}

/// Example echo tool for testing
pub struct EchoTool;

impl MCPTool for EchoTool {
    fn name(&self) -> &str {
        "echo"
    }
    
    fn description(&self) -> &str {
        "Echo back the input message"
    }
    
    fn schema(&self) -> Value {
        object()
            .string("type", "object")
            .object("properties", object()
                .object("message", object()
                    .string("type", "string")
                    .string("description", "Message to echo")
                    .build())
                .build())
            .array("required", vec![Value::String("message".to_string())])
            .build()
    }
    
    fn execute(&self, params: Value) -> Result<Value, Box<dyn Error>> {
        let message = params.get("message")
            .and_then(|v| v.as_str())
            .ok_or("Message parameter is required")?;
        
        Ok(Value::String(message.to_string()))
    }
}

/// Math calculation tool
pub struct CalculatorTool;

impl MCPTool for CalculatorTool {
    fn name(&self) -> &str {
        "calculator"
    }
    
    fn description(&self) -> &str {
        "Perform basic math calculations"
    }
    
    fn schema(&self) -> Value {
        object()
            .string("type", "object")
            .object("properties", object()
                .object("operation", object()
                    .string("type", "string")
                    .array("enum", vec![
                        Value::String("add".to_string()),
                        Value::String("subtract".to_string()),
                        Value::String("multiply".to_string()),
                        Value::String("divide".to_string()),
                    ])
                    .string("description", "Math operation to perform")
                    .build())
                .object("a", object()
                    .string("type", "number")
                    .string("description", "First operand")
                    .build())
                .object("b", object()
                    .string("type", "number")
                    .string("description", "Second operand")
                    .build())
                .build())
            .array("required", vec![
                Value::String("operation".to_string()),
                Value::String("a".to_string()),
                Value::String("b".to_string()),
            ])
            .build()
    }
    
    fn execute(&self, params: Value) -> Result<Value, Box<dyn Error>> {
        let operation = params.get("operation")
            .and_then(|v| v.as_str())
            .ok_or("Operation parameter is required")?;
        
        let a = params.get("a")
            .and_then(|v| v.as_f64())
            .ok_or("Parameter 'a' must be a number")?;
        
        let b = params.get("b")
            .and_then(|v| v.as_f64())
            .ok_or("Parameter 'b' must be a number")?;
        
        let result = match operation {
            "add" => a + b,
            "subtract" => a - b,
            "multiply" => a * b,
            "divide" => {
                if b == 0.0 {
                    return Err("Division by zero".into());
                }
                a / b
            }
            _ => return Err(format!("Unknown operation: {}", operation).into()),
        };
        
        Ok(object()
            .number("result", result)
            .string("operation", operation)
            .number("a", a)
            .number("b", b)
            .build())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_echo_tool() {
        let tool = EchoTool;
        
        let params = object()
            .string("message", "Hello, MCP!")
            .build();
        
        let result = tool.execute(params).unwrap();
        assert_eq!(result, Value::String("Hello, MCP!".to_string()));
    }
    
    #[test]
    fn test_calculator_tool() {
        let tool = CalculatorTool;
        
        // Test addition
        let params = object()
            .string("operation", "add")
            .number("a", 5.0)
            .number("b", 3.0)
            .build();
        
        let result = tool.execute(params).unwrap();
        assert_eq!(result.get("result").and_then(|v| v.as_f64()), Some(8.0));
        
        // Test division by zero
        let params = object()
            .string("operation", "divide")
            .number("a", 10.0)
            .number("b", 0.0)
            .build();
        
        assert!(tool.execute(params).is_err());
    }
}