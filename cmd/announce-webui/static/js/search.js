// Search page JavaScript

class SearchUI {
    constructor() {
        this.searchForm = document.getElementById('searchForm');
        this.searchResults = document.getElementById('searchResults');
        this.resultsTitle = document.getElementById('resultsTitle');
        
        this.init();
    }
    
    init() {
        this.setupEventListeners();
    }
    
    setupEventListeners() {
        this.searchForm.addEventListener('submit', (e) => {
            e.preventDefault();
            this.performSearch();
        });
        
        this.searchForm.addEventListener('reset', (e) => {
            setTimeout(() => {
                this.clearResults();
            }, 0);
        });
    }
    
    async performSearch() {
        const formData = new FormData(this.searchForm);
        const searchParams = this.buildSearchParams(formData);
        
        this.showLoading();
        
        try {
            const response = await fetch('/api/search', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(searchParams)
            });
            
            const data = await response.json();
            
            if (data.success) {
                this.displayResults(data.data);
            } else {
                this.showError(data.error || 'Search failed');
            }
        } catch (error) {
            console.error('Search error:', error);
            this.showError('Failed to perform search');
        }
    }
    
    buildSearchParams(formData) {
        const params = {};
        
        // Keywords
        const keywords = formData.get('keywords');
        if (keywords) {
            params.keywords = keywords.trim();
        }
        
        // Include tags
        const includeTags = formData.get('includeTags');
        if (includeTags) {
            params.includeTags = includeTags.split(',').map(t => t.trim()).filter(t => t);
        }
        
        // Exclude tags
        const excludeTags = formData.get('excludeTags');
        if (excludeTags) {
            params.excludeTags = excludeTags.split(',').map(t => t.trim()).filter(t => t);
        }
        
        // Topics
        const topics = formData.get('topics');
        if (topics) {
            params.topics = topics.split(',').map(t => t.trim()).filter(t => t);
        }
        
        // Include subtopics
        params.includeSubtopics = formData.get('includeSubtopics') === 'on';
        
        // Categories
        const categories = formData.getAll('categories');
        if (categories.length > 0) {
            params.categories = categories;
        }
        
        // Size classes
        const sizeClasses = formData.getAll('sizeClasses');
        if (sizeClasses.length > 0) {
            params.sizeClasses = sizeClasses;
        }
        
        // Since
        const since = formData.get('since');
        if (since) {
            params.since = since;
        }
        
        // Sort by
        params.sortBy = formData.get('sortBy') || 'relevance';
        
        return params;
    }
    
    showLoading() {
        this.resultsTitle.textContent = 'Searching...';
        this.searchResults.innerHTML = '<div class="loading">Searching announcements...</div>';
    }
    
    displayResults(results) {
        if (!results || results.length === 0) {
            this.resultsTitle.textContent = 'No Results Found';
            this.searchResults.innerHTML = '<div class="loading">No announcements match your search criteria</div>';
            return;
        }
        
        this.resultsTitle.textContent = `Found ${results.length} Results`;
        this.searchResults.innerHTML = '';
        
        results.forEach(ann => {
            const card = this.createAnnouncementCard(ann);
            this.searchResults.appendChild(card);
        });
    }
    
    showError(message) {
        this.resultsTitle.textContent = 'Search Error';
        this.searchResults.innerHTML = `<div class="loading error">${message}</div>`;
    }
    
    clearResults() {
        this.resultsTitle.textContent = 'Results';
        this.searchResults.innerHTML = '';
    }
    
    createAnnouncementCard(announcement) {
        const div = document.createElement('div');
        div.className = 'announcement-card';
        
        // Calculate relevance score display if present
        const relevanceDisplay = announcement.relevanceScore 
            ? `<span class="relevance-score" title="Relevance score">${Math.round(announcement.relevanceScore * 100)}%</span>`
            : '';
        
        div.innerHTML = `
            <div class="announcement-header">
                <span class="category-badge">${announcement.category}</span>
                <span class="size-badge">${announcement.sizeClass}</span>
                ${relevanceDisplay}
                <time class="timestamp">${this.formatTime(new Date(announcement.timestamp))}</time>
            </div>
            <div class="announcement-body">
                <div class="descriptor">
                    <label>Descriptor:</label>
                    <code class="descriptor-cid">${announcement.descriptor}</code>
                    <button class="copy-btn" title="Copy descriptor">📋</button>
                    <button class="btn btn-secondary download-btn">Download</button>
                </div>
                ${announcement.topic ? `
                <div class="topic">
                    <label>Topic:</label>
                    <span class="topic-path">${announcement.topic}</span>
                </div>
                ` : ''}
                <div class="tags">
                    <label>Tags:</label>
                    <div class="tag-list">
                        ${announcement.tags.map(tag => {
                            // Highlight matching tags
                            const isMatch = this.isMatchingTag(tag);
                            return `<span class="tag${isMatch ? ' match' : ''}">${tag}</span>`;
                        }).join('')}
                    </div>
                </div>
                <div class="expiry">
                    <label>Expires:</label>
                    <span class="expiry-time">${this.formatTime(new Date(announcement.expiry))}</span>
                </div>
            </div>
        `;
        
        // Add event listeners
        const copyBtn = div.querySelector('.copy-btn');
        copyBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(announcement.descriptor);
            copyBtn.textContent = '✓';
            setTimeout(() => copyBtn.textContent = '📋', 1000);
        });
        
        const downloadBtn = div.querySelector('.download-btn');
        downloadBtn.addEventListener('click', () => {
            this.showDownloadInstructions(announcement.descriptor);
        });
        
        return div;
    }
    
    isMatchingTag(tag) {
        // Check if this tag was in the search criteria
        const formData = new FormData(this.searchForm);
        const includeTags = formData.get('includeTags');
        
        if (includeTags) {
            const searchTags = includeTags.split(',').map(t => t.trim().toLowerCase());
            return searchTags.some(searchTag => tag.toLowerCase().includes(searchTag));
        }
        
        return false;
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
    
    showDownloadInstructions(descriptorCID) {
        alert(`To download this file:\n\n1. Open terminal\n2. Run: noisefs download ${descriptorCID}\n\nMake sure NoiseFS CLI is installed and configured.`);
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new SearchUI();
});