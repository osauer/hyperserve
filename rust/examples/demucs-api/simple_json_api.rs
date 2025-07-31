//! Simple Audio Analysis API - Standalone version
//! 
//! This accepts WAV files directly via POST with proper response

use hyperserve::{Method, Response, Status, Request};
use hyperserve::middleware::{LoggingMiddleware, SecurityHeadersMiddleware};
use std::fs;
use std::process::Command;
use std::time::{SystemTime, Instant};

/// Temporary directory for audio processing
const TEMP_DIR: &str = "/tmp/hyperserve-demucs";

fn main() -> std::io::Result<()> {
    // Ensure temp directory exists
    fs::create_dir_all(TEMP_DIR)?;
    
    // Create server
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8085")
        .middleware(LoggingMiddleware)
        .middleware(SecurityHeadersMiddleware::default())
        .route(Method::GET, "/", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "text/html")
                .body(HOME_HTML)
        })
        .route(Method::GET, "/health", |_| {
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(r#"{"status":"healthy","service":"audio-analysis-api"}"#)
        })
        .route(Method::POST, "/analyze", handle_analyze)
        .build()?;
    
    println!("Audio Analysis API running on http://127.0.0.1:8085");
    println!("\nEndpoints:");
    println!("  GET  / - Web interface");
    println!("  GET  /health - Health check");
    println!("  POST /analyze - Analyze WAV file (raw POST body)");
    println!("\nExample usage:");
    println!("  curl -X POST http://127.0.0.1:8085/analyze \\");
    println!("    -H 'Content-Type: audio/wav' \\");
    println!("    --data-binary @test_audio.wav");
    
    server.run()
}

/// Handle audio analysis request (accepts raw WAV data)
fn handle_analyze(req: &Request) -> Response {
    let start_time = Instant::now();
    
    // Check content type (case-insensitive header lookup)
    let content_type = req.headers.iter()
        .find(|(k, _)| k.eq_ignore_ascii_case("content-type"))
        .map(|(_, v)| v.to_lowercase())
        .unwrap_or_default();
    
    if !content_type.contains("audio") && !content_type.contains("wav") && !content_type.contains("octet-stream") {
        eprintln!("Rejected content-type: '{}'", content_type);
        eprintln!("Available headers: {:?}", req.headers.keys().collect::<Vec<_>>());
        return error_response("Content-Type should be audio/wav or application/octet-stream");
    }
    
    // Check file size
    eprintln!("Request body size: {} bytes", req.body.len());
    if req.body.len() > 50 * 1024 * 1024 {
        return error_response("File too large (max 50MB)");
    }
    
    if req.body.len() < 44 {
        return error_response("File too small to be a valid WAV");
    }
    
    // Check WAV header
    if req.body.len() >= 4 {
        eprintln!("First 4 bytes: {:?}", &req.body[..4]);
        if &req.body[..4] != b"RIFF" {
            eprintln!("Expected RIFF header, got: {:?}", std::str::from_utf8(&req.body[..4]));
        }
    }
    
    // Save WAV data to temp file
    let file_path = format!("{}/audio_{}.wav", TEMP_DIR, 
        SystemTime::now().duration_since(SystemTime::UNIX_EPOCH)
            .unwrap_or_default().as_millis());
    
    if let Err(e) = fs::write(&file_path, req.body) {
        return error_response(&format!("Failed to save file: {}", e));
    }
    
    // Analyze with Python script
    eprintln!("Analyzing file: {}", file_path);
    match analyze_audio(&file_path) {
        Ok(output) => {
            eprintln!("Analysis succeeded, output size: {} bytes", output.len());
            // Clean up
            let _ = fs::remove_file(&file_path);
            
            let processing_time = start_time.elapsed().as_secs_f64();
            
            // Parse the Python output and add processing time
            if let Ok(json_str) = std::str::from_utf8(&output) {
                eprintln!("Python output: {}", json_str);
                // Simple JSON injection of processing time
                let json_trimmed = json_str.trim();
                if let Some(pos) = json_trimmed.rfind('}') {
                    let mut result = json_trimmed[..pos].to_string();
                    result.push_str(&format!(", \"processing_time\": {:.3}, \"success\": true", processing_time));
                    result.push('}');
                    
                    eprintln!("Final response: {}", result);
                    return Response::new(Status::Ok)
                        .header("Content-Type", "application/json")
                        .body(result);
                }
            }
            
            // Fallback if JSON manipulation fails
            Response::new(Status::Ok)
                .header("Content-Type", "application/json")
                .body(format!(r#"{{"success": true, "processing_time": {:.3}, "raw_output": "{}"}}"#, 
                    processing_time, 
                    String::from_utf8_lossy(&output).escape_default()))
        }
        Err(e) => {
            let _ = fs::remove_file(&file_path);
            error_response(&format!("Analysis failed: {}", e))
        }
    }
}

