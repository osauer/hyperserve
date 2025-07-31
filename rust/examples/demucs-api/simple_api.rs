//! Simple Audio Analysis API with JSON input
//! 
//! This version accepts base64-encoded WAV files in JSON format,
//! making it easier to integrate with other applications.

use hyperserve::{Method, Response, Status, Request};
use hyperserve::middleware::{LoggingMiddleware, SecurityHeadersMiddleware};
use hyperserve::json::{object, parse, Value};
use std::fs;
use std::process::Command;
use std::time::{SystemTime, Instant};

/// Temporary directory for audio processing
const TEMP_DIR: &str = "/tmp/hyperserve-demucs";

fn main() -> std::io::Result<()> {
    // Ensure temp directory exists
    fs::create_dir_all(TEMP_DIR)?;
    
    // Create server
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8084")
        .middleware(LoggingMiddleware)
        .middleware(SecurityHeadersMiddleware::default())
        .route(Method::GET, "/", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{
                    "service": "Audio Analysis API",
                    "version": "1.0",
                    "endpoints": {
                        "/analyze": "POST - Analyze WAV file (base64 encoded)",
                        "/health": "GET - Health check"
                    }
                }"#)
        })
        .route(Method::GET, "/health", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{"status":"healthy","service":"audio-analysis-api"}"#)
        })
        .route(Method::POST, "/analyze", handle_analyze)
        .build()?;
    
    println!("Audio Analysis API running on http://127.0.0.1:8084");
    println!("\nUsage example:");
    println!("curl -X POST http://127.0.0.1:8084/analyze \\");
    println!("  -H 'Content-Type: application/json' \\");
    println!("  -d '{{\"audio\": \"<base64-encoded-wav-data>\", \"filename\": \"test.wav\"}}'");
    
    server.run()
}

/// Handle audio analysis request
fn handle_analyze(req: &Request) -> Response {
    let start_time = Instant::now();
    
    // Parse JSON body
    let body_str = match std::str::from_utf8(req.body) {
        Ok(s) => s,
        Err(_) => return error_response("Invalid UTF-8 in request body"),
    };
    
    let json_body = match parse(body_str) {
        Ok(v) => v,
        Err(e) => return error_response(&format!("Invalid JSON: {}", e)),
    };
    
    // Extract base64 audio data
    let audio_b64 = match json_body.get("audio").and_then(|v| v.as_str()) {
        Some(s) => s,
        None => return error_response("Missing 'audio' field in JSON"),
    };
    
    // Decode base64
    let audio_data = match base64_decode(audio_b64) {
        Ok(data) => data,
        Err(e) => return error_response(&format!("Failed to decode base64: {}", e)),
    };
    
    // Get filename (optional)
    let filename = json_body.get("filename")
        .and_then(|v| v.as_str())
        .unwrap_or("audio.wav");
    
    // Save file temporarily
    let file_path = format!("{}/{}_{}", TEMP_DIR, 
        SystemTime::now().duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default().as_millis(),
        filename);
    
    if let Err(e) = fs::write(&file_path, &audio_data) {
        return error_response(&format!("Failed to save file: {}", e));
    }
    
    // Analyze with Python script
    match analyze_audio(&file_path) {
        Ok(analysis) => {
            // Clean up
            let _ = fs::remove_file(&file_path);
            
            let processing_time = start_time.elapsed().as_secs_f64();
            
            // Build response
            let response = object()
                .bool("success", true)
                .object("analysis", analysis)
                .number("processing_time", processing_time)
                .build();
            
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(response.to_string())
        }
        Err(e) => {
            let _ = fs::remove_file(&file_path);
            error_response(&format!("Analysis failed: {}", e))
        }
    }
}

/// Simple base64 decoder
fn base64_decode(input: &str) -> Result<Vec<u8>, String> {
    // Remove whitespace
    let input = input.chars().filter(|c| !c.is_whitespace()).collect::<String>();
    
    // Base64 alphabet
    let alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut output = Vec::new();
    let mut buffer = 0u32;
    let mut bits = 0;
    
    for ch in input.chars() {
        if ch == '=' {
            break; // Padding
        }
        
        let value = alphabet.find(ch)
            .ok_or_else(|| format!("Invalid base64 character: {}", ch))? as u32;
        
        buffer = (buffer << 6) | value;
        bits += 6;
        
        if bits >= 8 {
            bits -= 8;
            output.push((buffer >> bits) as u8);
            buffer &= (1 << bits) - 1;
        }
    }
    
    Ok(output)
}

/// Analyze audio file using Python script
fn analyze_audio(file_path: &str) -> Result<Value, String> {
    let python_path = "python3";
    let analysis_script = include_str!("analyze_audio_simple.py");
    
    // Write analysis script to temp file
    let script_path = format!("{}/analyze_audio.py", TEMP_DIR);
    fs::write(&script_path, analysis_script)
        .map_err(|e| format!("Failed to write script: {}", e))?;
    
    // Run analysis
    let output = Command::new(python_path)
        .arg(&script_path)
        .arg(file_path)
        .output()
        .map_err(|e| format!("Failed to run analysis: {}", e))?;
    
    // Clean up script
    let _ = fs::remove_file(&script_path);
    
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!("Analysis failed: {}", stderr));
    }
    
    // Parse JSON output
    let stdout = String::from_utf8_lossy(&output.stdout);
    parse(&stdout)
        .map_err(|e| format!("Failed to parse analysis output: {}", e))
}

/// Create error response
fn error_response(message: &str) -> Response {
    let error_json = object()
        .bool("success", false)
        .string("error", message)
        .build();
    
    Response::new(Status::BadRequest)
        .header("Content-Type", "application/json")
        .body(error_json.to_string())
}