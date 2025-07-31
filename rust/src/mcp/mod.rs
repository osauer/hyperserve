//! Model Context Protocol (MCP) implementation for HyperServe
//! 
//! Zero-dependency MCP support for AI assistant integration

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use crate::http::{Request, Response, Status};
use crate::json::{self, Value, object};

mod jsonrpc;
mod sse;
pub mod tools;
pub mod resources;

pub use jsonrpc::{JsonRpcRequest, JsonRpcResponse, JsonRpcError};
pub use sse::SSEManager;
pub use tools::{MCPTool, MCPToolInfo};
pub use resources::{MCPResource, MCPResourceInfo};

/// MCP protocol version
pub const MCP_VERSION: &str = "2024-11-05";

/// MCP server information
#[derive(Debug, Clone)]
pub struct MCPServerInfo {
    /// Server name
    pub name: String,
    /// Server version
    pub version: String,
}

/// MCP client information
#[derive(Debug, Clone)]
pub struct MCPClientInfo {
    /// Client name
    pub name: String,
    /// Client version
    pub version: String,
}

/// MCP capabilities
#[derive(Debug, Clone)]
pub struct MCPCapabilities {
    /// Tool capabilities
    pub tools: Option<ToolsCapability>,
    /// Resource capabilities
    pub resources: Option<ResourcesCapability>,
    /// SSE capabilities
    pub sse: Option<SSECapability>,
}

/// Tools capability
#[derive(Debug, Clone)]
pub struct ToolsCapability {
    /// Whether tool list can change
    pub list_changed: bool,
}

/// Resources capability
#[derive(Debug, Clone)]
pub struct ResourcesCapability {
    /// Whether resources support subscription
    pub subscribe: bool,
    /// Whether resource list can change
    pub list_changed: bool,
}

/// SSE capability
#[derive(Debug, Clone)]
pub struct SSECapability {
    /// Whether SSE is enabled
    pub enabled: bool,
    /// SSE endpoint
    pub endpoint: String,
    /// Whether header routing is supported
    pub header_routing: bool,
}

/// MCP handler for managing tools and resources
pub struct MCPHandler {
    tools: Arc<Mutex<HashMap<String, Box<dyn MCPTool>>>>,
    resources: Arc<Mutex<HashMap<String, Box<dyn MCPResource>>>>,
    server_info: MCPServerInfo,
    sse_manager: Arc<SSEManager>,
    jsonrpc_engine: Arc<jsonrpc::JsonRpcEngine>,
}

impl MCPHandler {
    /// Create a new MCP handler
    pub fn new(server_info: MCPServerInfo) -> Self {
        let handler = Self {
            tools: Arc::new(Mutex::new(HashMap::new())),
            resources: Arc::new(Mutex::new(HashMap::new())),
            server_info,
            sse_manager: Arc::new(SSEManager::new()),
            jsonrpc_engine: Arc::new(jsonrpc::JsonRpcEngine::new()),
        };
        
        // Register MCP methods
        handler.register_mcp_methods();
        
        handler
    }
    
    /// Register MCP protocol methods
    fn register_mcp_methods(&self) {
        let engine = &self.jsonrpc_engine;
        
        // Initialize methods
        {
            let server_info = self.server_info.clone();
            let capabilities = self.get_capabilities();
            engine.register_method("initialize", move |params| {
                handle_initialize(params, &server_info, &capabilities)
            });
        }
        
        engine.register_method("initialized", |_| {
            Ok(Value::Null)
        });
        
        // Tool methods
        {
            let tools = Arc::clone(&self.tools);
            engine.register_method("tools/list", move |_| {
                handle_tools_list(&tools)
            });
        }
        
        {
            let tools = Arc::clone(&self.tools);
            engine.register_method("tools/call", move |params| {
                handle_tools_call(params, &tools)
            });
        }
        
        // Resource methods
        {
            let resources = Arc::clone(&self.resources);
            engine.register_method("resources/list", move |_| {
                handle_resources_list(&resources)
            });
        }
        
        {
            let resources = Arc::clone(&self.resources);
            engine.register_method("resources/read", move |params| {
                handle_resources_read(params, &resources)
            });
        }
        
        // Utility methods
        engine.register_method("ping", |_| {
            Ok(object()
                .string("message", "pong")
                .build())
        });
    }
    
