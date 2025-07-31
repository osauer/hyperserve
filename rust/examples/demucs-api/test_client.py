#!/usr/bin/env python3
"""
Test client for the Audio Analysis API
Shows how to send WAV files to the API endpoint
"""

import json
import base64
import sys
from pathlib import Path

try:
    import requests
except ImportError:
    print("Please install requests: pip install requests")
    print("Or use the curl example instead")
    sys.exit(1)

def analyze_wav_file(file_path, api_url="http://127.0.0.1:8084/analyze"):
    """Send a WAV file to the API for analysis"""
    
    # Read the WAV file
    with open(file_path, 'rb') as f:
        wav_data = f.read()
    
    # Encode to base64
    wav_b64 = base64.b64encode(wav_data).decode('utf-8')
    
    # Prepare request
    payload = {
        "audio": wav_b64,
        "filename": Path(file_path).name
    }
    
    # Send request
    response = requests.post(api_url, json=payload)
    
    # Parse response
    result = response.json()
    
    if result.get("success"):
        print(f"Analysis successful!")
        print(f"Processing time: {result['processing_time']:.2f} seconds")
        print("\nAnalysis results:")
        print(json.dumps(result['analysis'], indent=2))
    else:
        print(f"Analysis failed: {result.get('error', 'Unknown error')}")
    
    return result

def main():
    if len(sys.argv) < 2:
        print("Usage: python test_client.py <wav_file>")
        print("Example: python test_client.py test_audio.wav")
        sys.exit(1)
    
    wav_file = sys.argv[1]
    
    if not Path(wav_file).exists():
        print(f"Error: File '{wav_file}' not found")
        sys.exit(1)
    
    print(f"Analyzing {wav_file}...")
    analyze_wav_file(wav_file)

if __name__ == "__main__":
    main()