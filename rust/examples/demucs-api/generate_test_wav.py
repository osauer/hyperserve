#!/usr/bin/env python3
"""Generate a simple test WAV file for testing the API."""

import wave
import math
import struct

def generate_test_wav(filename="test_audio.wav", duration=5, sample_rate=44100):
    """Generate a test WAV file with multiple frequency components."""
    
    num_samples = int(sample_rate * duration)
    
    # Write WAV file
    with wave.open(filename, 'w') as wav:
        wav.setnchannels(2)  # Stereo
        wav.setsampwidth(2)  # 16-bit
        wav.setframerate(sample_rate)
        
        # Generate samples
        for i in range(num_samples):
            t = i / sample_rate
            
            # Create a signal with multiple components
            # Bass frequency (100 Hz)
            bass = 0.3 * math.sin(2 * math.pi * 100 * t)
            
            # Mid frequency (440 Hz - A4)
            mid = 0.4 * math.sin(2 * math.pi * 440 * t)
            
            # High frequency (2000 Hz)
            high = 0.2 * math.sin(2 * math.pi * 2000 * t)
            
            # Add some rhythm (4/4 beat at 120 BPM)
            beat_freq = 2  # 2 Hz = 120 BPM
            envelope = 0.5 * (1 + math.sin(2 * math.pi * beat_freq * t))
            
            # Combine signals
            signal = (bass + mid + high) * envelope * 0.5
            
            # Convert to 16-bit integer
            sample = int(signal * 32767)
            sample = max(-32768, min(32767, sample))  # Clip to valid range
            
            # Pack as stereo (same signal on both channels)
            packed = struct.pack('<hh', sample, sample)
            wav.writeframes(packed)
    
    print(f"Generated {filename}")
    print(f"Duration: {duration} seconds")
    print(f"Sample rate: {sample_rate} Hz")
    print(f"Channels: 2 (stereo)")
    print(f"Bit depth: 16-bit")

if __name__ == "__main__":
    generate_test_wav()