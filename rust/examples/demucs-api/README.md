# Audio Analysis API

Quick setup for audio analysis API using HyperServe.

## Quick Start

```bash
# Clone and build
git clone https://github.com/osauer/hyperserve.git
cd hyperserve/rust
cargo run --example audio-api
```

Server runs on: `http://127.0.0.1:8085`

## API Usage

### Endpoint
`POST http://127.0.0.1:8085/analyze`

### Input
- **Method**: POST
- **Content-Type**: `audio/wav`
- **Body**: Raw WAV file binary data (max 50MB)

### Output
JSON response with audio analysis:

```json
{
  "success": true,
  "metadata": {
    "duration": 1.0,
    "sample_rate": 44100,
    "channels": 1,
    "bit_depth": 16
  },
  "analysis": {
    "bpm": 120.0,
    "key": "C major",
    "energy": {
      "average": 0.707,
      "peak": 0.999,
      "dynamic_range": 16.1
    },
    "spectral": {
      "brightness": 0.199,
      "spectral_centroid": 219.75,
      "spectral_rolloff": 659.25
    },
    "stems": {
      "vocals": {"present": true, "confidence": 0.9},
      "drums": {"present": false, "confidence": 0.08},
      "bass": {"present": false, "confidence": 0.00001},
      "other": {"present": true, "confidence": 0.7}
    }
  },
  "processing_time": 0.072
}
```

### Example Request

```bash
curl -X POST http://127.0.0.1:8085/analyze \
  -H 'Content-Type: audio/wav' \
  --data-binary @your_audio.wav
```

### Python Example

```python
import requests

with open('audio.wav', 'rb') as f:
    response = requests.post(
        'http://127.0.0.1:8085/analyze',
        data=f.read(),
        headers={'Content-Type': 'audio/wav'}
    )
    result = response.json()
    print(f"BPM: {result['analysis']['bpm']}")
```

## Requirements
- Rust (cargo)
- Python 3