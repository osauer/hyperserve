#!/bin/bash
# Test the Audio Analysis API using curl

if [ $# -eq 0 ]; then
    echo "Usage: $0 <wav_file>"
    echo "Example: $0 test_audio.wav"
    exit 1
fi

WAV_FILE="$1"
API_URL="http://127.0.0.1:8084/analyze"

if [ ! -f "$WAV_FILE" ]; then
    echo "Error: File '$WAV_FILE' not found"
    exit 1
fi

echo "Analyzing $WAV_FILE..."

# Encode WAV file to base64
WAV_B64=$(base64 "$WAV_FILE" | tr -d '\n')

# Create JSON payload
JSON_PAYLOAD=$(cat <<EOF
{
  "audio": "$WAV_B64",
  "filename": "$(basename "$WAV_FILE")"
}
EOF
)

# Send request
curl -X POST "$API_URL" \
  -H "Content-Type: application/json" \
  -d "$JSON_PAYLOAD" \
  | jq .

# Alternative without jq:
# curl -X POST "$API_URL" \
#   -H "Content-Type: application/json" \
#   -d "$JSON_PAYLOAD"