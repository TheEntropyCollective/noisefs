# NoiseFS Search Service Design

## Overview

The NoiseFS Search Service adds powerful search capabilities to directories stored in NoiseFS. This design leverages the existing FileIndex and DirectoryCache infrastructure while providing fast, secure, and comprehensive search functionality.

## Library Selection: Bleve

After extensive research, **Bleve** was selected as the search indexing library for the following reasons:

### Why Bleve?
1. **Native Go Implementation**: No CGO dependencies, pure Go ecosystem compatibility
2. **Proven Maturity**: Used by established companies like Couchbase
3. **Feature Complete**: Full-text search, faceting, highlighting, multiple analyzers
4. **NoiseFS Integration**: Fits well with existing Go architecture and patterns
5. **Deployment Simplicity**: Single binary with no external dependencies

### Alternatives Considered
- **Tantivy-go**: Better performance but requires CGO and Rust dependencies
- **ZincSearch**: Server-based solution, too heavyweight for embedded use
- **Sonic**: Minimal features, requires external document storage

## Architecture Design

### 1. Search Index Storage Strategy

```
~/.noisefs/
├── index.json          # Existing file index
├── search/             # New search directory
│   ├── content.bleve   # Bleve content index
│   ├── metadata.db     # SQLite metadata cache
│   └── config.json     # Search configuration
```

### 2. Multi-Layer Search Architecture

**Layer 1: Metadata Search (Fast)**
- Stored in FileIndex extensions
- Filename, path, size, date filtering
- Instant response (<10ms)

**Layer 2: Content Preview Search (Medium)**
- Stored in SQLite database
- File type, content snippets, tags
- Fast response (<100ms)

**Layer 3: Full-Text Search (Complete)**
- Stored in Bleve index
- Complete file content analysis
- Comprehensive response (<1s)

### 3. Index Update Strategy

**Real-Time Updates:**
- Hook into FileIndex.AddFile() and AddDirectory()
- Immediate metadata indexing
- Queued content indexing

**Background Processing:**
- Content extraction and full-text indexing
- Batch processing for efficiency
- Periodic re-indexing for consistency

## Implementation Components

### 1. SearchManager Structure

```go
type SearchManager struct {
    // Core components
    fileIndex    *FileIndex
    contentIndex bleve.Index
    metadataDB   *sql.DB
    
    // Processing
    indexQueue   chan IndexRequest
    workers      sync.WaitGroup
    
    // Configuration
    config       SearchConfig
    mutex        sync.RWMutex
}

type SearchConfig struct {
    MaxIndexSize     int64         // Maximum index size
    Workers          int           // Background workers
    BatchSize        int           // Batch processing size
    ContentPreview   int           // Preview text length
    SupportedTypes   []string      // Indexable file types
    ReindexInterval  time.Duration // Full reindex interval
}
```

### 2. Search API Interface

```go
type SearchService interface {
    // Basic search operations
    Search(query string, options SearchOptions) (SearchResults, error)
    SearchMetadata(filters MetadataFilters) (SearchResults, error)
    
    // Index management
    UpdateIndex(path string, metadata FileMetadata) error
    RemoveFromIndex(path string) error
    RebuildIndex() error
    
    // Status and maintenance
    GetIndexStats() IndexStats
    OptimizeIndex() error
}

type SearchOptions struct {
    MaxResults   int
    Offset       int
    SortBy       SortField
    Filters      map[string]interface{}
    Highlight    bool
    IncludeBody  bool
}
```

### 3. Integration Points

**FileIndex Extension:**
```go
// Add to existing IndexEntry
type IndexEntry struct {
    // ... existing fields ...
    
    // Search metadata
    ContentHash     string    `json:"content_hash,omitempty"`
    ContentPreview  string    `json:"preview,omitempty"`
    MimeType        string    `json:"mime_type,omitempty"`
    Tags            []string  `json:"tags,omitempty"`
    IndexedAt       time.Time `json:"indexed_at,omitempty"`
    SearchScore     float64   `json:"search_score,omitempty"`
}
```

**DirectoryCache Integration:**
```go
// Hook into directory cache for content indexing
func (dc *DirectoryCache) onManifestLoaded(cid string, manifest *DirectoryManifest) {
    // Extract directory structure for search
    searchManager.IndexDirectoryStructure(cid, manifest)
}
```

