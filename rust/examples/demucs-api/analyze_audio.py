#!/usr/bin/env python3
"""
Audio analysis script using Demucs and basic signal processing.
This script is embedded in the Rust binary and executed as needed.
"""

import sys
import json
import wave
import math
from pathlib import Path
import warnings
warnings.filterwarnings('ignore')

def analyze_wav_file(file_path):
    """Analyze a WAV file and extract metadata and features."""
    result = {
        "metadata": {},
        "analysis": {
            "bpm": None,
            "key": None,
            "energy": {},
            "spectral": {},
            "stems": {}
        }
    }
    
    try:
        # Read WAV file metadata
        with wave.open(file_path, 'rb') as wav:
            result["metadata"] = {
                "duration": wav.getnframes() / wav.getframerate(),
                "format": "wav",
                "sample_rate": wav.getframerate(),
                "channels": wav.getnchannels(),
                "bit_depth": wav.getsampwidth() * 8,
                "frames": wav.getnframes()
            }
            
            # Read audio data for analysis
            frames = wav.readframes(wav.getnframes())
            audio_data = np.frombuffer(frames, dtype=np.int16)
            
            # Convert to float and normalize
            audio_float = audio_data.astype(np.float32) / 32768.0
            
            # If stereo, convert to mono for analysis
            if wav.getnchannels() == 2:
                audio_float = audio_float.reshape(-1, 2).mean(axis=1)
            
            # Energy analysis
            result["analysis"]["energy"] = analyze_energy(audio_float)
            
            # Spectral analysis
            result["analysis"]["spectral"] = analyze_spectral(audio_float, wav.getframerate())
            
            # Simple BPM estimation
            result["analysis"]["bpm"] = estimate_bpm(audio_float, wav.getframerate())
            
            # Key estimation (simplified)
            result["analysis"]["key"] = estimate_key(audio_float, wav.getframerate())
            
            # Stem presence detection (mock - would use Demucs in real implementation)
            result["analysis"]["stems"] = detect_stems(audio_float, wav.getframerate())
            
    except Exception as e:
        result["error"] = str(e)
    
    return result

def analyze_energy(audio):
    """Analyze energy characteristics of the audio."""
    # RMS energy
    rms = np.sqrt(np.mean(audio ** 2))
    
    # Peak amplitude
    peak = np.max(np.abs(audio))
    
    # Dynamic range (simplified)
    sorted_audio = np.sort(np.abs(audio))
    percentile_95 = sorted_audio[int(len(sorted_audio) * 0.95)]
    percentile_10 = sorted_audio[int(len(sorted_audio) * 0.10)]
    
    dynamic_range = 20 * np.log10(percentile_95 / (percentile_10 + 1e-10))
    
    return {
        "average": float(rms),
        "peak": float(peak),
        "dynamic_range": float(dynamic_range)
    }

def analyze_spectral(audio, sample_rate):
    """Analyze spectral characteristics."""
    # Compute FFT
    fft = np.fft.rfft(audio)
    magnitude = np.abs(fft)
    freqs = np.fft.rfftfreq(len(audio), 1/sample_rate)
    
    # Spectral centroid
    centroid = np.sum(freqs * magnitude) / np.sum(magnitude)
    
    # Spectral rolloff (95% of energy)
    cumsum = np.cumsum(magnitude)
    rolloff_idx = np.where(cumsum >= 0.95 * cumsum[-1])[0][0]
    rolloff = freqs[rolloff_idx]
    
    # Brightness (ratio of energy above 1500 Hz)
    brightness_idx = np.where(freqs >= 1500)[0][0]
    brightness = np.sum(magnitude[brightness_idx:]) / np.sum(magnitude)
    
    return {
        "brightness": float(brightness),
        "spectral_centroid": float(centroid),
        "spectral_rolloff": float(rolloff)
    }

