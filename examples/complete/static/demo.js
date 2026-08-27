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

// Exercise the application-owned gate mounted on the protected /api prefix.
async function testRateLimit() {
    const result = document.getElementById('rate-limit-result');
    const attempts = 250;
    result.textContent = `Sending ${attempts} requests through the /api gate...\n`;

    const token = 'demo-token-123';

    try {
        const responses = await Promise.all(
            Array.from({ length: attempts }, () => fetch('/api/user', {
                headers: {
                    'Authorization': `Bearer ${token}`
                }
            }))
        );
        const limited = responses.filter(response => response.status === 429);
        const accepted = responses.length - limited.length;
        result.textContent += `Accepted: ${accepted}\nRejected with 429: ${limited.length}\n`;
        if (limited.length > 0) {
            const first = limited[0];
            result.textContent += `Retry-After: ${first.headers.get('Retry-After')}s\n`;
            result.textContent += `RateLimit-Reset: ${first.headers.get('RateLimit-Reset')}s\n`;
        } else {
            result.textContent += 'No rejection observed; the gate refilled while requests were in flight.\n';
        }
    } catch (error) {
        result.textContent += `Error: ${error.message}\n`;
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
            result.textContent += 'The recovery wrapper installed by hyperserve.New catches panics.';
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
