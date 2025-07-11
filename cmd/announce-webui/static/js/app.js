// Main application JavaScript

class AnnouncementUI {
    constructor() {
        this.ws = null;
        this.announcements = [];
        this.filters = {
            category: '',
            sizeClass: ''
        };
        
        this.init();
    }
    
    init() {
        this.setupWebSocket();
        this.setupEventListeners();
        this.loadAnnouncements();
        this.loadStats();
    }
    
    setupWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/ws`;
        
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.updateConnectionStatus(true);
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.updateConnectionStatus(false);
            // Reconnect after 5 seconds
            setTimeout(() => this.setupWebSocket(), 5000);
        };
        
        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            this.handleWebSocketMessage(message);
        };
        
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }
    
    setupEventListeners() {
        // Filter listeners
        document.getElementById('categoryFilter').addEventListener('change', (e) => {
            this.filters.category = e.target.value;
            this.renderAnnouncements();
        });
        
        document.getElementById('sizeFilter').addEventListener('change', (e) => {
            this.filters.sizeClass = e.target.value;
            this.renderAnnouncements();
        });
        
        // Refresh button
        document.getElementById('refreshBtn').addEventListener('click', () => {
            this.loadAnnouncements();
        });
    }
    
    async loadAnnouncements() {
        try {
            const response = await fetch('/api/announcements');
            const data = await response.json();
            
            if (data.success) {
                this.announcements = data.data;
                this.renderAnnouncements();
            }
        } catch (error) {
            console.error('Failed to load announcements:', error);
        }
    }
    
    async loadStats() {
        try {
            const response = await fetch('/api/stats');
            const data = await response.json();
            
            if (data.success) {
                this.updateStats(data.data);
            }
        } catch (error) {
            console.error('Failed to load stats:', error);
        }
    }
    
    renderAnnouncements() {
        const container = document.getElementById('announcementsList');
        container.innerHTML = '';
        
        // Filter announcements
        const filtered = this.announcements.filter(ann => {
            if (this.filters.category && ann.category !== this.filters.category) {
                return false;
            }
            if (this.filters.sizeClass && ann.sizeClass !== this.filters.sizeClass) {
                return false;
            }
            return true;
        });
        
        if (filtered.length === 0) {
            container.innerHTML = '<div class="loading">No announcements found</div>';
            return;
        }
        
        // Sort by timestamp (newest first)
        filtered.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
        
        // Render each announcement
        filtered.forEach(ann => {
            const card = this.createAnnouncementCard(ann);
            container.appendChild(card);
        });
    }
    
    createAnnouncementCard(announcement) {
        const template = document.getElementById('announcementTemplate');
        const card = template.content.cloneNode(true);
        
        // Set category and size badges
        card.querySelector('.category-badge').textContent = announcement.category;
        card.querySelector('.size-badge').textContent = announcement.sizeClass;
        
        // Set timestamp
        const timestamp = new Date(announcement.timestamp);
        card.querySelector('.timestamp').textContent = this.formatTime(timestamp);
        card.querySelector('.timestamp').title = timestamp.toLocaleString();
        
        // Set descriptor
        card.querySelector('.descriptor-cid').textContent = announcement.descriptor;
        
        // Set topic
        const topicEl = card.querySelector('.topic-path');
        if (announcement.topic) {
            topicEl.textContent = announcement.topic;
        } else {
            topicEl.innerHTML = `<code>${announcement.topicHash.substring(0, 16)}...</code>`;
        }
        
        // Set tags
        const tagList = card.querySelector('.tag-list');
        if (announcement.tags && announcement.tags.length > 0) {
            announcement.tags.forEach(tag => {
                const tagEl = document.createElement('span');
                tagEl.className = 'tag';
                tagEl.textContent = tag;
                tagList.appendChild(tagEl);
            });
        } else {
            tagList.innerHTML = '<em>No tags detected</em>';
        }
        
        // Set expiry
        const expiry = new Date(announcement.expiry);
        card.querySelector('.expiry-time').textContent = this.formatTime(expiry);
        
        // Copy button
        card.querySelector('.copy-btn').addEventListener('click', () => {
            navigator.clipboard.writeText(announcement.descriptor);
            // Show feedback
            const btn = event.target;
            btn.textContent = '✓';
            setTimeout(() => btn.textContent = '📋', 1000);
        });
        
        // Download button
        card.querySelector('.download-btn').addEventListener('click', () => {
            this.showDownloadInstructions(announcement.descriptor);
        });
        
        return card;
    }
    
    handleWebSocketMessage(message) {
        switch (message.type) {
            case 'announcement':
                this.handleNewAnnouncement(message.data);
                break;
            case 'stats':
                this.updateStats(message.data);
                break;
        }
    }
    
    handleNewAnnouncement(announcement) {
        // Add to beginning of array
        this.announcements.unshift(announcement);
        
        // Re-render
        this.renderAnnouncements();
        
        // Show notification
        this.showNotification('New announcement received');
    }
    
    updateStats(stats) {
        document.getElementById('totalAnnouncements').textContent = stats.totalAnnouncements || stats.total || '-';
        document.getElementById('activeSubscriptions').textContent = stats.activeSubscriptions || stats.activeSubs || '-';
    }
    
    updateConnectionStatus(connected) {
        const statusEl = document.getElementById('connectionStatus');
        if (connected) {
            statusEl.innerHTML = '<span class="status-indicator online"></span>Connected';
        } else {
            statusEl.innerHTML = '<span class="status-indicator offline"></span>Offline';
        }
    }
    
    formatTime(date) {
        const now = new Date();
        const diff = now - date;
        
        if (diff < 0) {
            // Future time (for expiry)
            const future = -diff;
            if (future < 60000) return 'in ' + Math.floor(future / 1000) + 's';
            if (future < 3600000) return 'in ' + Math.floor(future / 60000) + 'm';
            if (future < 86400000) return 'in ' + Math.floor(future / 3600000) + 'h';
            return 'in ' + Math.floor(future / 86400000) + 'd';
        }
        
        // Past time
        if (diff < 60000) return Math.floor(diff / 1000) + 's ago';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        return Math.floor(diff / 86400000) + 'd ago';
    }
    
    showNotification(message) {
        // Simple notification - could be enhanced with a toast library
        console.log('Notification:', message);
    }
    
    showDownloadInstructions(descriptorCID) {
        alert(`To download this file:\n\n1. Open terminal\n2. Run: noisefs download ${descriptorCID}\n\nMake sure NoiseFS CLI is installed and configured.`);
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new AnnouncementUI();
});