def estimate_bpm(audio, sample_rate):
    """Simple BPM estimation using autocorrelation."""
    # Downsample for efficiency
    downsample_factor = 4
    audio_down = audio[::downsample_factor]
    sr_down = sample_rate // downsample_factor
    
    # Apply onset detection (simplified - just use energy)
    window_size = int(0.01 * sr_down)  # 10ms windows
    hop_size = window_size // 2
    
    energy = []
    for i in range(0, len(audio_down) - window_size, hop_size):
        window = audio_down[i:i + window_size]
        energy.append(np.sum(window ** 2))
    
    energy = np.array(energy)
    
    # Compute autocorrelation
    autocorr = np.correlate(energy, energy, mode='full')
    autocorr = autocorr[len(autocorr)//2:]
    
    # Find peaks in autocorrelation (potential tempo periods)
    min_period = int(60 / 200 * sr_down / hop_size)  # 200 BPM max
    max_period = int(60 / 60 * sr_down / hop_size)   # 60 BPM min
    
    if max_period < len(autocorr):
        autocorr_slice = autocorr[min_period:max_period]
        if len(autocorr_slice) > 0:
            peak_idx = np.argmax(autocorr_slice) + min_period
            bpm = 60 / (peak_idx * hop_size / sr_down)
            return float(np.clip(bpm, 60, 200))
    
    return 120.0  # Default BPM

def estimate_key(audio, sample_rate):
    """Simplified key estimation using chromagram."""
    # This is a very simplified version
    # In practice, you'd use a proper key detection algorithm
    
    # Compute chromagram (12 pitch classes)
    fft = np.fft.rfft(audio)
    magnitude = np.abs(fft)
    freqs = np.fft.rfftfreq(len(audio), 1/sample_rate)
    
    # Map frequencies to pitch classes
    A4 = 440.0
    pitch_classes = np.zeros(12)
    
    for i, freq in enumerate(freqs):
        if freq > 0:
            # Convert frequency to MIDI note number
            midi = 69 + 12 * np.log2(freq / A4)
            pitch_class = int(midi) % 12
            if 0 <= pitch_class < 12:
                pitch_classes[pitch_class] += magnitude[i]
    
    # Simple key detection based on major scale template
    major_template = [1, 0, 1, 0, 1, 1, 0, 1, 0, 1, 0, 1]
    keys = ['C', 'C#', 'D', 'D#', 'E', 'F', 'F#', 'G', 'G#', 'A', 'A#', 'B']
    
    best_key = 'C'
    best_score = 0
    
    for i in range(12):
        # Rotate template
        template = major_template[i:] + major_template[:i]
        score = np.dot(pitch_classes, template)
        if score > best_score:
            best_score = score
            best_key = keys[i]
    
    return f"{best_key} major"

def detect_stems(audio, sample_rate):
    """Mock stem detection (would use Demucs in real implementation)."""
    # This is a placeholder that analyzes frequency content
    # to make educated guesses about stem presence
    
    fft = np.fft.rfft(audio)
    magnitude = np.abs(fft)
    freqs = np.fft.rfftfreq(len(audio), 1/sample_rate)
    
    # Analyze frequency bands
    bass_band = (magnitude[freqs < 250].sum() / magnitude.sum()) > 0.1
    mid_band = (magnitude[(freqs > 250) & (freqs < 2000)].sum() / magnitude.sum()) > 0.3
    high_band = (magnitude[freqs > 2000].sum() / magnitude.sum()) > 0.2
    
    return {
        "vocals": {
            "present": mid_band and high_band,
            "confidence": 0.75 if (mid_band and high_band) else 0.25
        },
        "drums": {
            "present": bass_band and high_band,
            "confidence": 0.80 if (bass_band and high_band) else 0.30
        },
        "bass": {
            "present": bass_band,
            "confidence": 0.85 if bass_band else 0.20
        },
        "other": {
            "present": True,
            "confidence": 0.70
        }
    }

def main():
    if len(sys.argv) != 2:
        print(json.dumps({"error": "Usage: analyze_audio.py <audio_file>"}))
        sys.exit(1)
    
    file_path = sys.argv[1]
    
    if not Path(file_path).exists():
        print(json.dumps({"error": f"File not found: {file_path}"}))
        sys.exit(1)
    
    # Analyze the audio file
    result = analyze_wav_file(file_path)
    
    # Output JSON result
    print(json.dumps(result, indent=2))

if __name__ == "__main__":
    main()