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
    
    // Stream preview button handler
    const streamPreviewBtn = document.getElementById('streamPreviewBtn');
    streamPreviewBtn.addEventListener('click', handleStreamPreview);
    
    // Download mode change handler
    const downloadModeSelect = document.getElementById('downloadMode');
    downloadModeSelect.addEventListener('change', function() {
        const streamingPreview = document.getElementById('streamingPreview');
        
        if (this.value === 'streaming') {
            streamPreviewBtn.style.display = 'inline-block';
        } else {
            streamPreviewBtn.style.display = 'none';
            streamingPreview.style.display = 'none';
        }
    });
    
    // Encryption toggle handler
    const encryptSelect = document.getElementById('encrypt');
    const passwordGroup = document.getElementById('passwordGroup');
    
    // Initial state
    togglePasswordField();
    
    encryptSelect.addEventListener('change', togglePasswordField);
    
    function togglePasswordField() {
        const isEncrypted = encryptSelect.value === 'encrypted';
        passwordGroup.style.display = isEncrypted ? 'block' : 'none';
        
        const passwordInput = document.getElementById('password');
        passwordInput.required = isEncrypted;
    }
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
    const password = formData.get('downloadPassword') || '';
    const downloadMode = formData.get('downloadMode') || 'traditional';
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
        let url = `/api/download?cid=${encodeURIComponent(descriptorCID)}`;
        if (password) {
            url += `&password=${encodeURIComponent(password)}`;
        }
        if (downloadMode === 'streaming') {
            url += `&stream=true`;
        }
        const response = await fetch(url);
        
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

async function handleStreamPreview(event) {
    const form = document.getElementById('downloadForm');
    const formData = new FormData(form);
    const descriptorCID = formData.get('descriptorCID').trim();
    const password = formData.get('downloadPassword') || '';
    const streamingPreview = document.getElementById('streamingPreview');
    const mediaContainer = document.getElementById('mediaContainer');
    const resultDiv = document.getElementById('downloadResult');
    
    // Validate CID input
    if (!descriptorCID) {
        showResult(resultDiv, 'Please enter a descriptor CID.', 'error');
        return;
    }
    
    try {
        showProgress(resultDiv, 'Loading streaming preview...');
        
        // Build streaming URL
        let streamUrl = `/api/download?cid=${encodeURIComponent(descriptorCID)}&stream=true`;
        if (password) {
            streamUrl += `&password=${encodeURIComponent(password)}`;
        }
        
        // Test if the file can be loaded for streaming
        const testResponse = await fetch(streamUrl, { method: 'HEAD' });
        
        if (testResponse.ok) {
            const contentType = testResponse.headers.get('Content-Type') || '';
            const contentLength = testResponse.headers.get('Content-Length');
            const acceptsRanges = testResponse.headers.get('Accept-Ranges') === 'bytes';
            
            // Create appropriate media element based on content type
            let mediaElement = null;
            
            if (contentType.startsWith('video/')) {
                mediaElement = document.createElement('video');
                mediaElement.controls = true;
                mediaElement.style.maxWidth = '100%';
                mediaElement.style.height = 'auto';
                mediaElement.preload = 'metadata';
            } else if (contentType.startsWith('audio/')) {
                mediaElement = document.createElement('audio');
                mediaElement.controls = true;
                mediaElement.style.width = '100%';
                mediaElement.preload = 'metadata';
            } else if (contentType.startsWith('image/')) {
                mediaElement = document.createElement('img');
                mediaElement.style.maxWidth = '100%';
                mediaElement.style.height = 'auto';
            } else {
                // For other file types, show download link
                mediaElement = document.createElement('div');
                mediaElement.innerHTML = `
                    <p><strong>File Type:</strong> ${contentType}</p>
                    <p><strong>Size:</strong> ${contentLength ? formatBytes(parseInt(contentLength)) : 'Unknown'}</p>
                    <p><strong>Range Requests:</strong> ${acceptsRanges ? 'Supported' : 'Not supported'}</p>
                    <a href="${streamUrl}" target="_blank" class="download-link">Open/Download File</a>
                `;
            }
            
            if (mediaElement.tagName === 'VIDEO' || mediaElement.tagName === 'AUDIO' || mediaElement.tagName === 'IMG') {
                mediaElement.src = streamUrl;
                
                // Add event listeners for media elements
                if (mediaElement.tagName !== 'IMG') {
                    mediaElement.addEventListener('loadstart', () => {
                        showProgress(resultDiv, 'Loading media...');
                    });
                    
                    mediaElement.addEventListener('canplay', () => {
                        showResult(resultDiv, 'Media ready for streaming!', 'success');
                    });
                    
                    mediaElement.addEventListener('error', (e) => {
                        showResult(resultDiv, 'Error loading media: ' + e.message, 'error');
                    });
                } else {
                    mediaElement.onload = () => {
                        showResult(resultDiv, 'Image loaded successfully!', 'success');
                    };
                    mediaElement.onerror = () => {
                        showResult(resultDiv, 'Error loading image', 'error');
                    };
                }
            }
            
            // Clear previous content and add new media element
            mediaContainer.innerHTML = '';
            mediaContainer.appendChild(mediaElement);
            streamingPreview.style.display = 'block';
            
            if (mediaElement.tagName === 'DIV') {
                showResult(resultDiv, 'File information loaded', 'success');
            }
            
        } else {
            const errorData = await testResponse.json().catch(() => ({}));
            showResult(resultDiv, errorData.error || 'Failed to load file for streaming', 'error');
        }
    } catch (error) {
        showResult(resultDiv, 'Error: ' + error.message, 'error');
    }
}

function formatBytes(bytes, decimals = 2) {
    if (bytes === 0) return '0 Bytes';
    
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
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