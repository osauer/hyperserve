#!/usr/bin/env python3
"""
Analyze benchmark results and generate summary report.
"""

import sys
import os
import re
from pathlib import Path
from collections import defaultdict

def parse_wrk_output(content):
    """Parse wrk benchmark output."""
    results = {}
    
    # Extract requests/sec
    req_match = re.search(r'Requests/sec:\s+([\d.]+)', content)
    if req_match:
        results['requests_per_sec'] = float(req_match.group(1))
    
    # Extract latency
    lat_match = re.search(r'Latency\s+([\d.]+)(\w+)\s+([\d.]+)(\w+)\s+([\d.]+)(\w+)', content)
    if lat_match:
        results['latency_avg'] = lat_match.group(1) + lat_match.group(2)
        results['latency_stdev'] = lat_match.group(3) + lat_match.group(4)
        results['latency_max'] = lat_match.group(5) + lat_match.group(6)
    
    # Extract transfer rate
    transfer_match = re.search(r'Transfer/sec:\s+([\d.]+)(\w+)', content)
    if transfer_match:
        results['transfer_per_sec'] = transfer_match.group(1) + transfer_match.group(2)
    
    return results

def parse_ab_output(content):
    """Parse Apache Bench output."""
    results = {}
    
    # Extract requests per second
    req_match = re.search(r'Requests per second:\s+([\d.]+)', content)
    if req_match:
        results['requests_per_sec'] = float(req_match.group(1))
    
    # Extract time per request
    time_match = re.search(r'Time per request:\s+([\d.]+)\s+\[ms\]\s+\(mean\)', content)
    if time_match:
        results['latency_avg'] = time_match.group(1) + 'ms'
    
    # Extract transfer rate
    transfer_match = re.search(r'Transfer rate:\s+([\d.]+)\s+\[Kbytes/sec\]', content)
    if transfer_match:
        results['transfer_per_sec'] = transfer_match.group(1) + 'KB/s'
    
    return results

def analyze_results(results_dir):
    """Analyze all benchmark results in directory."""
    results = defaultdict(dict)
    
    for file_path in Path(results_dir).glob('*.txt'):
        if file_path.name == 'summary.txt':
            continue
            
        with open(file_path, 'r') as f:
            content = f.read()
        
        # Determine parser based on content
        if 'wrk' in content or 'Latency' in content:
            parsed = parse_wrk_output(content)
        else:
            parsed = parse_ab_output(content)
        
        if parsed:
            test_name = file_path.stem
            results[test_name] = parsed
    
    return results

def generate_summary(results):
    """Generate human-readable summary."""
    print("HyperServe Benchmark Results")
    print("=" * 50)
    print()
    
    # Group by test type
    test_pairs = defaultdict(dict)
    for test_name, data in results.items():
        if test_name.startswith('go_'):
            test_type = test_name[3:]
            test_pairs[test_type]['go'] = data
        elif test_name.startswith('rust_'):
            test_type = test_name[5:]
            test_pairs[test_type]['rust'] = data
    
    # Compare results
    for test_type, implementations in sorted(test_pairs.items()):
        print(f"\n{test_type.upper().replace('_', ' ')}:")
        print("-" * 40)
        
        if 'go' in implementations and 'rust' in implementations:
            go_data = implementations['go']
            rust_data = implementations['rust']
            
            # Compare requests/sec
            if 'requests_per_sec' in go_data and 'requests_per_sec' in rust_data:
                go_rps = go_data['requests_per_sec']
                rust_rps = rust_data['requests_per_sec']
                diff_pct = ((rust_rps - go_rps) / go_rps) * 100 if go_rps > 0 else 0
                
                print(f"  Requests/sec:")
                print(f"    Go:   {go_rps:,.0f}")
                print(f"    Rust: {rust_rps:,.0f}")
                print(f"    Difference: {diff_pct:+.1f}%")
            
            # Compare latency
            if 'latency_avg' in go_data and 'latency_avg' in rust_data:
                print(f"  Average Latency:")
                print(f"    Go:   {go_data['latency_avg']}")
                print(f"    Rust: {rust_data['latency_avg']}")
            
            # Compare transfer rate
            if 'transfer_per_sec' in go_data and 'transfer_per_sec' in rust_data:
                print(f"  Transfer Rate:")
                print(f"    Go:   {go_data['transfer_per_sec']}")
                print(f"    Rust: {rust_data['transfer_per_sec']}")
    
    print("\n" + "=" * 50)
    print("\nNOTE: Results may vary based on system load and configuration.")
    print("Run multiple times for more accurate comparisons.")

def main():
    if len(sys.argv) != 2:
        print("Usage: python analyze_results.py <results_directory>")
        sys.exit(1)
    
    results_dir = sys.argv[1]
    if not os.path.exists(results_dir):
        print(f"Error: Directory {results_dir} not found")
        sys.exit(1)
    
    results = analyze_results(results_dir)
    generate_summary(results)

if __name__ == "__main__":
    main()