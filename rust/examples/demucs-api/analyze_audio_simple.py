#!/usr/bin/env python3
"""
Simple audio analysis script using only standard library.
This script is embedded in the Rust binary and executed as needed.
"""

import sys
import json
import wave
import math
import struct
from pathlib import Path

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
            
            # Convert bytes to samples
            if wav.getsampwidth() == 2:  # 16-bit
                fmt = '<' + 'h' * (len(frames) // 2)
                audio_data = list(struct.unpack(fmt, frames))
                # Normalize to [-1, 1]
                audio_float = [s / 32768.0 for s in audio_data]
            else:
                # For simplicity, only support 16-bit
                raise ValueError("Only 16-bit WAV files are supported")
            
            # If stereo, convert to mono for analysis
            if wav.getnchannels() == 2:
                # Average left and right channels
                mono = []
                for i in range(0, len(audio_float), 2):
                    mono.append((audio_float[i] + audio_float[i+1]) / 2)
                audio_float = mono
            
            # Energy analysis
            result["analysis"]["energy"] = analyze_energy(audio_float)
            
            # Spectral analysis (simplified)
            result["analysis"]["spectral"] = analyze_spectral_simple(audio_float, wav.getframerate())
            
            # Simple BPM estimation
            result["analysis"]["bpm"] = estimate_bpm_simple(audio_float, wav.getframerate())
            
            # Key estimation (simplified)
            result["analysis"]["key"] = "C major"  # Placeholder
            
            # Stem presence detection (mock)
            result["analysis"]["stems"] = detect_stems_simple(audio_float, wav.getframerate())
            
    except Exception as e:
        result["error"] = str(e)
    
    return result

def analyze_energy(audio):
    """Analyze energy characteristics of the audio."""
    # RMS energy
    sum_squares = sum(s * s for s in audio)
    rms = math.sqrt(sum_squares / len(audio))
    
    # Peak amplitude
    peak = max(abs(s) for s in audio)
    
    # Simple dynamic range
    sorted_audio = sorted(abs(s) for s in audio)
    percentile_95 = sorted_audio[int(len(sorted_audio) * 0.95)]
    percentile_10 = sorted_audio[int(len(sorted_audio) * 0.10)]
    
    if percentile_10 > 0:
        dynamic_range = 20 * math.log10(percentile_95 / percentile_10)
    else:
        dynamic_range = 20.0
    
    return {
        "average": float(rms),
        "peak": float(peak),
        "dynamic_range": float(dynamic_range)
    }

def analyze_spectral_simple(audio, sample_rate):
    """Simple spectral analysis without FFT."""
    # This is a very simplified version
    # We'll estimate brightness based on zero-crossing rate
    
    zero_crossings = 0
    for i in range(1, len(audio)):
        if (audio[i-1] >= 0 and audio[i] < 0) or (audio[i-1] < 0 and audio[i] >= 0):
            zero_crossings += 1
    
    # Zero-crossing rate correlates with spectral centroid
    zcr = zero_crossings / len(audio)
    
    # Estimate spectral features from ZCR
    spectral_centroid = zcr * sample_rate / 4  # Rough approximation
    spectral_rolloff = spectral_centroid * 3    # Rough approximation
    brightness = min(1.0, zcr * 10)              # Normalized brightness
    
    return {
        "brightness": float(brightness),
        "spectral_centroid": float(spectral_centroid),
        "spectral_rolloff": float(spectral_rolloff)
    }

def estimate_bpm_simple(audio, sample_rate):
    """Very simple BPM estimation using peak detection."""
    # Downsample for efficiency
    downsample_factor = 10
    downsampled = audio[::downsample_factor]
    sr_down = sample_rate // downsample_factor
    
    # Simple onset detection using energy changes
    window_size = int(0.05 * sr_down)  # 50ms windows
    if window_size == 0:
        window_size = 1
        
    energy_curve = []
    for i in range(0, len(downsampled) - window_size, window_size):
        window = downsampled[i:i + window_size]
        energy = sum(s * s for s in window)
        energy_curve.append(energy)
    
    if len(energy_curve) < 10:
        return 120.0  # Default BPM
    
    # Find peaks in energy curve
    peaks = []
    for i in range(1, len(energy_curve) - 1):
        if energy_curve[i] > energy_curve[i-1] and energy_curve[i] > energy_curve[i+1]:
            if energy_curve[i] > sum(energy_curve) / len(energy_curve) * 2:
                peaks.append(i)
    
    if len(peaks) < 2:
        return 120.0  # Default BPM
    
    # Calculate average time between peaks
    peak_intervals = []
    for i in range(1, len(peaks)):
        interval = (peaks[i] - peaks[i-1]) * window_size / sr_down
        if 0.3 < interval < 1.0:  # 60-200 BPM range
            peak_intervals.append(interval)
    
    if peak_intervals:
        avg_interval = sum(peak_intervals) / len(peak_intervals)
        bpm = 60.0 / avg_interval
        return float(min(200, max(60, bpm)))
    
    return 120.0  # Default BPM

def detect_stems_simple(audio, sample_rate):
    """Mock stem detection based on simple frequency analysis."""
    # Count zero crossings in different parts of the signal
    # This is a very rough approximation
    
    # Analyze first 10 seconds or whole file
    samples_to_analyze = min(len(audio), int(sample_rate * 10))
    audio_segment = audio[:samples_to_analyze]
    
    # Simple frequency content estimation
    low_energy = 0
    mid_energy = 0
    high_energy = 0
    
    # Use simple filters (moving average)
    window = 100
    for i in range(window, len(audio_segment) - window):
        # Low freq: more averaging
        low = sum(audio_segment[i-window:i+window]) / (2 * window)
        low_energy += low * low
        
        # Mid freq: less averaging
        mid = sum(audio_segment[i-10:i+10]) / 20
        mid_energy += mid * mid
        
        # High freq: difference between samples
        high = abs(audio_segment[i] - audio_segment[i-1])
        high_energy += high
    
    total_energy = low_energy + mid_energy + high_energy
    if total_energy > 0:
        low_ratio = low_energy / total_energy
        mid_ratio = mid_energy / total_energy
        high_ratio = high_energy / total_energy
    else:
        low_ratio = mid_ratio = high_ratio = 0.33
    
    return {
        "vocals": {
            "present": mid_ratio > 0.3,
            "confidence": min(0.9, mid_ratio * 2)
        },
        "drums": {
            "present": high_ratio > 0.2 and low_ratio > 0.2,
            "confidence": min(0.9, (high_ratio + low_ratio))
        },
        "bass": {
            "present": low_ratio > 0.3,
            "confidence": min(0.9, low_ratio * 2)
        },
        "other": {
            "present": True,
            "confidence": 0.7
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