//! Demucs Audio Analysis API Example
//! 
//! This example demonstrates building an audio analysis API with HyperServe
//! that integrates with Python-based Demucs for audio decomposition.

use hyperserve::{Method, Response, Status, Request};
use hyperserve::middleware::{LoggingMiddleware, SecurityHeadersMiddleware};
use hyperserve::json::{object, parse, Value};
use std::fs;
use std::process::Command;
use std::time::{SystemTime, Instant};

/// Maximum file size in bytes (50MB)
const MAX_FILE_SIZE: usize = 50 * 1024 * 1024;

/// Temporary directory for audio processing
const TEMP_DIR: &str = "/tmp/hyperserve-demucs";

fn main() -> std::io::Result<()> {
    // Ensure temp directory exists
    fs::create_dir_all(TEMP_DIR)?;
    
    // Create server using builder
    let server = hyperserve::builder::ServerBuilder::new("127.0.0.1:8082")
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
                .body(r#"{"status":"healthy","service":"demucs-api"}"#)
        })
        .route(Method::POST, "/api/audio/analyze", handle_analyze)
        .build()?;
    
    println!("Demucs Audio Analysis API running on http://127.0.0.1:8082");
    println!("Upload audio files to /api/audio/analyze for analysis");
    
    server.run()
}

