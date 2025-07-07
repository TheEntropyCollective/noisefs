document.addEventListener('DOMContentLoaded', function() {
    // Initialize the application
    initializeTheme();
    initializeEventListeners();
    loadMetrics();
    
    // Auto-refresh metrics every 5 seconds
    setInterval(loadMetrics, 5000);
});

function initializeTheme() {
    const themeToggle = document.getElementById('themeToggle');
    const savedTheme = localStorage.getItem('noisefs-theme') || 'light';
    
    // Apply saved theme
    document.body.setAttribute('data-theme', savedTheme);
    updateThemeToggle(savedTheme);
    
    // Theme toggle event listener
    themeToggle.addEventListener('click', function() {
        const currentTheme = document.body.getAttribute('data-theme') || 'light';
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';
        
        document.body.setAttribute('data-theme', newTheme);
        localStorage.setItem('noisefs-theme', newTheme);
        updateThemeToggle(newTheme);
    });
}

function updateThemeToggle(theme) {
    const themeToggle = document.getElementById('themeToggle');
    themeToggle.textContent = theme === 'light' ? '🌙' : '☀️';
}

function initializeEventListeners() {
    // Upload form handler
    const uploadForm = document.getElementById('uploadForm');
    uploadForm.addEventListener('submit', handleUpload);
    
    // Download form handler
    const downloadForm = document.getElementById('downloadForm');
    downloadForm.addEventListener('submit', handleDownload);
}

async function handleUpload(event) {
    event.preventDefault();
    
    const form = event.target;
    const formData = new FormData(form);
    const fileInput = document.getElementById('file');
    const resultDiv = document.getElementById('uploadResult');
    const submitButton = form.querySelector('button[type="submit"]');
    
    // Validate file selection
    if (!fileInput.files.length) {
        showResult(resultDiv, 'Please select a file to upload.', 'error');
        return;
    }
    
    const file = fileInput.files[0];
    
    // Show progress and disable button
    submitButton.disabled = true;
    submitButton.textContent = 'Uploading...';
    showProgress(resultDiv, 'Uploading file...');
    
    try {
        const response = await fetch('/api/upload', {
            method: 'POST',
            body: formData
        });
        
        const result = await response.json();
        
        if (result.success) {
            showUploadSuccess(resultDiv, result);
            form.reset();
            loadMetrics(); // Refresh metrics
        } else {
            showResult(resultDiv, result.error || 'Upload failed', 'error');
        }
    } catch (error) {
        showResult(resultDiv, 'Network error: ' + error.message, 'error');
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = 'Upload';
    }
}

async function handleDownload(event) {
    event.preventDefault();
    
    const form = event.target;
    const formData = new FormData(form);
    const descriptorCID = formData.get('descriptorCID').trim();
    const resultDiv = document.getElementById('downloadResult');
    const submitButton = form.querySelector('button[type="submit"]');
    
    // Validate CID input
    if (!descriptorCID) {
        showResult(resultDiv, 'Please enter a descriptor CID.', 'error');
        return;
    }
    
    // Show progress and disable button
    submitButton.disabled = true;
    submitButton.textContent = 'Downloading...';
    showProgress(resultDiv, 'Downloading file...');
    
    try {
        const response = await fetch(`/api/download?cid=${encodeURIComponent(descriptorCID)}`);
        
        if (response.ok) {
            // Get filename from Content-Disposition header
            const contentDisposition = response.headers.get('Content-Disposition');
            let filename = 'download';
            if (contentDisposition) {
                const filenameMatch = contentDisposition.match(/filename="([^"]+)"/);
                if (filenameMatch) {
                    filename = filenameMatch[1];
                }
            }
            
            // Download the file
            const blob = await response.blob();
            downloadBlob(blob, filename);
            
            showResult(resultDiv, `File "${filename}" downloaded successfully!`, 'success');
            loadMetrics(); // Refresh metrics
        } else {
            const errorData = await response.json().catch(() => ({ error: 'Download failed' }));
            showResult(resultDiv, errorData.error || 'Download failed', 'error');
        }
    } catch (error) {
        showResult(resultDiv, 'Network error: ' + error.message, 'error');
    } finally {
        submitButton.disabled = false;
        submitButton.textContent = 'Download';
    }
}