/// Analyze audio file using Python script
fn analyze_audio(file_path: &str) -> Result<Vec<u8>, String> {
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
        return Err(format!("Python error: {}", stderr));
    }
    
    Ok(output.stdout)
}

/// Create error response
fn error_response(message: &str) -> Response {
    Response::new(Status::BadRequest)
        .header("Content-Type", "application/json")
        .body(format!(r#"{{"success": false, "error": "{}"}}"#, message))
}

/// Simple test interface
const HOME_HTML: &str = r#"<!DOCTYPE html>
<html>
<head>
    <title>Audio Analysis API</title>
    <style>
        body { 
            font-family: Arial, sans-serif; 
            max-width: 800px; 
            margin: 50px auto; 
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        code {
            background: #f5f5f5;
            padding: 2px 5px;
            border-radius: 3px;
        }
        pre {
            background: #f5f5f5;
            padding: 15px;
            border-radius: 5px;
            overflow-x: auto;
        }
        .endpoint {
            margin: 20px 0;
            padding: 15px;
            background: #f9f9f9;
            border-left: 4px solid #4CAF50;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎵 Audio Analysis API</h1>
        <p>Simple API for analyzing WAV files using HyperServe (Rust) with Python audio processing.</p>
        
        <h2>API Endpoints</h2>
        
        <div class="endpoint">
            <h3>POST /analyze</h3>
            <p>Analyze a WAV file and return metadata</p>
            <p><strong>Content-Type:</strong> <code>audio/wav</code> or <code>application/octet-stream</code></p>
            <p><strong>Body:</strong> Raw WAV file data (binary)</p>
            <p><strong>Response:</strong> JSON with analysis results</p>
        </div>
        
        <h2>Example Usage</h2>
        
        <h3>Using curl:</h3>
        <pre>curl -X POST http://127.0.0.1:8085/analyze \
  -H 'Content-Type: audio/wav' \
  --data-binary @your_audio.wav</pre>
        
        <h3>Using Python:</h3>
        <pre>import requests

with open('your_audio.wav', 'rb') as f:
    response = requests.post(
        'http://127.0.0.1:8085/analyze',
        data=f.read(),
        headers={'Content-Type': 'audio/wav'}
    )
    
print(response.json())</pre>
        
        <h3>Response Format:</h3>
        <pre>{
  "success": true,
  "metadata": {
    "duration": 5.0,
    "sample_rate": 44100,
    "channels": 2,
    "bit_depth": 16
  },
  "analysis": {
    "bpm": 120.5,
    "key": "C major",
    "energy": {
      "average": 0.45,
      "peak": 0.89,
      "dynamic_range": 12.5
    },
    "spectral": {
      "brightness": 0.72,
      "spectral_centroid": 2500.0,
      "spectral_rolloff": 8000.0
    },
    "stems": {
      "vocals": {"present": true, "confidence": 0.85},
      "drums": {"present": true, "confidence": 0.92},
      "bass": {"present": true, "confidence": 0.88},
      "guitar": {"present": true, "confidence": 0.75},
      "piano": {"present": false, "confidence": 0.25},
      "other": {"present": true, "confidence": 0.70}
    }
  },
  "processing_time": 0.234
}</pre>
        
        <h2>Test the API</h2>
        <p>You can test the API by running the test script:</p>
        <pre>./test_api.sh test_audio.wav</pre>
        
        <p>Or use the Python test client:</p>
        <pre>python test_client.py test_audio.wav</pre>
    </div>
</body>
</html>"#;