    /// Get server capabilities
    fn get_capabilities(&self) -> MCPCapabilities {
        MCPCapabilities {
            tools: Some(ToolsCapability {
                list_changed: false,
            }),
            resources: Some(ResourcesCapability {
                subscribe: false,
                list_changed: false,
            }),
            sse: Some(SSECapability {
                enabled: true,
                endpoint: "same".to_string(),
                header_routing: true,
            }),
        }
    }
    
    /// Register a tool
    pub fn register_tool(&self, tool: Box<dyn MCPTool>) {
        let mut tools = self.tools.lock().unwrap();
        tools.insert(tool.name().to_string(), tool);
    }
    
    /// Register a resource
    pub fn register_resource(&self, resource: Box<dyn MCPResource>) {
        let mut resources = self.resources.lock().unwrap();
        resources.insert(resource.uri().to_string(), resource);
    }
    
    /// Handle HTTP request
    pub fn handle_request(&self, request: &Request) -> Response {
        // Check for SSE
        if request.headers.get("Accept").map(|v| v.as_ref()) == Some("text/event-stream") {
            return self.sse_manager.handle_sse(request);
        }
        
        // Handle GET requests
        if request.method == crate::http::Method::GET {
            return handle_get_request(request, &self.server_info, &self.get_capabilities());
        }
        
        // Handle POST requests
        if request.method != crate::http::Method::POST {
            return Response::new(Status::MethodNotAllowed)
                .body("Method not allowed. MCP requires POST requests.");
        }
        
        // Check content type
        let content_type = request.headers.get("Content-Type").map(|v| v.as_ref()).unwrap_or("");
        if !content_type.contains("application/json") {
            return Response::new(Status::BadRequest)
                .body("Content-Type must be application/json");
        }
        
        // Parse JSON-RPC request
        let request_str = std::str::from_utf8(request.body).unwrap_or("");
        match json::parse(request_str) {
            Ok(json_value) => {
                match JsonRpcRequest::from_json(json_value) {
                    Ok(jsonrpc_req) => {
                        let response = self.jsonrpc_engine.process_request(&jsonrpc_req);
                        Response::new(Status::Ok)
                            .header("Content-Type", "application/json")
                            .body(response.to_json().to_string())
                    }
                    Err(e) => {
                        let error_response = JsonRpcResponse::error(
                            None,
                            JsonRpcError {
                                code: -32600,
                                message: "Invalid Request".to_string(),
                                data: Some(Value::String(e)),
                            },
                        );
                        Response::new(Status::Ok)
                            .header("Content-Type", "application/json")
                            .body(error_response.to_json().to_string())
                    }
                }
            }
            Err(e) => {
                let error_response = JsonRpcResponse::error(
                    None,
                    JsonRpcError {
                        code: -32700,
                        message: "Parse error".to_string(),
                        data: Some(Value::String(e)),
                    },
                );
                Response::new(Status::Ok)
                    .header("Content-Type", "application/json")
                    .body(error_response.to_json().to_string())
            }
        }
    }
}