async function loadMetrics() {
    const metricsDiv = document.getElementById('metrics');
    
    try {
        const response = await fetch('/api/metrics');
        const metrics = await response.json();
        
        if (response.ok) {
            displayMetrics(metricsDiv, metrics);
        } else {
            metricsDiv.innerHTML = '<p class="error">Failed to load metrics</p>';
        }
    } catch (error) {
        metricsDiv.innerHTML = '<p class="error">Error loading metrics: ' + error.message + '</p>';
    }
}

function displayMetrics(container, metrics) {
    const html = `
        <div class="metrics-grid">
            <div class="metric-card">
                <span class="metric-value">${metrics.block_reuse_rate.toFixed(1)}%</span>
                <div class="metric-label">Block Reuse Rate</div>
            </div>
            <div class="metric-card">
                <span class="metric-value">${metrics.cache_hit_rate.toFixed(1)}%</span>
                <div class="metric-label">Cache Hit Rate</div>
            </div>
            <div class="metric-card">
                <span class="metric-value">${metrics.storage_efficiency.toFixed(1)}%</span>
                <div class="metric-label">Storage Overhead</div>
            </div>
            <div class="metric-card">
                <span class="metric-value">${metrics.total_uploads}</span>
                <div class="metric-label">Total Uploads</div>
            </div>
            <div class="metric-card">
                <span class="metric-value">${metrics.total_downloads}</span>
                <div class="metric-label">Total Downloads</div>
            </div>
            <div class="metric-card">
                <span class="metric-value">${metrics.blocks_reused + metrics.blocks_generated}</span>
                <div class="metric-label">Total Blocks</div>
            </div>
        </div>
        
        <div style="margin-top: 20px;">
            <h3>System Statistics</h3>
            <p><strong>Blocks Reused:</strong> ${metrics.blocks_reused} / ${metrics.blocks_reused + metrics.blocks_generated}</p>
            <p><strong>Cache Performance:</strong> ${metrics.cache_hits} hits, ${metrics.cache_misses} misses</p>
            ${metrics.bytes_uploaded_original > 0 ? 
                `<p><strong>Data Processed:</strong> ${formatBytes(metrics.bytes_uploaded_original)} → ${formatBytes(metrics.bytes_stored_ipfs)}</p>` 
                : ''
            }
            <p><strong>Last Updated:</strong> ${new Date(metrics.timestamp).toLocaleTimeString()}</p>
        </div>
    `;
    
    container.innerHTML = html;
}

function showUploadSuccess(container, result) {
    const html = `
        <div class="result success">
            <h4>Upload Successful!</h4>
            <p><strong>File:</strong> ${result.filename} (${formatBytes(result.size)})</p>
            <p><strong>Descriptor CID:</strong></p>
            <div class="cid-display">
                ${result.descriptor_cid}
                <button class="copy-button" onclick="copyToClipboard('${result.descriptor_cid}')">Copy</button>
            </div>
            <p><em>Save this CID to download your file later!</em></p>
        </div>
    `;
    container.innerHTML = html;
}

function showResult(container, message, type) {
    const html = `<div class="result ${type}"><p>${message}</p></div>`;
    container.innerHTML = html;
}

function showProgress(container, message) {
    const html = `
        <div class="result">
            <p>${message}</p>
            <div class="progress">
                <div class="progress-bar" style="width: 100%; animation: pulse 1.5s ease-in-out infinite;"></div>
            </div>
        </div>
    `;
    container.innerHTML = html;
    
    // Add CSS animation if not already present
    if (!document.getElementById('pulse-animation')) {
        const style = document.createElement('style');
        style.id = 'pulse-animation';
        style.textContent = `
            @keyframes pulse {
                0% { opacity: 1; }
                50% { opacity: 0.5; }
                100% { opacity: 1; }
            }
        `;
        document.head.appendChild(style);
    }
}

function downloadBlob(blob, filename) {
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.style.display = 'none';
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);
}

function copyToClipboard(text) {
    navigator.clipboard.writeText(text).then(function() {
        // Show temporary feedback
        const button = event.target;
        const originalText = button.textContent;
        button.textContent = 'Copied!';
        button.style.background = '#48bb78';
        
        setTimeout(() => {
            button.textContent = originalText;
            button.style.background = '';
        }, 2000);
    }).catch(function(err) {
        console.error('Failed to copy: ', err);
        // Fallback for older browsers
        const textArea = document.createElement('textarea');
        textArea.value = text;
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
    });
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}