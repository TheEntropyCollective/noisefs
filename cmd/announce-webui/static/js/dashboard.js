// Dashboard functionality
class Dashboard {
    constructor() {
        this.charts = {};
        this.ws = null;
        this.stats = {
            total: 0,
            today: 0,
            categories: {},
            tags: new Map(),
            timeline: []
        };
        
        this.init();
    }
    
    async init() {
        await this.loadInitialData();
        this.setupCharts();
        this.connectWebSocket();
        this.startPeriodicUpdates();
    }
    
    async loadInitialData() {
        try {
            // Load stats
            const statsRes = await fetch('/api/stats');
            const statsData = await statsRes.json();
            if (statsData.success) {
                this.updateStats(statsData.data);
            }
            
            // Load recent announcements
            const annRes = await fetch('/api/announcements?limit=50');
            const annData = await annRes.json();
            if (annData.success) {
                this.processAnnouncements(annData.data);
            }
            
            // Load topics
            const topicsRes = await fetch('/api/topics');
            const topicsData = await topicsRes.json();
            if (topicsData.success) {
                this.renderTopicTree(topicsData.data);
            }
            
        } catch (err) {
            console.error('Failed to load initial data:', err);
        }
    }
    
    setupCharts() {
        // Timeline Chart
        const timelineCtx = document.getElementById('timelineChart').getContext('2d');
        this.charts.timeline = new Chart(timelineCtx, {
            type: 'line',
            data: {
                labels: this.generateTimeLabels(),
                datasets: [{
                    label: 'Announcements',
                    data: [],
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59, 130, 246, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false }
                },
                scales: {
                    y: {
                        beginAtZero: true,
                        ticks: { stepSize: 1 }
                    }
                }
            }
        });
        
        // Category Chart
        const categoryCtx = document.getElementById('categoryChart').getContext('2d');
        this.charts.category = new Chart(categoryCtx, {
            type: 'doughnut',
            data: {
                labels: [],
                datasets: [{
                    data: [],
                    backgroundColor: [
                        '#3b82f6',
                        '#8b5cf6',
                        '#f59e0b',
                        '#10b981',
                        '#ef4444',
                        '#6b7280'
                    ]
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: {
                        position: 'bottom',
                        labels: { padding: 20 }
                    }
                }
            }
        });
    }
    
    connectWebSocket() {
        const wsUrl = `ws://${window.location.host}/api/ws`;
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.updateConnectionStatus(true);
        };
        
        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.handleWebSocketMessage(data);
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.updateConnectionStatus(false);
            // Reconnect after 5 seconds
            setTimeout(() => this.connectWebSocket(), 5000);
        };
        
