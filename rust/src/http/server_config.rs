//! Server configuration and optimization utilities

use std::thread;

/// Calculate optimal thread pool size for server workloads
pub struct ThreadPoolConfig {
    /// Minimum threads (never go below this)
    pub min_threads: usize,
    /// Maximum threads (never exceed this)
    pub max_threads: usize,
    /// Target threads (optimal for most workloads)
    pub target_threads: usize,
}

impl ThreadPoolConfig {
    /// Calculate optimal thread pool configuration
    /// 
    /// Best practices for server applications:
    /// - Reserve 25-50% of cores for system tasks
    /// - Account for I/O vs CPU-bound workloads
    /// - Consider NUMA topology on large systems
    pub fn optimal() -> Self {
        let total_cpus = thread::available_parallelism()
            .map(|n| n.get())
            .unwrap_or(8);
        
        // Reserve cores for system tasks
        // Rule of thumb: Use 50-75% of available cores
        let reserved_cores = (total_cpus as f64 * 0.25).ceil() as usize;
        let available_cores = total_cpus.saturating_sub(reserved_cores).max(1);
        
        // For I/O bound workloads (like HTTP servers), we can use more threads
        // than CPU cores, but not too many to avoid context switching
        let min_threads = available_cores;
        let target_threads = available_cores * 2;  // Good balance
        let max_threads = available_cores * 3;     // Absolute maximum
        
        // Ensure reasonable bounds
        let min_threads = min_threads.max(2).min(64);
        let max_threads = max_threads.max(4).min(256);
        let target_threads = target_threads.max(min_threads).min(max_threads);
        
        Self {
            min_threads,
            max_threads,
            target_threads,
        }
    }
    
    /// Create configuration for CPU-bound workloads
    pub fn cpu_bound() -> Self {
        let total_cpus = thread::available_parallelism()
            .map(|n| n.get())
            .unwrap_or(8);
        
        // For CPU-bound work, use fewer threads to minimize context switching
        let available_cores = (total_cpus as f64 * 0.75).floor() as usize;
        
        Self {
            min_threads: available_cores,
            max_threads: available_cores,
            target_threads: available_cores,
        }
    }
    
    /// Create configuration for development/testing
    pub fn development() -> Self {
        Self {
            min_threads: 2,
            max_threads: 4,
            target_threads: 4,
        }
    }
}

/// Detect system characteristics for optimization
pub struct SystemInfo {
    pub physical_cores: usize,
    pub logical_cores: usize,
    pub numa_nodes: usize,
}

impl SystemInfo {
    /// Detect system information
    pub fn detect() -> Self {
        let logical_cores = thread::available_parallelism()
            .map(|n| n.get())
            .unwrap_or(8);
        
        // Estimate physical cores (hyperthreading usually doubles logical cores)
        // This is a heuristic - proper detection would require platform-specific code
        let physical_cores = if logical_cores > 4 {
            logical_cores / 2
        } else {
            logical_cores
        };
        
        Self {
            physical_cores,
            logical_cores,
            numa_nodes: 1, // Would need platform-specific detection
        }
    }
}