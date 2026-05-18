// Interactive demo functionality for HyperServe features

let eventSource = null;
let chartData = [];
const maxDataPoints = 20;

// Authentication test
async function testAuth(token) {
    const result = document.getElementById('auth-result');
    result.textContent = 'Testing...';
    
    try {
        const response = await fetch('/api/user', {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });
        
        const data = await response.text();
        result.textContent = `Response (${response.status}):\n${data}`;
        
        if (response.ok) {
            result.style.color = '#155724';
        } else {
            result.style.color = '#721c24';
        }
    } catch (error) {
        result.textContent = `Error: ${error.message}`;
        result.style.color = '#721c24';
    }
}

// SSE streaming
function startSSE() {
    if (eventSource) {
        stopSSE();
    }
    
    const status = document.getElementById('sse-status');
    status.textContent = 'Connecting...';
    status.className = '';
    
    eventSource = new EventSource('/api/stream');
    
    eventSource.onopen = () => {
        status.textContent = 'Connected - receiving real-time updates';
        status.className = 'connected';
    };
    
    eventSource.onmessage = (event) => {
        const data = JSON.parse(event.data);
        updateChart(data);
    };
    
    eventSource.onerror = (error) => {
        status.textContent = 'Connection error - will retry automatically';
        status.className = '';
    };
}

function stopSSE() {
    if (eventSource) {
        eventSource.close();
        eventSource = null;
        document.getElementById('sse-status').textContent = 'Disconnected';
        document.getElementById('sse-status').className = '';
    }
}

// Update chart with SSE data
function updateChart(data) {
    chartData.push({
        cpu: data.cpu,
        memory: data.memory,
        time: new Date(data.time)
    });
    
    if (chartData.length > maxDataPoints) {
        chartData.shift();
    }
    
    drawChart();
}

// Simple canvas chart
function drawChart() {
    const canvas = document.getElementById('chart');
    const ctx = canvas.getContext('2d');
    const width = canvas.width;
    const height = canvas.height;
    
    // Clear canvas
    ctx.clearRect(0, 0, width, height);
    
    if (chartData.length < 2) return;
    
    // Draw grid
    ctx.strokeStyle = '#e0e0e0';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
        const y = (height / 4) * i;
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(width, y);
        ctx.stroke();
    }
    
    // Draw data
    const xStep = width / (maxDataPoints - 1);
    
    // CPU line (blue)
    ctx.strokeStyle = '#0066cc';
    ctx.lineWidth = 2;
    ctx.beginPath();
    chartData.forEach((point, i) => {
        const x = i * xStep;
        const y = height - (point.cpu / 100 * height);
        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.stroke();
    
    // Memory line (green)
    ctx.strokeStyle = '#28a745';
    ctx.beginPath();
    chartData.forEach((point, i) => {
        const x = i * xStep;
        const y = height - (point.memory / 100 * height);
        if (i === 0) {
            ctx.moveTo(x, y);
        } else {
            ctx.lineTo(x, y);
        }
    });
    ctx.stroke();
    
    // Legend
    ctx.font = '12px sans-serif';
    ctx.fillStyle = '#0066cc';
    ctx.fillText('CPU', 10, 20);
    ctx.fillStyle = '#28a745';
    ctx.fillText('Memory', 50, 20);
}

// File upload
async function uploadFile() {
    const input = document.getElementById('file-input');
    const result = document.getElementById('upload-result');
    
    if (!input.files.length) {
        result.textContent = 'Please select a file';
        return;
    }
    
    const formData = new FormData();
    formData.append('file', input.files[0]);
    
    result.textContent = 'Uploading...';
    
    try {
        const response = await fetch('/api/upload', {
            method: 'POST',
            body: formData
        });
        
        const data = await response.text();
        result.textContent = `Response (${response.status}):\n${data}`;
    } catch (error) {
        result.textContent = `Error: ${error.message}`;
    }
}

// Rate limit test - now testing protected endpoint
async function testRateLimit() {
    const result = document.getElementById('rate-limit-result');
    result.textContent = 'Testing rate limits on /api/user (protected endpoint)...\n';
    
    // Use a valid token for the test
    const token = 'demo-token-123';
    
    for (let i = 0; i < 10; i++) {
        try {
            const response = await fetch('/api/user', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            });
            
            const headers = {
                limit: response.headers.get('X-RateLimit-Limit'),
                remaining: response.headers.get('X-RateLimit-Remaining'),
                reset: response.headers.get('X-RateLimit-Reset'),
                retryAfter: response.headers.get('Retry-After')
            };
            
            result.textContent += `Request ${i + 1}: ${response.status}`;
            if (headers.remaining) {
                result.textContent += ` - Remaining: ${headers.remaining}/${headers.limit}`;
            }
            result.textContent += '\n';
            
            if (response.status === 429) {
                result.textContent += `Rate limited! Retry after: ${headers.retryAfter}s\n`;
                result.textContent += '\nNote: SecureAPI middleware stack includes rate limiting for /api/* routes';
                break;
            }
        } catch (error) {
            result.textContent += `Request ${i + 1}: Error - ${error.message}\n`;
        }
        
        // Small delay between requests
        await new Promise(resolve => setTimeout(resolve, 100));
    }
}

// Error test
async function testError() {
    const result = document.getElementById('error-result');
    result.textContent = 'Testing error handling...';
    
    try {
        const response = await fetch('/api/error');
        const data = await response.text();
        result.textContent = `Response (${response.status}):\n${data}\n\n`;
        
        if (response.status === 500) {
            result.textContent += 'Error handled gracefully!\n';
            result.textContent += 'DefaultMiddleware includes RecoveryMiddleware which catches panics.';
        }
    } catch (error) {
        result.textContent = `Network error: ${error.message}`;
    }
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    // Draw empty chart
    const canvas = document.getElementById('chart');
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = '#f0f0f0';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = '#666';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('Start streaming to see real-time data', canvas.width / 2, canvas.height / 2);
});