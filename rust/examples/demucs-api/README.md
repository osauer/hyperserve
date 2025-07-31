# Audio Analysis API Example

This example demonstrates how to build an audio analysis API using HyperServe (Rust) with Python integration.

## Features

- **Zero-dependency HTTP server** (HyperServe)
- **WAV file upload** via POST with raw binary data
- **Audio analysis** using Python (without external dependencies like numpy)
- **Comprehensive metadata extraction**:
  - Duration, sample rate, channels, bit depth
  - BPM detection
  - Key detection
  - Energy analysis (average, peak, dynamic range)
  - Spectral analysis (brightness, centroid, rolloff)
  - Stem detection (vocals, drums, bass, other)

## Running the Example

```bash
# From the rust directory
cargo run --example audio-api
```

The server will start on `http://127.0.0.1:8085`

## API Endpoints

### `GET /` - Web Interface
Returns an HTML page with API documentation and usage examples.

### `GET /health` - Health Check
Returns the server status.

### `POST /analyze` - Analyze Audio File
Accepts a WAV file as raw binary data and returns analysis results.

**Request:**
```bash
curl -X POST http://127.0.0.1:8085/analyze \
  -H 'Content-Type: audio/wav' \
  --data-binary @your_audio.wav
```

**Response:**
```json
{
  "success": true,
  "metadata": {
    "duration": 1.0,
    "format": "wav",
    "sample_rate": 44100,
    "channels": 2,
    "bit_depth": 16,
    "frames": 44100
  },
  "analysis": {
    "bpm": 120.0,
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
      "other": {"present": true, "confidence": 0.70}
    }
  },
  "processing_time": 0.234
}
```

## Implementation Details

### Server Architecture

1. **HyperServe HTTP Server**: Handles incoming requests with zero dependencies
2. **Request Parsing**: Properly handles large POST bodies (up to 50MB)
3. **Python Integration**: Spawns Python subprocess for audio analysis
4. **JSON Response**: Combines Python analysis with server metadata

### Audio Analysis

The Python script (`analyze_audio_simple.py`) performs:

1. **WAV File Parsing**: Extracts format information without external libraries
2. **BPM Detection**: Simple zero-crossing based tempo estimation
3. **Energy Analysis**: RMS, peak detection, and dynamic range calculation
4. **Spectral Analysis**: Brightness, centroid, and rolloff frequency
5. **Stem Detection**: Heuristic-based detection of different audio components

### Key Implementation Features

- **Large File Support**: Fixed HTTP body reading to handle files larger than 8KB
- **Case-Insensitive Headers**: Proper HTTP header parsing
- **Error Handling**: Comprehensive error messages for debugging
- **Security Headers**: Includes standard security headers via middleware
- **Logging**: Request/response logging for monitoring

## Testing

Create a test WAV file:
```python
import wave
import struct
import math

# Create a 440Hz sine wave
sample_rate = 44100
duration = 1
frequency = 440

with wave.open('test_audio.wav', 'wb') as wav:
    wav.setnchannels(1)
    wav.setsampwidth(2)
    wav.setframerate(sample_rate)
    
    for i in range(int(sample_rate * duration)):
        sample = int(32767 * math.sin(2 * math.pi * frequency * i / sample_rate))
        wav.writeframes(struct.pack('<h', sample))
```

Test the API:
```bash
curl -X POST http://127.0.0.1:8085/analyze \
  -H 'Content-Type: audio/wav' \
  --data-binary @test_audio.wav | jq
```

## Production Considerations

1. **File Size Limits**: Currently limited to 50MB, adjust as needed
2. **Temporary Files**: Files are saved to `/tmp/hyperserve-demucs/` and cleaned up after processing
3. **Python Dependency**: Requires Python 3 to be installed
4. **Security**: Consider adding authentication for production use
5. **Performance**: For high load, consider caching analysis results

## Extending the Example

This example can be extended to:
- Support more audio formats (MP3, FLAC, etc.)
- Integrate with actual Demucs for source separation
- Add WebSocket support for real-time analysis updates
- Store results in a database
- Add batch processing capabilities