/// Handle audio analysis request
fn handle_analyze(req: &Request) -> Response {
    let start_time = Instant::now();
    
    // Check content type
    let content_type = req.headers.get("content-type")
        .map(|v| v.split(';').next().unwrap_or(""))
        .unwrap_or("");
    
    if !content_type.starts_with("multipart/form-data") {
        return Response::new(Status::BadRequest)
            .header("Content-Type", "application/json")
            .body(r#"{"error":"Content-Type must be multipart/form-data"}"#);
    }
    
    // Parse multipart data
    match parse_multipart(req) {
        Ok(file_data) => {
            // Save file temporarily
            let file_path = format!("{}/audio_{}.wav", TEMP_DIR, 
                SystemTime::now().duration_since(SystemTime::UNIX_EPOCH)
                    .unwrap_or_default().as_millis());
            
            if let Err(e) = fs::write(&file_path, &file_data) {
                return error_response(&format!("Failed to save file: {}", e));
            }
            
            // Analyze with Demucs
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
        Err(e) => error_response(&format!("Failed to parse upload: {}", e))
    }
}

/// Simple multipart parser (extracts first file found)
fn parse_multipart(req: &Request) -> Result<Vec<u8>, String> {
    // Get boundary from Content-Type
    let content_type = req.headers.get("content-type")
        .copied()
        .ok_or("Missing Content-Type")?;
    
    let boundary = content_type
        .split("boundary=")
        .nth(1)
        .ok_or("Missing boundary")?
        .trim();
    
    let body = std::str::from_utf8(req.body)
        .map_err(|_| "Invalid UTF-8 in body")?;
    
    // Find file data between boundaries
    let boundary_marker = format!("--{}", boundary);
    let parts: Vec<&str> = body.split(&boundary_marker).collect();
    
    for part in parts {
        if part.contains("Content-Disposition: form-data") && part.contains("filename=") {
            // Find the double CRLF that separates headers from data
            if let Some(data_start) = part.find("\r\n\r\n") {
                let file_data = &part[data_start + 4..];
                // Remove trailing boundary markers
                let file_data = file_data.trim_end_matches(&format!("--{}--", boundary))
                    .trim_end_matches("\r\n");
                
                // Check file size
                if file_data.len() > MAX_FILE_SIZE {
                    return Err("File too large".to_string());
                }
                
                return Ok(file_data.as_bytes().to_vec());
            }
        }
    }
    
    Err("No file found in multipart data".to_string())
}

/// Analyze audio file using Python script
fn analyze_audio(file_path: &str) -> Result<Value, String> {
    // Use system Python for now (in production, use venv with demucs)
    let python_path = "python3";
    let analysis_script = include_str!("analyze_audio_simple.py");
    
    // Write analysis script to temp file
    let script_path = format!("{}/analyze_audio.py", TEMP_DIR);
    fs::write(&script_path, analysis_script)
        .map_err(|e| format!("Failed to write script: {}", e))?;
    
    // Run analysis
    let output = Command::new(&python_path)
        .arg(&script_path)
        .arg(file_path)
        // No venv needed for simple script
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
    
    Response::new(Status::InternalServerError)
        .header("Content-Type", "application/json")
        .body(error_json.to_string())
}

/// Home page HTML
const HOME_HTML: &str = r#"<!DOCTYPE html>
<html>
<head>
    <title>Demucs Audio Analysis API</title>
    <style>
        body { 
            font-family: Arial, sans-serif; 
            max-width: 800px; 
            margin: 0 auto; 
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 30px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 { color: #333; }
        .upload-area {
            border: 2px dashed #ccc;
            border-radius: 5px;
            padding: 30px;
            text-align: center;
            margin: 20px 0;
            cursor: pointer;
            transition: all 0.3s;
        }
        .upload-area:hover {
            border-color: #4CAF50;
            background: #f9f9f9;
        }
        .upload-area.dragging {
            border-color: #4CAF50;
            background: #e8f5e9;
        }
        button {
            background: #4CAF50;
            color: white;
            border: none;
            padding: 10px 20px;
            border-radius: 5px;
            cursor: pointer;
            font-size: 16px;
        }
        button:hover { background: #45a049; }
        button:disabled { 
            background: #ccc; 
            cursor: not-allowed;
        }
        .results {
            margin-top: 20px;
            padding: 20px;
            background: #f9f9f9;
            border-radius: 5px;
            display: none;
        }
        .error {
            color: #f44336;
            margin-top: 10px;
        }
        .success {
            color: #4CAF50;
        }
        pre {
            background: #f5f5f5;
            padding: 10px;
            border-radius: 5px;
            overflow-x: auto;
        }
        .spinner {
            display: none;
            margin: 20px auto;
            width: 50px;
            height: 50px;
            border: 5px solid #f3f3f3;
            border-top: 5px solid #4CAF50;
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎵 Demucs Audio Analysis API</h1>
        <p>Upload an audio file to analyze its musical properties.</p>
        
        <div class="upload-area" id="uploadArea">
            <p>Drag and drop an audio file here or click to select</p>
            <input type="file" id="fileInput" accept="audio/*" style="display: none;">
        </div>
        
        <div style="text-align: center;">
            <button id="analyzeBtn" disabled>Analyze Audio</button>
        </div>
        
        <div class="spinner" id="spinner"></div>
        
        <div id="error" class="error"></div>
        
        <div class="results" id="results">
            <h3>Analysis Results</h3>
            <pre id="resultContent"></pre>
        </div>
        
        <h2>API Usage</h2>
        <pre>curl -X POST http://localhost:8082/api/audio/analyze \
  -F "audio=@/path/to/your/audio.wav" \
  -H "Accept: application/json"</pre>
    </div>
    
    <script>
        const uploadArea = document.getElementById('uploadArea');
        const fileInput = document.getElementById('fileInput');
        const analyzeBtn = document.getElementById('analyzeBtn');
        const results = document.getElementById('results');
        const resultContent = document.getElementById('resultContent');
        const error = document.getElementById('error');
        const spinner = document.getElementById('spinner');
        
        let selectedFile = null;
        
        // Click to upload
        uploadArea.addEventListener('click', () => fileInput.click());
        
        // Drag and drop
        uploadArea.addEventListener('dragover', (e) => {
            e.preventDefault();
            uploadArea.classList.add('dragging');
        });
        
        uploadArea.addEventListener('dragleave', () => {
            uploadArea.classList.remove('dragging');
        });
        
        uploadArea.addEventListener('drop', (e) => {
            e.preventDefault();
            uploadArea.classList.remove('dragging');
            
            const files = e.dataTransfer.files;
            if (files.length > 0) {
                handleFile(files[0]);
            }
        });
        
        // File selection
        fileInput.addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                handleFile(e.target.files[0]);
            }
        });
        
        function handleFile(file) {
            if (!file.type.startsWith('audio/')) {
                showError('Please select an audio file');
                return;
            }
            
            if (file.size > 50 * 1024 * 1024) {
                showError('File too large. Maximum size is 50MB');
                return;
            }
            
            selectedFile = file;
            uploadArea.innerHTML = `<p>Selected: ${file.name} (${(file.size / 1024 / 1024).toFixed(2)}MB)</p>`;
            analyzeBtn.disabled = false;
            error.textContent = '';
        }
        
        // Analyze button
        analyzeBtn.addEventListener('click', async () => {
            if (!selectedFile) return;
            
            const formData = new FormData();
            formData.append('audio', selectedFile);
            
            analyzeBtn.disabled = true;
            spinner.style.display = 'block';
            results.style.display = 'none';
            error.textContent = '';
            
            try {
                const response = await fetch('/api/audio/analyze', {
                    method: 'POST',
                    body: formData
                });
                
                const data = await response.json();
                
                if (data.success) {
                    resultContent.textContent = JSON.stringify(data, null, 2);
                    results.style.display = 'block';
                } else {
                    showError(data.error || 'Analysis failed');
                }
            } catch (e) {
                showError('Failed to analyze: ' + e.message);
            } finally {
                analyzeBtn.disabled = false;
                spinner.style.display = 'none';
            }
        });
        
        function showError(message) {
            error.textContent = message;
            results.style.display = 'none';
        }
    </script>
</body>
</html>"#;