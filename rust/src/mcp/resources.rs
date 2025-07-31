//! MCP resources interface and implementations

use crate::json::{Value, object};
use std::error::Error;

/// MCP resource information
#[derive(Debug, Clone)]
pub struct MCPResourceInfo {
    /// Resource URI
    pub uri: String,
    /// Resource name
    pub name: String,
    /// Resource description
    pub description: String,
    /// MIME type
    pub mime_type: String,
}

/// Trait for MCP resources
pub trait MCPResource: Send + Sync {
    /// Get resource URI
    fn uri(&self) -> &str;
    
    /// Get resource name
    fn name(&self) -> &str;
    
    /// Get resource description
    fn description(&self) -> &str;
    
    /// Get MIME type
    fn mime_type(&self) -> &str;
    
    /// Read the resource content
    fn read(&self) -> Result<Value, Box<dyn Error>>;
    
    /// List sub-resources (optional)
    fn list(&self) -> Result<Vec<String>, Box<dyn Error>> {
        Ok(vec![])
    }
}

/// Example static resource for testing
pub struct StaticResource {
    uri: String,
    name: String,
    description: String,
    content: String,
}

impl StaticResource {
    /// Create a new static resource
    pub fn new(uri: String, name: String, description: String, content: String) -> Self {
        Self {
            uri,
            name,
            description,
            content,
        }
    }
}

impl MCPResource for StaticResource {
    fn uri(&self) -> &str {
        &self.uri
    }
    
    fn name(&self) -> &str {
        &self.name
    }
    
    fn description(&self) -> &str {
        &self.description
    }
    
    fn mime_type(&self) -> &str {
        "text/plain"
    }
    
    fn read(&self) -> Result<Value, Box<dyn Error>> {
        Ok(Value::String(self.content.clone()))
    }
}

/// System information resource
pub struct SystemInfoResource;

impl MCPResource for SystemInfoResource {
    fn uri(&self) -> &str {
        "system://info"
    }
    
    fn name(&self) -> &str {
        "System Information"
    }
    
    fn description(&self) -> &str {
        "Basic system information"
    }
    
    fn mime_type(&self) -> &str {
        "application/json"
    }
    
    fn read(&self) -> Result<Value, Box<dyn Error>> {
        Ok(object()
            .string("rust_version", "1.85")
            .string("os", std::env::consts::OS)
            .string("arch", std::env::consts::ARCH)
            .string("family", std::env::consts::FAMILY)
            .build())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_static_resource() {
        let resource = StaticResource::new(
            "test://hello".to_string(),
            "Hello Resource".to_string(),
            "A test resource".to_string(),
            "Hello, MCP!".to_string(),
        );
        
        assert_eq!(resource.uri(), "test://hello");
        assert_eq!(resource.name(), "Hello Resource");
        assert_eq!(resource.mime_type(), "text/plain");
        
        let content = resource.read().unwrap();
        assert_eq!(content, Value::String("Hello, MCP!".to_string()));
    }
    
    #[test]
    fn test_system_info_resource() {
        let resource = SystemInfoResource;
        
        assert_eq!(resource.uri(), "system://info");
        
        let content = resource.read().unwrap();
        assert!(content.as_object().is_some());
        assert!(content.get("os").is_some());
        assert!(content.get("arch").is_some());
    }
}