/// Handle GET requests with helpful information
fn handle_get_request(request: &Request, server_info: &MCPServerInfo, capabilities: &MCPCapabilities) -> Response {
    let accept = request.headers.get("Accept").map(|v| v.as_ref()).unwrap_or("");
    
    if is_json_accepted(accept) {
        // Return JSON status
        let status = object()
            .string("status", "ready")
            .object("server", object()
                .string("name", &server_info.name)
                .string("version", &server_info.version)
                .build())
            .object("capabilities", object()
                .object("tools", object()
                    .bool("listChanged", capabilities.tools.as_ref().map(|t| t.list_changed).unwrap_or(false))
                    .build())
                .object("resources", object()
                    .bool("subscribe", capabilities.resources.as_ref().map(|r| r.subscribe).unwrap_or(false))
                    .bool("listChanged", capabilities.resources.as_ref().map(|r| r.list_changed).unwrap_or(false))
                    .build())
                .object("sse", object()
                    .bool("enabled", capabilities.sse.as_ref().map(|s| s.enabled).unwrap_or(false))
                    .string("endpoint", &capabilities.sse.as_ref().map(|s| s.endpoint.clone()).unwrap_or_else(|| "".to_string()))
                    .bool("headerRouting", capabilities.sse.as_ref().map(|s| s.header_routing).unwrap_or(false))
                    .build())
                .build())
            .string("endpoint", request.path)
            .string("transport", "http")
            .build();
        
        return Response::new(Status::Ok)
            .header("Content-Type", "application/json")
            .body(status.to_string());
    }
    
    // Return HTML help page
    let html = format!(r#"<!DOCTYPE html>
<html>
<head>
    <title>MCP Endpoint - HyperServe</title>
    <style>
        body {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
               max-width: 800px; margin: 50px auto; padding: 20px; line-height: 1.6; }}
        h1 {{ color: #333; }}
        code {{ background: #f4f4f4; padding: 2px 6px; border-radius: 3px; }}
        pre {{ background: #f4f4f4; padding: 15px; border-radius: 5px; overflow-x: auto; }}
        .example {{ margin: 20px 0; }}
        .note {{ background: #e8f4f8; padding: 15px; border-left: 4px solid #0084c7; margin: 20px 0; }}
    </style>
</head>
<body>
    <h1>Model Context Protocol (MCP) Endpoint</h1>
    
    <p>This endpoint implements the <a href="https://modelcontextprotocol.io">Model Context Protocol</a> 
    for AI assistant integration.</p>
    
    <div class="note">
        <strong>Note:</strong> MCP uses JSON-RPC 2.0 over HTTP POST. GET requests are not supported.
    </div>
    
    <h2>How to Use</h2>
    
    <div class="example">
        <h3>Initialize Connection</h3>
        <pre>curl -X POST {} \
  -H "Content-Type: application/json" \
  -d '{{
    "jsonrpc": "2.0",
    "method": "initialize",
    "params": {{
      "protocolVersion": "2024-11-05",
      "capabilities": {{}},
      "clientInfo": {{"name": "test-client", "version": "1.0.0"}}
    }},
    "id": 1
  }}'</pre>
    </div>
    
    <div class="example">
        <h3>List Available Tools</h3>
        <pre>curl -X POST {} \
  -H "Content-Type: application/json" \
  -d '{{"jsonrpc": "2.0", "method": "tools/list", "id": 2}}'</pre>
    </div>
    
    <h2>Available Methods</h2>
    <ul>
        <li><code>initialize</code> - Initialize MCP session</li>
        <li><code>ping</code> - Test connectivity</li>
        <li><code>tools/list</code> - List available tools</li>
        <li><code>tools/call</code> - Execute a tool</li>
        <li><code>resources/list</code> - List available resources</li>
        <li><code>resources/read</code> - Read a resource</li>
    </ul>
    
    <h2>Server Information</h2>
    <p>Server: {} v{}</p>
</body>
</html>"#, request.path, request.path, server_info.name, server_info.version);
    
    Response::new(Status::Ok)
        .header("Content-Type", "text/html; charset=utf-8")
        .body(html)
}

/// Check if JSON is accepted
fn is_json_accepted(accept: &str) -> bool {
    if accept.is_empty() {
        return false;
    }
    
    let accept_lower = accept.to_lowercase();
    
    // Handle */*
    if accept_lower == "*/*" {
        return true;
    }
    
    // Parse Accept header
    for part in accept_lower.split(',') {
        let media_type = part.split(';').next().unwrap_or("").trim();
        if media_type == "application/json" || 
           media_type == "*/*" || 
           media_type == "application/*" {
            return true;
        }
    }
    
    false
}

/// Handle initialize method
fn handle_initialize(
    params: Option<Value>,
    server_info: &MCPServerInfo,
    capabilities: &MCPCapabilities,
) -> Result<Value, JsonRpcError> {
    // Parse client info if provided
    let _client_info = if let Some(params) = params {
        if let Some(client_obj) = params.get("clientInfo").and_then(|v| v.as_object()) {
            if let (Some(name), Some(version)) = (
                client_obj.get("name").and_then(|v| v.as_str()),
                client_obj.get("version").and_then(|v| v.as_str())
            ) {
                Some(MCPClientInfo {
                    name: name.to_string(),
                    version: version.to_string(),
                })
            } else {
                None
            }
        } else {
            None
        }
    } else {
        None
    };
    
    Ok(object()
        .string("protocolVersion", MCP_VERSION)
        .object("capabilities", object()
            .object("tools", object()
                .bool("listChanged", capabilities.tools.as_ref().map(|t| t.list_changed).unwrap_or(false))
                .build())
            .object("resources", object()
                .bool("subscribe", capabilities.resources.as_ref().map(|r| r.subscribe).unwrap_or(false))
                .bool("listChanged", capabilities.resources.as_ref().map(|r| r.list_changed).unwrap_or(false))
                .build())
            .object("sse", object()
                .bool("enabled", capabilities.sse.as_ref().map(|s| s.enabled).unwrap_or(false))
                .string("endpoint", &capabilities.sse.as_ref().map(|s| s.endpoint.clone()).unwrap_or_else(|| "".to_string()))
                .bool("headerRouting", capabilities.sse.as_ref().map(|s| s.header_routing).unwrap_or(false))
                .build())
            .build())
        .object("serverInfo", object()
            .string("name", &server_info.name)
            .string("version", &server_info.version)
            .build())
        .string("instructions", "Follow the initialization protocol: after receiving this response, send an 'initialized' notification")
        .build())
}

/// Handle tools/list method
fn handle_tools_list(tools: &Arc<Mutex<HashMap<String, Box<dyn MCPTool>>>>) -> Result<Value, JsonRpcError> {
    let tools = tools.lock().unwrap();
    let mut tool_list = Vec::new();
    
    for tool in tools.values() {
        tool_list.push(object()
            .string("name", tool.name())
            .string("description", tool.description())
            .object("inputSchema", tool.schema())
            .build());
    }
    
    Ok(object()
        .array("tools", tool_list)
        .build())
}

/// Handle tools/call method
fn handle_tools_call(
    params: Option<Value>,
    tools: &Arc<Mutex<HashMap<String, Box<dyn MCPTool>>>>,
) -> Result<Value, JsonRpcError> {
    let params = params.ok_or_else(|| JsonRpcError {
        code: -32602,
        message: "Invalid params".to_string(),
        data: None,
    })?;
    
    let tool_name = params.get("name").and_then(|v| v.as_str()).ok_or_else(|| JsonRpcError {
        code: -32602,
        message: "Tool name is required".to_string(),
        data: None,
    })?;
    
    let tool_args = params.get("arguments").cloned().unwrap_or(Value::Null);
    
    let tools = tools.lock().unwrap();
    let tool = tools.get(tool_name).ok_or_else(|| JsonRpcError {
        code: -32602,
        message: format!("Tool not found: {}", tool_name),
        data: None,
    })?;
    
    match tool.execute(tool_args) {
        Ok(result) => {
            // Format result as MCP response
            let content = match result {
                Value::String(s) => vec![object()
                    .string("type", "text")
                    .string("text", &s)
                    .build()],
                _ => vec![object()
                    .string("type", "text")
                    .string("text", &result.to_string())
                    .build()],
            };
            
            Ok(object()
                .array("content", content)
                .build())
        }
        Err(e) => Err(JsonRpcError {
            code: -32603,
            message: format!("Tool execution failed: {}", e),
            data: None,
        }),
    }
}

/// Handle resources/list method
fn handle_resources_list(resources: &Arc<Mutex<HashMap<String, Box<dyn MCPResource>>>>) -> Result<Value, JsonRpcError> {
    let resources = resources.lock().unwrap();
    let mut resource_list = Vec::new();
    
    for resource in resources.values() {
        resource_list.push(object()
            .string("uri", resource.uri())
            .string("name", resource.name())
            .string("description", resource.description())
            .string("mimeType", resource.mime_type())
            .build());
    }
    
    Ok(object()
        .array("resources", resource_list)
        .build())
}

/// Handle resources/read method
fn handle_resources_read(
    params: Option<Value>,
    resources: &Arc<Mutex<HashMap<String, Box<dyn MCPResource>>>>,
) -> Result<Value, JsonRpcError> {
    let params = params.ok_or_else(|| JsonRpcError {
        code: -32602,
        message: "Invalid params".to_string(),
        data: None,
    })?;
    
    let uri = params.get("uri").and_then(|v| v.as_str()).ok_or_else(|| JsonRpcError {
        code: -32602,
        message: "URI is required".to_string(),
        data: None,
    })?;
    
    let resources = resources.lock().unwrap();
    let resource = resources.get(uri).ok_or_else(|| JsonRpcError {
        code: -32602,
        message: format!("Resource not found: {}", uri),
        data: None,
    })?;
    
    match resource.read() {
        Ok(content) => {
            let text_content = match content {
                Value::String(s) => s,
                _ => content.to_string(),
            };
            
            let contents = vec![object()
                .string("uri", resource.uri())
                .string("mimeType", resource.mime_type())
                .string("text", &text_content)
                .build()];
            
            Ok(object()
                .array("contents", contents)
                .build())
        }
        Err(e) => Err(JsonRpcError {
            code: -32603,
            message: format!("Failed to read resource: {}", e),
            data: None,
        }),
    }
}