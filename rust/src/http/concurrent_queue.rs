//! Lock-free concurrent queue implementation
//! Based on Michael & Scott algorithm

use std::sync::atomic::{AtomicPtr, AtomicUsize, Ordering};
use std::ptr;

struct Node<T> {
    value: Option<T>,
    next: AtomicPtr<Node<T>>,
}

/// Lock-free concurrent queue
pub struct ConcurrentQueue<T> {
    head: AtomicPtr<Node<T>>,
    tail: AtomicPtr<Node<T>>,
    size: AtomicUsize,
}

unsafe impl<T: Send> Send for ConcurrentQueue<T> {}
unsafe impl<T: Send> Sync for ConcurrentQueue<T> {}

impl<T> ConcurrentQueue<T> {
    pub fn new() -> Self {
        let node = Box::into_raw(Box::new(Node {
            value: None,
            next: AtomicPtr::new(ptr::null_mut()),
        }));
        
        Self {
            head: AtomicPtr::new(node),
            tail: AtomicPtr::new(node),
            size: AtomicUsize::new(0),
        }
    }
    
    pub fn push(&self, value: T) {
        let new_node = Box::into_raw(Box::new(Node {
            value: Some(value),
            next: AtomicPtr::new(ptr::null_mut()),
        }));
        
        loop {
            let tail = self.tail.load(Ordering::Acquire);
            let next = unsafe { (*tail).next.load(Ordering::Acquire) };
            
            if next.is_null() {
                if unsafe { (*tail).next.compare_exchange(
                    ptr::null_mut(),
                    new_node,
                    Ordering::Release,
                    Ordering::Relaxed
                ).is_ok() } {
                    let _ = self.tail.compare_exchange(
                        tail,
                        new_node,
                        Ordering::Release,
                        Ordering::Relaxed
                    );
                    self.size.fetch_add(1, Ordering::Relaxed);
                    break;
                }
            } else {
                let _ = self.tail.compare_exchange(
                    tail,
                    next,
                    Ordering::Release,
                    Ordering::Relaxed
                );
            }
        }
    }
    
    pub fn pop(&self) -> Option<T> {
        loop {
            let head = self.head.load(Ordering::Acquire);
            let tail = self.tail.load(Ordering::Acquire);
            let next = unsafe { (*head).next.load(Ordering::Acquire) };
            
            if head == tail {
                if next.is_null() {
                    return None;
                }
                let _ = self.tail.compare_exchange(
                    tail,
                    next,
                    Ordering::Release,
                    Ordering::Relaxed
                );
            } else {
                if next.is_null() {
                    continue;
                }
                
                let value = unsafe { (*next).value.take() };
                
                if self.head.compare_exchange(
                    head,
                    next,
                    Ordering::Release,
                    Ordering::Relaxed
                ).is_ok() {
                    self.size.fetch_sub(1, Ordering::Relaxed);
                    unsafe { drop(Box::from_raw(head)); }
                    return value;
                }
            }
        }
    }
    
    pub fn len(&self) -> usize {
        self.size.load(Ordering::Relaxed)
    }
    
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }
}

impl<T> Drop for ConcurrentQueue<T> {
    fn drop(&mut self) {
        while self.pop().is_some() {}
        
        unsafe {
            let head = self.head.load(Ordering::Relaxed);
            if !head.is_null() {
                drop(Box::from_raw(head));
            }
        }
    }
}