// Enhanced Search with Tag Recovery
class EnhancedSearch {
    constructor() {
        this.selectedTags = new Set();
        this.tagSuggestions = [];
        this.searchHistory = [];
        this.savedSearches = this.loadSavedSearches();
        this.currentPage = 1;
        this.resultsPerPage = 20;
        
        this.init();
    }
    
    async init() {
        this.setupEventListeners();
        await this.loadTopics();
        await this.loadTagSuggestions();
        this.renderSavedSearches();
    }
    
    setupEventListeners() {
        // Form submission
        const form = document.getElementById('searchForm');
        form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.performSearch();
        });
        
        // Tag input with autocomplete
        const tagInput = document.getElementById('tagInput');
        tagInput.addEventListener('input', (e) => this.handleTagInput(e));
        tagInput.addEventListener('keydown', (e) => this.handleTagKeydown(e));
        
        // Clear button
        document.getElementById('clearBtn').addEventListener('click', () => this.clearForm());
        
        // Save search button
        document.getElementById('saveSearchBtn').addEventListener('click', () => this.saveSearch());
        
        // Tag suggestions clicks
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.tag-input-container')) {
                this.hideTagSuggestions();
            }
        });
    }
    
    async loadTopics() {
        try {
            const response = await fetch('/api/topics');
            const data = await response.json();
            
            if (data.success) {
                const select = document.getElementById('topics');
                select.innerHTML = '<option value="">All Topics</option>';
                
                // Build hierarchical options
                this.renderTopicOptions(data.data, select, 0);
            }
        } catch (err) {
            console.error('Failed to load topics:', err);
        }
    }
    
    renderTopicOptions(topics, select, level) {
        topics.forEach(topic => {
            const option = document.createElement('option');
            option.value = topic.path;
            option.textContent = '  '.repeat(level) + topic.name;
            if (topic.announcementCount > 0) {
                option.textContent += ` (${topic.announcementCount})`;
            }
            select.appendChild(option);
            
            if (topic.children && topic.children.length > 0) {
                this.renderTopicOptions(topic.children, select, level + 1);
            }
        });
    }
    
    async loadTagSuggestions() {
        // Load popular tags from the server
        try {
            const response = await fetch('/api/tags/popular');
            const data = await response.json();
            
            if (data.success) {
                this.tagSuggestions = data.data;
            }
        } catch (err) {
            // Fallback to common tags
            this.tagSuggestions = [
                'res:720p', 'res:1080p', 'res:4k',
                'genre:action', 'genre:comedy', 'genre:drama',
                'format:mkv', 'format:mp4', 'format:pdf',
                'lang:en', 'lang:es', 'lang:fr',
                'year:2024', 'year:2023',
                'quality:high', 'quality:hdr',
                'source:bluray', 'source:web',
                'type:movie', 'type:series', 'type:documentary'
            ];
        }
    }
    
    handleTagInput(event) {
        const input = event.target;
        const value = input.value.toLowerCase();
        
        if (value.length < 2) {
            this.hideTagSuggestions();
            return;
        }
        
        // Filter suggestions
        const matches = this.tagSuggestions.filter(tag => 
            tag.toLowerCase().includes(value) && !this.selectedTags.has(tag)
        );
        
        this.showTagSuggestions(matches, value);
    }
    
    handleTagKeydown(event) {
        if (event.key === 'Enter') {
            event.preventDefault();
            const value = event.target.value.trim();
            if (value) {
                this.addTag(value);
                event.target.value = '';
                this.hideTagSuggestions();
            }
        }
    }
    
    showTagSuggestions(matches, query) {
        const container = document.getElementById('tagSuggestions');
        container.innerHTML = '';
        
        if (matches.length === 0) {
            container.classList.remove('show');
            return;
        }
        
        matches.slice(0, 10).forEach(tag => {
            const div = document.createElement('div');
            div.className = 'tag-suggestion';
            
            // Highlight matching part
            const regex = new RegExp(`(${this.escapeRegex(query)})`, 'gi');
            div.innerHTML = tag.replace(regex, '<span class="match">$1</span>');
            
            div.addEventListener('click', () => {
                this.addTag(tag);
                document.getElementById('tagInput').value = '';
                this.hideTagSuggestions();
            });
            
            container.appendChild(div);
        });
        
        container.classList.add('show');
    }
    
    hideTagSuggestions() {
        document.getElementById('tagSuggestions').classList.remove('show');
    }
    
    addTag(tag) {
        if (this.selectedTags.has(tag)) return;
        
        this.selectedTags.add(tag);
        this.renderSelectedTags();
    }
    
    removeTag(tag) {
        this.selectedTags.delete(tag);
        this.renderSelectedTags();
    }
    
    renderSelectedTags() {
        const container = document.getElementById('selectedTags');
        container.innerHTML = '';
        
        this.selectedTags.forEach(tag => {
            const span = document.createElement('span');
            span.className = 'selected-tag';
            span.innerHTML = `
                ${tag}
                <span class="remove" data-tag="${tag}">×</span>
            `;
            
            span.querySelector('.remove').addEventListener('click', () => {
                this.removeTag(tag);
            });
            
            container.appendChild(span);
        });
    }
    
    async performSearch(page = 1) {
        this.currentPage = page;
        const startTime = performance.now();
        
        // Build search query
        const query = this.buildSearchQuery();
        
        // Update URL with search params
        this.updateURL(query);
        
        // Show loading state
        this.showLoading();
        
        try {
            const response = await fetch('/api/announcements/search', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(query)
            });
            
            const data = await response.json();
            
            if (data.success) {
                const endTime = performance.now();
                const searchTime = ((endTime - startTime) / 1000).toFixed(2);
                
                this.displayResults(data.data, searchTime);
                this.updateSearchStats(data.data.length, searchTime);
                
                // Learn from search
                if (data.data.length > 0) {
                    this.learnFromSearch(query, data.data);
                }
            }
        } catch (err) {
            console.error('Search failed:', err);
            this.showError('Search failed. Please try again.');
        }
    }
    
    buildSearchQuery() {
        const form = document.getElementById('searchForm');
        const formData = new FormData(form);
        
        return {
            keywords: formData.get('keywords')?.split(' ').filter(k => k) || [],
            includeTags: Array.from(this.selectedTags),
            excludeTags: [],
            tagMode: 'any',
            topics: Array.from(formData.getAll('topics')).filter(t => t),
            includeSubtopics: document.getElementById('includeSubtopics').checked,
            categories: Array.from(formData.getAll('categories')),
            sizeClasses: Array.from(formData.getAll('sizeClasses')),
            since: formData.get('dateFrom') ? new Date(formData.get('dateFrom')).toISOString() : null,
            until: formData.get('dateTo') ? new Date(formData.get('dateTo')).toISOString() : null,
            sortBy: formData.get('sortBy'),
            sortOrder: formData.get('sortOrder'),
            limit: this.resultsPerPage,
            offset: (this.currentPage - 1) * this.resultsPerPage
        };
    }
    
    displayResults(results, searchTime) {
        const container = document.getElementById('searchResults');
        container.innerHTML = '';
        
        if (results.length === 0) {
            container.innerHTML = '<div class="no-results">No announcements found matching your search.</div>';
            document.getElementById('pagination').style.display = 'none';
            return;
        }
        
        results.forEach(result => {
            container.appendChild(this.createResultElement(result));
        });
        
        // Update pagination
        this.updatePagination(results.length);
    }
    
    createResultElement(result) {
        const template = document.getElementById('resultTemplate');
        const element = template.content.cloneNode(true);
        
        // Category and size badges
        element.querySelector('.category-badge').textContent = result.category;
        element.querySelector('.category-badge').className = `category-badge ${result.category}`;
        element.querySelector('.size-badge').textContent = result.sizeClass;
        
        // Timestamp
        const time = new Date(result.timestamp);
        element.querySelector('.timestamp').textContent = time.toLocaleString();
        
        // Descriptor
        element.querySelector('.descriptor-cid').textContent = result.descriptor;
        element.querySelector('.copy-btn').addEventListener('click', () => {
            navigator.clipboard.writeText(result.descriptor);
        });
        
        // Topic
        element.querySelector('.topic-path').textContent = result.topic || result.topicHash;
        
        // Tags
        const tagsContainer = element.querySelector('.tags');
        if (result.tags && result.tags.length > 0) {
            result.tags.forEach(tag => {
                const span = document.createElement('span');
                span.className = 'tag';
                span.textContent = tag;
                tagsContainer.appendChild(span);
            });
        }
        
        // Highlights
        if (result.highlights) {
            const highlights = element.querySelector('.highlights');
            highlights.innerHTML = this.formatHighlights(result.highlights);
        } else {
            element.querySelector('.highlights').remove();
        }
        
        // Actions
        element.querySelector('.similar-btn').addEventListener('click', () => {
            this.findSimilar(result.id);
        });
        
        return element;
    }
    
    formatHighlights(highlights) {
        // Format search result highlights
        let html = '';
        for (const [field, snippets] of Object.entries(highlights)) {
            snippets.forEach(snippet => {
                html += `${field}: ${snippet}<br>`;
            });
        }
        return html;
    }
    
    updateSearchStats(resultCount, searchTime) {
        document.getElementById('resultCount').textContent = `${resultCount} results`;
        document.getElementById('searchTime').textContent = `in ${searchTime}s`;
        
        // Get tag recovery stats if available
        this.updateTagRecoveryStats();
        
        document.getElementById('searchStats').style.display = 'flex';
    }
    
    async updateTagRecoveryStats() {
        try {
            const response = await fetch('/api/search/tag-recovery-stats');
            const data = await response.json();
            
            if (data.success && data.data.enabled !== false) {
                const stats = data.data;
                const el = document.getElementById('tagRecoveryStats');
                el.textContent = `Tag Recovery: ${stats.dictionary_size} tags, ${Math.round(stats.success_rate * 100)}% accuracy`;
            }
        } catch (err) {
            // Ignore errors
        }
    }
    
    updatePagination(resultCount) {
        const container = document.getElementById('pagination');
        container.innerHTML = '';
        
        if (resultCount < this.resultsPerPage) {
            container.style.display = 'none';
            return;
        }
        
        container.style.display = 'flex';
        
        // Previous button
        const prevBtn = document.createElement('button');
        prevBtn.textContent = 'Previous';
        prevBtn.disabled = this.currentPage === 1;
        prevBtn.addEventListener('click', () => this.performSearch(this.currentPage - 1));
        container.appendChild(prevBtn);
        
        // Page info
        const pageInfo = document.createElement('span');
        pageInfo.className = 'page-info';
        pageInfo.textContent = `Page ${this.currentPage}`;
        container.appendChild(pageInfo);
        
        // Next button
        const nextBtn = document.createElement('button');
        nextBtn.textContent = 'Next';
        nextBtn.disabled = resultCount < this.resultsPerPage;
        nextBtn.addEventListener('click', () => this.performSearch(this.currentPage + 1));
        container.appendChild(nextBtn);
    }
    
    async findSimilar(announcementId) {
        // Implement find similar functionality
        console.log('Finding similar to:', announcementId);
    }
    
    async learnFromSearch(query, results) {
        // Send learning data to server
        try {
            await fetch('/api/search/learn', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    query: query,
                    selectedResults: results.slice(0, 5) // Top 5 results
                })
            });
        } catch (err) {
            // Ignore learning errors
        }
    }
    
    clearForm() {
        document.getElementById('searchForm').reset();
        this.selectedTags.clear();
        this.renderSelectedTags();
        document.getElementById('searchResults').innerHTML = '';
        document.getElementById('searchStats').style.display = 'none';
        document.getElementById('pagination').style.display = 'none';
    }
    
    saveSearch() {
        const name = prompt('Enter a name for this search:');
        if (!name) return;
        
        const query = this.buildSearchQuery();
        const savedSearch = {
            id: Date.now(),
            name: name,
            query: query,
            timestamp: new Date().toISOString()
        };
        
        this.savedSearches.push(savedSearch);
        this.saveSavedSearches();
        this.renderSavedSearches();
    }
    
    loadSavedSearches() {
        const saved = localStorage.getItem('noisefs-saved-searches');
        return saved ? JSON.parse(saved) : [];
    }
    
    saveSavedSearches() {
        localStorage.setItem('noisefs-saved-searches', JSON.stringify(this.savedSearches));
    }
    
    renderSavedSearches() {
        const container = document.getElementById('savedSearchList');
        container.innerHTML = '';
        
        if (this.savedSearches.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: var(--text-secondary);">No saved searches yet</p>';
            return;
        }
        
        this.savedSearches.forEach(search => {
            const div = document.createElement('div');
            div.className = 'saved-search-item';
            div.innerHTML = `
                <div class="name">${search.name}</div>
                <div class="details">${new Date(search.timestamp).toLocaleDateString()}</div>
            `;
            
            div.addEventListener('click', () => this.loadSavedSearch(search));
            container.appendChild(div);
        });
    }
    
    loadSavedSearch(search) {
        // Load the saved query into the form
        const query = search.query;
        
        document.getElementById('keywords').value = query.keywords.join(' ');
        
        // Load tags
        this.selectedTags = new Set(query.includeTags);
        this.renderSelectedTags();
        
        // Load other fields
        if (query.topics.length > 0) {
            const topicsSelect = document.getElementById('topics');
            Array.from(topicsSelect.options).forEach(option => {
                option.selected = query.topics.includes(option.value);
            });
        }
        
        // Perform search
        this.performSearch();
    }
    
    updateURL(query) {
        const params = new URLSearchParams();
        
        if (query.keywords.length > 0) {
            params.set('q', query.keywords.join(' '));
        }
        if (query.includeTags.length > 0) {
            params.set('tags', query.includeTags.join(','));
        }
        
        const newURL = window.location.pathname + '?' + params.toString();
        window.history.pushState({ query }, '', newURL);
    }
    
    showLoading() {
        document.getElementById('searchResults').innerHTML = '<div class="loading">Searching...</div>';
    }
    
    showError(message) {
        document.getElementById('searchResults').innerHTML = `<div class="error">${message}</div>`;
    }
    
    escapeRegex(string) {
        return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }
}

// Initialize enhanced search when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new EnhancedSearch();
});