// Topics page JavaScript

class TopicsUI {
    constructor() {
        this.currentPath = '';
        this.topics = [];
        this.subscriptions = new Set();
        
        this.init();
    }
    
    init() {
        this.loadSubscriptions();
        this.loadTopics('');
        this.setupEventListeners();
    }
    
    setupEventListeners() {
        // Breadcrumb clicks
        document.getElementById('breadcrumb').addEventListener('click', (e) => {
            if (e.target.tagName === 'A') {
                e.preventDefault();
                const path = e.target.dataset.path;
                this.navigateToPath(path);
            }
        });
    }
    
    async loadSubscriptions() {
        try {
            const response = await fetch('/api/subscriptions');
            const data = await response.json();
            
            if (data.success) {
                this.subscriptions = new Set(data.data);
            }
        } catch (error) {
            console.error('Failed to load subscriptions:', error);
        }
    }
    
    async loadTopics(parentPath) {
        try {
            const url = `/api/topics${parentPath ? `?parent=${encodeURIComponent(parentPath)}` : ''}`;
            const response = await fetch(url);
            const data = await response.json();
            
            if (data.success) {
                this.currentPath = parentPath;
                this.topics = data.data;
                this.renderTopics();
                this.updateBreadcrumb();
            }
        } catch (error) {
            console.error('Failed to load topics:', error);
        }
    }
    
    renderTopics() {
        const container = document.getElementById('topicsGrid');
        container.innerHTML = '';
        
        if (this.topics.length === 0) {
            container.innerHTML = '<div class="loading">No topics found</div>';
            return;
        }
        
        this.topics.forEach(topic => {
            const card = this.createTopicCard(topic);
            container.appendChild(card);
        });
    }
    
    createTopicCard(topic) {
        const template = document.getElementById('topicCardTemplate');
        const card = template.content.cloneNode(true);
        const cardEl = card.querySelector('.topic-card');
        
        cardEl.dataset.path = topic.path;
        
        // Set topic name
        card.querySelector('.topic-name').textContent = topic.name;
        
        // Set topic hash
        card.querySelector('.topic-hash code').textContent = topic.hash.substring(0, 16) + '...';
        
        // Set stats
        card.querySelector('.announcement-count').textContent = `${topic.announcementCount} announcements`;
        card.querySelector('.child-count').textContent = `${topic.children.length} subtopics`;
        
        // Subscribe button
        const subscribeBtn = card.querySelector('.subscribe-btn');
        if (this.subscriptions.has(topic.path)) {
            subscribeBtn.classList.add('subscribed');
            subscribeBtn.title = 'Unsubscribe from topic';
        }
        
        subscribeBtn.addEventListener('click', () => {
            this.toggleSubscription(topic.path, subscribeBtn);
        });
        
        // Browse button
        card.querySelector('.browse-btn').addEventListener('click', () => {
            this.navigateToPath(topic.path);
        });
        
        // View announcements button
        card.querySelector('.view-btn').addEventListener('click', () => {
            this.viewTopicAnnouncements(topic);
        });
        
        return card;
    }
    
    async toggleSubscription(topicPath, button) {
        const isSubscribed = this.subscriptions.has(topicPath);
        const endpoint = isSubscribed ? 'unsubscribe' : 'subscribe';
        
        try {
            const response = await fetch(`/api/topics/${encodeURIComponent(topicPath)}/${endpoint}`, {
                method: 'POST'
            });
            
            const data = await response.json();
            
            if (data.success) {
                if (isSubscribed) {
                    this.subscriptions.delete(topicPath);
                    button.classList.remove('subscribed');
                    button.title = 'Subscribe to topic';
                } else {
                    this.subscriptions.add(topicPath);
                    button.classList.add('subscribed');
                    button.title = 'Unsubscribe from topic';
                }
            }
        } catch (error) {
            console.error('Failed to toggle subscription:', error);
        }
    }
    
    navigateToPath(path) {
        this.loadTopics(path);
        if (path) {
            this.clearTopicAnnouncements();
        }
    }
    
    updateBreadcrumb() {
        const breadcrumb = document.getElementById('breadcrumb');
        breadcrumb.innerHTML = '';
        
        // Root link
        const rootLink = document.createElement('a');
        rootLink.href = '#';
        rootLink.dataset.path = '';
        rootLink.textContent = 'Root';
        breadcrumb.appendChild(rootLink);
        
        // Build path parts
        if (this.currentPath) {
            const parts = this.currentPath.split('/');
            let currentPath = '';
            
            parts.forEach((part, index) => {
                // Add separator
                const separator = document.createElement('span');
                separator.textContent = ' / ';
                breadcrumb.appendChild(separator);
                
                // Add link
                currentPath += (index > 0 ? '/' : '') + part;
                const link = document.createElement('a');
                link.href = '#';
                link.dataset.path = currentPath;
                link.textContent = part;
                breadcrumb.appendChild(link);
            });
        }
    }
    
    async viewTopicAnnouncements(topic) {
        const titleEl = document.getElementById('topicTitle');
        const containerEl = document.getElementById('topicAnnouncements');
        
        titleEl.textContent = `Announcements for ${topic.name}`;
        containerEl.innerHTML = '<div class="loading">Loading announcements...</div>';
        
        try {
            const response = await fetch(`/api/announcements?topic=${encodeURIComponent(topic.path)}`);
            const data = await response.json();
            
            if (data.success) {
                this.renderTopicAnnouncements(data.data);
            }
        } catch (error) {
            console.error('Failed to load topic announcements:', error);
            containerEl.innerHTML = '<div class="loading">Failed to load announcements</div>';
        }
    }
    
    renderTopicAnnouncements(announcements) {
        const container = document.getElementById('topicAnnouncements');
        container.innerHTML = '';
        
        if (announcements.length === 0) {
            container.innerHTML = '<div class="loading">No announcements in this topic</div>';
            return;
        }
        
        // Reuse announcement card creation from main UI
        announcements.forEach(ann => {
            const card = this.createAnnouncementCard(ann);
            container.appendChild(card);
        });
    }
    
    clearTopicAnnouncements() {
        document.getElementById('topicTitle').textContent = 'Select a topic to view announcements';
        document.getElementById('topicAnnouncements').innerHTML = '';
    }
    
    // Simplified announcement card (could share with main UI)
    createAnnouncementCard(announcement) {
        const div = document.createElement('div');
        div.className = 'announcement-card';
        
        div.innerHTML = `
            <div class="announcement-header">
                <span class="category-badge">${announcement.category}</span>
                <span class="size-badge">${announcement.sizeClass}</span>
                <time class="timestamp">${this.formatTime(new Date(announcement.timestamp))}</time>
            </div>
            <div class="announcement-body">
                <div class="descriptor">
                    <label>Descriptor:</label>
                    <code class="descriptor-cid">${announcement.descriptor}</code>
                </div>
                <div class="tags">
                    <label>Tags:</label>
                    <div class="tag-list">
                        ${announcement.tags.map(tag => `<span class="tag">${tag}</span>`).join('')}
                    </div>
                </div>
            </div>
        `;
        
        return div;
    }
    
    formatTime(date) {
        const now = new Date();
        const diff = now - date;
        
        if (diff < 60000) return Math.floor(diff / 1000) + 's ago';
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
        return Math.floor(diff / 86400000) + 'd ago';
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new TopicsUI();
});