## Security Considerations

### 1. Encryption Handling

**Challenge**: NoiseFS files are encrypted with per-file keys
**Solution**: Encrypted local search index

```go
type EncryptedSearchIndex struct {
    masterKey    []byte           // User's master key
    contentIndex *EncryptedBleve  // Encrypted Bleve index
    metadata     *EncryptedDB     // Encrypted SQLite
}
```

### 2. Privacy Protection

- Search index encrypted with user's master key
- No plaintext content stored in index
- Content extraction only during active decryption
- Search results filtered by access permissions

### 3. Key Management

- Leverage existing NoiseFS encryption infrastructure
- Master key derived from user authentication
- Automatic re-encryption on key rotation

## Performance Optimization

### 1. Indexing Strategy

**Selective Indexing:**
```go
type IndexingRules struct {
    MaxFileSize     int64    // Skip files larger than limit
    SupportedTypes  []string // Only index specific types
    ExcludePatterns []string // Skip patterns like *.tmp
    ContentDepth    int      // How deep to analyze content
}
```

**Background Processing:**
- Queue-based indexing system
- Multiple worker goroutines
- Batch processing for efficiency
- Priority system (new files first)

### 2. Search Optimization

**Multi-Stage Search:**
1. Quick metadata filter
2. Content preview search
3. Full-text search if needed

**Caching:**
- Recent search results cache
- Computed relevance scores
- Pre-computed facets and filters

### 3. Index Management

**Size Management:**
- Maximum index size limits
- Automatic old entry cleanup
- Compression and optimization
- Incremental updates only

## CLI Integration

### Search Command Structure

```bash
# Basic search
noisefs search "keyword"
noisefs search --name "*.pdf" 
noisefs search --content "NoiseFS architecture"

# Advanced filters
noisefs search --size ">1MB" --modified "2024-01-01"
noisefs search --directory "/documents" --type "pdf,docx"
noisefs search --tag "important,work"

# Output formats
noisefs search "keyword" --json
noisefs search "keyword" --count
noisefs search "keyword" --highlight
```

### Search Result Format

```json
{
  "query": "NoiseFS architecture",
  "results": [
    {
      "path": "/documents/noisefs-design.pdf",
      "score": 0.95,
      "size": 2048576,
      "modified": "2024-01-15T10:30:00Z",
      "mime_type": "application/pdf",
      "preview": "NoiseFS implements the OFFSystem architecture...",
      "highlights": ["<em>NoiseFS</em> <em>architecture</em>"],
      "descriptor_cid": "QmXXX..."
    }
  ],
  "total": 15,
  "time_ms": 45,
  "facets": {
    "type": {"pdf": 8, "docx": 4, "txt": 3},
    "size": {"<1MB": 5, "1-10MB": 8, "10MB+": 2}
  }
}
```

## Implementation Phases

### Phase 1: Metadata Search Foundation
1. Extend FileIndex with search metadata
2. Implement basic filename/path search
3. Create search CLI command
4. Add metadata filtering

### Phase 2: Content Preview
1. Add content extraction during upload
2. Store content previews in index
3. Implement preview-based search
4. Add file type detection

### Phase 3: Full-Text Index
1. Integrate Bleve for full-text search
2. Background content indexing
3. Advanced query parsing
4. Search result highlighting

### Phase 4: Advanced Features
1. Faceted search with aggregations
2. Search suggestions and autocomplete
3. Search analytics and optimization
4. Advanced filtering and sorting

## Testing Strategy

### Unit Tests
- Search query parsing and execution
- Index update and removal operations
- Encryption/decryption of search data
- Performance benchmarks

### Integration Tests
- End-to-end search workflows
- Large dataset performance
- Concurrent search operations
- Index corruption recovery

### Performance Tests
- Search latency under load
- Index size growth patterns
- Memory usage optimization
- Background processing efficiency

## Conclusion

This design provides a comprehensive, secure, and performant search solution for NoiseFS directories. By building on the existing infrastructure and using proven technologies like Bleve, we can deliver immediate value while maintaining the security and privacy guarantees that define NoiseFS.