        this.ws.onerror = (err) => {
            console.error('WebSocket error:', err);
        };
    }
    
    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'announcement':
                this.addLiveFeedItem(data.data);
                this.updateRealTimeStats(data.data);
                break;
            case 'stats':
                this.updateStats(data.data);
                break;
        }
    }
    
    updateStats(stats) {
        // Update stat cards
        document.getElementById('totalAnnouncements').textContent = stats.totalAnnouncements || '0';
        document.getElementById('activeSubscriptions').textContent = stats.activeSubs || '0';
        
        // Calculate today's announcements
        const today = stats.recentCount || 0;
        document.getElementById('announcementChange').textContent = `+${today} today`;
        
        // Update category chart
        if (stats.byCategory) {
            const labels = Object.keys(stats.byCategory);
            const data = Object.values(stats.byCategory);
            this.charts.category.data.labels = labels;
            this.charts.category.data.datasets[0].data = data;
            this.charts.category.update();
        }
        
        // Update unique tags count
        const uniqueTags = this.stats.tags.size;
        document.getElementById('uniqueTags').textContent = uniqueTags;
        document.getElementById('tagChange').textContent = 'discovered';
    }
    
    processAnnouncements(announcements) {
        // Process timeline data
        const hourCounts = new Map();
        const now = new Date();
        
        announcements.forEach(ann => {
            // Count by hour
            const annTime = new Date(ann.timestamp);
            const hourKey = new Date(annTime);
            hourKey.setMinutes(0, 0, 0);
            
            const key = hourKey.getTime();
            hourCounts.set(key, (hourCounts.get(key) || 0) + 1);
            
            // Collect tags
            if (ann.tags && ann.tags.length > 0) {
                ann.tags.forEach(tag => {
                    this.stats.tags.set(tag, (this.stats.tags.get(tag) || 0) + 1);
                });
            }
        });
        
        // Update timeline chart
        const labels = this.generateTimeLabels();
        const data = labels.map(label => {
            const time = new Date(label);
            return hourCounts.get(time.getTime()) || 0;
        });
        
        this.charts.timeline.data.datasets[0].data = data;
        this.charts.timeline.update();
        
        // Update tag cloud
        this.renderTagCloud();
        
        // Add recent items to live feed
        announcements.slice(0, 10).forEach(ann => {
            this.addLiveFeedItem(ann, false);
        });
    }
    
    addLiveFeedItem(announcement, animate = true) {
        const feedContainer = document.getElementById('liveFeed');
        const template = document.getElementById('feedItemTemplate');
        const item = template.content.cloneNode(true);
        
        // Format time
        const time = new Date(announcement.timestamp);
        const timeStr = time.toLocaleTimeString('en-US', { 
            hour: '2-digit', 
            minute: '2-digit' 
        });
        
        item.querySelector('.feed-time').textContent = timeStr;
        item.querySelector('.feed-category').textContent = announcement.category;
        item.querySelector('.feed-category').classList.add(announcement.category);
        item.querySelector('.feed-topic').textContent = announcement.topic || announcement.topicHash.substring(0, 8) + '...';
        
        // Add tags
        const tagsContainer = item.querySelector('.feed-tags');
        if (announcement.tags && announcement.tags.length > 0) {
            announcement.tags.forEach(tag => {
                const tagEl = document.createElement('span');
                tagEl.className = 'tag';
                tagEl.textContent = tag;
                tagsContainer.appendChild(tagEl);
            });
        }
        
        // Add to feed
        const feedItem = item.querySelector('.feed-item');
        if (!animate) {
            feedItem.style.animation = 'none';
        }
        
        feedContainer.insertBefore(item, feedContainer.firstChild);
        
        // Limit feed items
        while (feedContainer.children.length > 50) {
            feedContainer.removeChild(feedContainer.lastChild);
        }
    }
    
    renderTagCloud() {
        const container = document.getElementById('tagCloud');
        container.innerHTML = '';
        
        // Sort tags by frequency
        const sortedTags = Array.from(this.stats.tags.entries())
            .sort((a, b) => b[1] - a[1])
            .slice(0, 30);
        
        if (sortedTags.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: var(--text-secondary);">No tags discovered yet</p>';
            return;
        }
        
        // Calculate size classes
        const maxCount = sortedTags[0][1];
        const minCount = sortedTags[sortedTags.length - 1][1];
        const range = maxCount - minCount || 1;
        
        sortedTags.forEach(([tag, count]) => {
            const el = document.createElement('a');
            el.className = 'cloud-tag';
            el.textContent = tag;
            el.href = `/search?tag=${encodeURIComponent(tag)}`;
            
            // Calculate size class (1-5)
            const normalized = (count - minCount) / range;
            const sizeClass = Math.floor(normalized * 4) + 1;
            el.classList.add(`size-${sizeClass}`);
            
            el.addEventListener('click', (e) => {
                e.preventDefault();
                window.location.href = `/search?tags=${encodeURIComponent(tag)}`;
            });
            
            container.appendChild(el);
        });
    }
    
    renderTopicTree(topics) {
        const container = document.getElementById('topicTree');
        container.innerHTML = '';
        
        // Create hierarchical structure
        const tree = this.buildTopicTree(topics);
        
        // Render tree
        this.renderTopicNodes(tree, container, 0);
    }
    
    buildTopicTree(topics) {
        const tree = [];
        const nodeMap = new Map();
        
        // First pass: create all nodes
        topics.forEach(topic => {
            nodeMap.set(topic.path, {
                ...topic,
                children: []
            });
        });
        
        // Second pass: build hierarchy
        topics.forEach(topic => {
            if (!topic.parent) {
                tree.push(nodeMap.get(topic.path));
            } else {
                const parent = nodeMap.get(topic.parent);
                if (parent) {
                    parent.children.push(nodeMap.get(topic.path));
                }
            }
        });
        
        return tree;
    }
    
    renderTopicNodes(nodes, container, level) {
        nodes.forEach(node => {
            const el = document.createElement('div');
            el.className = 'topic-node';
            el.style.marginLeft = `${level * 20}px`;
            el.textContent = node.name;
            
            if (node.subscribed) {
                el.classList.add('subscribed');
            }
            
            if (node.announcementCount > 0) {
                const count = document.createElement('span');
                count.className = 'count';
                count.textContent = node.announcementCount;
                el.appendChild(count);
            }
            
            el.addEventListener('click', () => {
                window.location.href = `/topics?focus=${encodeURIComponent(node.path)}`;
            });
            
            container.appendChild(el);
            
            if (node.children.length > 0) {
                this.renderTopicNodes(node.children, container, level + 1);
            }
        });
    }
    
    updateRealTimeStats(announcement) {
        // Increment total
        const totalEl = document.getElementById('totalAnnouncements');
        const currentTotal = parseInt(totalEl.textContent) || 0;
        totalEl.textContent = currentTotal + 1;
        
        // Update today's count
        this.stats.today++;
        document.getElementById('announcementChange').textContent = `+${this.stats.today} today`;
        
        // Update network activity
        const activityEl = document.getElementById('networkActivity');
        const currentHour = new Date().getHours();
        const activity = parseInt(activityEl.textContent) || 0;
        activityEl.textContent = activity + 1;
        
        // Update tag stats
        if (announcement.tags && announcement.tags.length > 0) {
            announcement.tags.forEach(tag => {
                this.stats.tags.set(tag, (this.stats.tags.get(tag) || 0) + 1);
            });
            document.getElementById('uniqueTags').textContent = this.stats.tags.size;
            
            // Refresh tag cloud periodically
            if (this.stats.tags.size % 10 === 0) {
                this.renderTagCloud();
            }
        }
    }
    
    updateConnectionStatus(connected) {
        const statusEl = document.getElementById('connectionStatus');
        if (connected) {
            statusEl.innerHTML = '<span class="status-indicator online"></span> Online';
        } else {
            statusEl.innerHTML = '<span class="status-indicator offline"></span> Offline';
        }
    }
    
    startPeriodicUpdates() {
        // Update network health every 30 seconds
        setInterval(() => this.updateNetworkHealth(), 30000);
        this.updateNetworkHealth();
        
        // Reset hourly activity counter
        setInterval(() => {
            document.getElementById('networkActivity').textContent = '0';
        }, 3600000);
    }
    
    async updateNetworkHealth() {
        // Simulate network health metrics
        // In production, these would come from actual monitoring
        
        const dhtHealth = 85 + Math.random() * 15;
        const pubsubHealth = 90 + Math.random() * 10;
        const storageUsed = 20 + Math.random() * 30;
        
        // Update DHT health
        document.getElementById('dhtHealth').style.width = `${dhtHealth}%`;
        document.getElementById('dhtValue').textContent = `${Math.round(dhtHealth)}%`;
        
        // Update PubSub health
        document.getElementById('pubsubHealth').style.width = `${pubsubHealth}%`;
        document.getElementById('pubsubValue').textContent = `${Math.round(pubsubHealth)}%`;
        
        // Update storage
        document.getElementById('storageHealth').style.width = `${storageUsed}%`;
        document.getElementById('storageValue').textContent = `${Math.round(storageUsed)}%`;
        
        // Update health bar colors
        const storageBar = document.getElementById('storageHealth');
        storageBar.classList.remove('good', 'warning', 'danger');
        if (storageUsed < 60) {
            storageBar.classList.add('good');
        } else if (storageUsed < 80) {
            storageBar.classList.add('warning');
        } else {
            storageBar.classList.add('danger');
        }
    }
    
    generateTimeLabels() {
        const labels = [];
        const now = new Date();
        
        for (let i = 23; i >= 0; i--) {
            const time = new Date(now);
            time.setHours(now.getHours() - i, 0, 0, 0);
            labels.push(time.toISOString());
        }
        
        return labels;
    }
}

// Initialize dashboard when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new Dashboard();
});