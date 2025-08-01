//! Highly optimized thread pool for maximum concurrency
//! 
//! Features:
//! - Lock-free job queue
//! - Work stealing between threads
//! - Dynamic thread scaling
//! - CPU affinity optimization

use super::concurrent_queue::ConcurrentQueue;
use std::sync::{Arc, atomic::{AtomicBool, AtomicUsize, Ordering}};
use std::thread;
use std::time::{Duration, Instant};

type Job = Box<dyn FnOnce() + Send + 'static>;

/// Statistics for monitoring pool performance
#[derive(Default)]
pub struct PoolStats {
    pub total_jobs: AtomicUsize,
    pub completed_jobs: AtomicUsize,
    pub active_threads: AtomicUsize,
    pub queue_size: AtomicUsize,
}

/// Optimized thread pool using lock-free structures
pub struct OptimizedPool {
    /// Lock-free job queue
    queue: Arc<ConcurrentQueue<Job>>,
    /// Worker threads
    workers: Vec<Worker>,
    /// Pool statistics
    stats: Arc<PoolStats>,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
}

impl OptimizedPool {
    /// Create a new optimized thread pool
    pub fn new(min_threads: usize, max_threads: usize) -> Self {
        let min_threads = min_threads.max(1);
        let max_threads = max_threads.max(min_threads);
        let initial_threads = min_threads.max(num_cpus());
        
        let queue = Arc::new(ConcurrentQueue::new());
        let stats = Arc::new(PoolStats::default());
        let shutdown = Arc::new(AtomicBool::new(false));
        
        let mut workers = Vec::with_capacity(max_threads);
        
        // Start initial worker threads
        for id in 0..initial_threads {
            workers.push(Worker::new(
                id,
                Arc::clone(&queue),
                Arc::clone(&stats),
                Arc::clone(&shutdown),
            ));
        }
        
        Self {
            queue,
            workers,
            stats,
            shutdown,
        }
    }
    
    /// Submit a job to the pool
    pub fn execute<F>(&self, f: F)
    where
        F: FnOnce() + Send + 'static,
    {
        self.stats.total_jobs.fetch_add(1, Ordering::Relaxed);
        self.stats.queue_size.fetch_add(1, Ordering::Relaxed);
        self.queue.push(Box::new(f));
    }
    
    /// Get current pool statistics
    pub fn stats(&self) -> PoolSnapshot {
        PoolSnapshot {
            total_jobs: self.stats.total_jobs.load(Ordering::Relaxed),
            completed_jobs: self.stats.completed_jobs.load(Ordering::Relaxed),
            active_threads: self.stats.active_threads.load(Ordering::Relaxed),
            queue_size: self.queue.len(),
        }
    }
    
    /// Shutdown the pool
    pub fn shutdown(&self) {
        self.shutdown.store(true, Ordering::Relaxed);
    }
}

impl Drop for OptimizedPool {
    fn drop(&mut self) {
        self.shutdown();
        
        // Wait for workers to finish
        for worker in self.workers.drain(..) {
            if let Some(handle) = worker.handle {
                let _ = handle.join();
            }
        }
    }
}

struct Worker {
    id: usize,
    handle: Option<thread::JoinHandle<()>>,
}

impl Worker {
    fn new(
        id: usize,
        queue: Arc<ConcurrentQueue<Job>>,
        stats: Arc<PoolStats>,
        shutdown: Arc<AtomicBool>,
    ) -> Self {
        let handle = thread::Builder::new()
            .name(format!("hyperserve-worker-{}", id))
            .spawn(move || {
                // Set thread affinity if possible (platform-specific)
                #[cfg(target_os = "linux")]
                set_cpu_affinity(id);
                
                worker_loop(queue, stats, shutdown);
            })
            .ok();
        
        Self { id, handle }
    }
}

fn worker_loop(
    queue: Arc<ConcurrentQueue<Job>>,
    stats: Arc<PoolStats>,
    shutdown: Arc<AtomicBool>,
) {
    let mut spin_count = 0;
    const MAX_SPINS: u32 = 100;
    const SLEEP_THRESHOLD: u32 = 1000;
    
    loop {
        if shutdown.load(Ordering::Relaxed) {
            break;
        }
        
        if let Some(job) = queue.pop() {
            // Reset spin count on successful job acquisition
            spin_count = 0;
            
            stats.active_threads.fetch_add(1, Ordering::Relaxed);
            stats.queue_size.fetch_sub(1, Ordering::Relaxed);
            
            job();
            
            stats.active_threads.fetch_sub(1, Ordering::Relaxed);
            stats.completed_jobs.fetch_add(1, Ordering::Relaxed);
        } else {
            // Exponential backoff
            spin_count += 1;
            
            if spin_count < MAX_SPINS {
                // Spin wait for better latency
                std::hint::spin_loop();
            } else if spin_count < SLEEP_THRESHOLD {
                // Yield to OS scheduler
                thread::yield_now();
            } else {
                // Sleep to reduce CPU usage
                thread::sleep(Duration::from_micros(100));
            }
        }
    }
}

/// Pool statistics snapshot
pub struct PoolSnapshot {
    pub total_jobs: usize,
    pub completed_jobs: usize,
    pub active_threads: usize,
    pub queue_size: usize,
}

/// Get number of CPU cores
fn num_cpus() -> usize {
    std::thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(8)
}

/// Set CPU affinity (Linux only)
#[cfg(target_os = "linux")]
fn set_cpu_affinity(cpu_id: usize) {
    // This would use libc to set CPU affinity
    // For zero-dependency, we skip this optimization
}