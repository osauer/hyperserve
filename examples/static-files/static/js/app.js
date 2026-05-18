// Simple JavaScript to demonstrate interaction with the server

document.addEventListener('DOMContentLoaded', function() {
    console.log('HyperServe static files example loaded!');

    // Get references to our elements
    const statusBtn = document.getElementById('status-btn');
    const statusResult = document.getElementById('status-result');

    // Add click handler for the status button
    if (statusBtn) {
        statusBtn.addEventListener('click', checkServerStatus);
    }

    // Function to check server status
    async function checkServerStatus() {
        try {
            // Show loading state
            statusResult.textContent = 'Checking server status...';
            statusResult.classList.add('show');

            // Make request to our custom API endpoint
            const response = await fetch('/api/status');
            const data = await response.json();

            // Display the result
            statusResult.textContent = JSON.stringify(data, null, 2);
            
            // Add a success message
            statusResult.textContent += '\n\n✅ Server is responding!';

        } catch (error) {
            // Handle any errors
            statusResult.textContent = `Error: ${error.message}`;
            statusResult.style.color = 'red';
        }
    }

    // Log that JavaScript is working
    console.log('Static file serving is working correctly!');
    console.log('Try clicking the "Check Server Status" button');
});