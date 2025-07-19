# NoiseFS Announcement System API Reference

## Package Structure

```
pkg/announce/
├── types.go           # Core types and interfaces
├── creator.go         # Announcement creation
├── topic.go           # Topic hashing and normalization
├── bloom.go           # Bloom filter implementation
├── validation.go      # Announcement validation
├── ratelimit.go       # Rate limiting
├── spam.go            # Spam detection
├── reputation.go      # Reputation tracking
├── hierarchy.go       # Topic hierarchy
├── crossdiscovery.go  # Cross-topic discovery
├── search.go          # Search engine
├── aggregator.go      # Multi-source aggregation
├── security/
│   └── manager.go     # Security coordination
├── dht/
│   ├── publisher.go   # DHT publishing
│   └── subscriber.go  # DHT subscription
├── pubsub/
│   └── realtime.go    # Real-time PubSub
├── store/
│   └── store.go       # Local storage
├── tags/
│   ├── parser.go      # Tag parsing
│   ├── auto.go        # Auto-tagging
│   ├── conventions.go # Tag conventions
│   └── matcher.go     # Tag matching
└── config/
    └── config.go      # Configuration management
```

## Core Types

### Announcement

```go
type Announcement struct {
    Version    string `json:"v"`      // Protocol version (currently "1.0")
    Descriptor string `json:"d"`      // IPFS CID of file descriptor
    TopicHash  string `json:"t"`      // SHA-256 hash of topic (64 chars)
    TagBloom   string `json:"tb"`     // Base64-encoded bloom filter
    Category   string `json:"c"`      // Category: video|audio|document|image|archive|other
    SizeClass  string `json:"s"`      // Size: tiny|small|medium|large|huge
    Timestamp  int64  `json:"ts"`     // Unix timestamp
    TTL        int64  `json:"ttl"`    // Time-to-live in seconds
    Nonce      string `json:"nonce"`  // Random string (8-32 chars)
}
```

### Methods

```go
// Validate checks if announcement is valid
func (a *Announcement) Validate() error

// IsExpired checks if announcement has expired
func (a *Announcement) IsExpired() bool

// GetExpiry returns expiration time
func (a *Announcement) GetExpiry() time.Time
```

## Creating Announcements

### Creator

```go
// Create a new announcement creator
creator := announce.NewCreator()

// Create announcement from file
announcement, err := creator.CreateFromFile(
    descriptorCID,
    filePath,
    announce.CreateOptions{
        Topic:    "documentaries/nature/ocean",
        Tags:     []string{"res:4k", "educational"},
        TTL:      7 * 24 * time.Hour,
        AutoTags: true,  // Extract tags from file metadata
    },
)

// Create manual announcement
announcement, err := creator.Create(
    descriptorCID,
    announce.CreateOptions{
        Topic:     "documents/books/technical",
        Tags:      []string{"lang:en", "format:pdf"},
        Category:  "document",
        SizeClass: "medium",
        TTL:       3 * 24 * time.Hour,
    },
)
```

### Auto-Tagging

```go
// Create auto-tagger
tagger := tags.NewAutoTagger()

// Extract tags from file
extractedTags, err := tagger.ExtractTags(filePath)
// Returns: ["ext:mp4", "res:1080p", "vcodec:h264", "year:2024"]
```

## Topic Management

### Topic Hashing

```go
// Hash a topic
topic := "books/public-domain/shakespeare"
topicHash := announce.HashTopic(topic)
// Result: "a7b9c2d4e6f8..." (64 character hex string)

// Normalize topic before hashing
normalized := announce.NormalizeTopic("Movies/SciFi/Classic")
// Result: "movies/scifi/classic"
```

### Topic Hierarchy

```go
// Create hierarchy
hierarchy := announce.NewTopicHierarchy()

// Add topics
hierarchy.AddTopic("content/books/technical", map[string]string{
    "description": "Technical Books and Documentation",
})

// Navigate hierarchy
node, exists := hierarchy.GetTopic("content/books/technical")
children := hierarchy.GetChildren("content/books")
ancestors := hierarchy.GetAncestors("content/books/technical/programming")
descendants := hierarchy.GetDescendants("content")

// Find related topics
related := hierarchy.GetRelated("content/books/technical", maxDistance)

// Search topics
matches := hierarchy.FindTopics("technical")
```

## Tag System

### Tag Parsing

```go
// Create parser
parser := tags.NewTagParser()

// Parse single tag
tag, err := parser.Parse("res:4k")
// Returns: Tag{Namespace: "res", Value: "4k", Raw: "res:4k"}

// Parse multiple tags
tags, err := parser.ParseMultiple([]string{
    "res:4k",
    "subject:science",
    "opensource",  // Tag without namespace
})

// Validate tag conventions
valid, suggestion := tags.ValidateTagConvention("resolution:4k")
// Returns: false, "Unknown namespace. Did you mean 'res'?"
```

### Tag Matching

```go
// Create matcher
matcher := tags.NewMatcher(tags.MatchAny)

// Match tags
contentTags := []string{"res:4k", "subject:science", "year:2024"}
queryTags := []string{"res:4k", "subject:history"}

matches, err := matcher.Match(contentTags, queryTags)
// Returns: true (at least one tag matches)

// Get match score
score, err := matcher.MatchWithScore(contentTags, queryTags)
// Returns: 0.5 (1 out of 2 query tags match)

// Rank items by tag match
items := []tags.TaggedItem{} // populate with actual items
ranked := matcher.RankByTags(items, queryTags)
```

### Bloom Filters

```go
// Create bloom filter for tags
bloom := announce.CreateTagBloom([]string{
    "res:4k",
    "subject:science",
    "year:2024",
})

// Encode for storage
encoded := bloom.Encode()
// Returns: "AWFzZGZhc2RmYXNkZg..."

// Test tags against bloom filter
matches, matchedTags, err := announce.MatchesTags(
    encodedBloom,
    []string{"res:4k", "genre:drama"},
)
// Returns: true, ["res:4k"], nil
```

## Publishing Announcements

### DHT Publisher

```go
// Create DHT publisher
publisher, err := dht.NewPublisher(dht.PublisherConfig{
    IPFSClient:  ipfsClient,
    IPFSShell:   ipfsShell,
    PublishRate: 1 * time.Minute,  // Rate limit
})

// Publish announcement
ctx := context.Background()
err = publisher.Publish(ctx, announcement)

// Get metrics
metrics := publisher.GetMetrics()
fmt.Printf("Published: %d, Errors: %d\n", 
    metrics.PublishCount, metrics.PublishErrors)
```

### PubSub Publisher

```go
// Create real-time publisher
rtPublisher, err := pubsub.NewRealtimePublisher(ipfsShell)

// Publish to topic channel
err = rtPublisher.Publish(ctx, announcement)

// Check if topic is active
active := rtPublisher.IsTopicActive(topicHash)
```

## Subscribing to Announcements

### DHT Subscriber

```go
// Create DHT subscriber
subscriber, err := dht.NewSubscriber(dht.SubscriberConfig{
    IPFSClient:   ipfsClient,
    IPFSShell:    ipfsShell,
    PollInterval: 30 * time.Second,
})

// Subscribe to topic
err = subscriber.Subscribe(
    "documents/research",
    func(ann *announce.Announcement) error {
        fmt.Printf("New: %s\n", ann.Descriptor)
        return nil
    },
)

// Start monitoring
subscriber.Start()
defer subscriber.Stop()
```

### PubSub Subscriber

```go
// Create real-time subscriber
rtSubscriber, err := pubsub.NewRealtimeSubscriber(ipfsShell)

// Subscribe to real-time updates
err = rtSubscriber.Subscribe(
    "documents/research",
    func(ann *announce.Announcement) error {
        // Handle real-time announcement
        return nil
    },
)

// Start listening
rtSubscriber.Start()
defer rtSubscriber.Stop()
```

## Security Features

### Validation

```go
// Create validator with custom config
validator := announce.NewValidator(&announce.ValidationConfig{
    MaxDescriptorLength: 100,
    MaxTopicLength:      256,
    MaxTagCount:         50,
    MaxTTL:              7 * 24 * time.Hour,
    MinTTL:              1 * time.Hour,
    MaxFutureTime:       5 * time.Minute,
})

// Validate announcement
err := validator.ValidateAnnouncement(announcement)

// Validate JSON before parsing
err = announce.ValidateJSON(jsonBytes)
```

### Rate Limiting

```go
// Create rate limiter
limiter := announce.NewRateLimiter(&announce.RateLimitConfig{
    MaxPerMinute:    10,
    MaxPerHour:      100,
    MaxPerDay:       500,
    BurstSize:       5,
    CleanupInterval: 1 * time.Hour,
})

// Check rate limit
key := announce.RateLimitKey("announce", sourceID)
err := limiter.CheckLimit(key)

// Get status
status := limiter.GetStatus(key)
fmt.Printf("Remaining this hour: %d\n", status.HourRemaining)

// Reset limits for a source
limiter.Reset(key)
```

### Spam Detection

```go
// Create spam detector
detector := announce.NewSpamDetector(&announce.SpamConfig{
    DuplicateWindow:  1 * time.Hour,
    SimilarityWindow: 24 * time.Hour,
    MaxDuplicates:    3,
    SuspiciousPatterns: []string{
        "test", "spam", "xxx",
    },
})

// Check for spam
isSpam, reason := detector.CheckSpam(announcement)

// Get spam score (0-100)
score := detector.SpamScore(announcement)

// Get statistics
stats := detector.GetStats()
fmt.Printf("Unique hashes: %d\n", stats.UniqueHashes)
```

### Reputation System

```go
// Create reputation system
reputation := announce.NewReputationSystem(&announce.ReputationConfig{
    InitialScore:    50.0,
    MaxScore:        100.0,
    MinScore:        0.0,
    DecayRate:       0.1,
    PositiveWeight:  1.0,
    NegativeWeight:  5.0,
    RequiredHistory: 10,
})

// Record events
reputation.RecordPositive(sourceID, "valid_announcement")
reputation.RecordNegative(sourceID, "spam_detected")

// Check reputation
score := reputation.GetScore(sourceID)
trusted := reputation.IsTrusted(sourceID)
blacklisted := reputation.IsBlacklisted(sourceID)
level := reputation.GetTrustLevel(sourceID)
// Returns: "trusted"|"good"|"neutral"|"suspicious"|"untrusted"

// Get detailed reputation
rep, exists := reputation.GetReputation(sourceID)
```

### Security Manager

```go
// Create security manager
manager := security.NewManager(&security.Config{
    ValidationConfig:  announce.DefaultValidationConfig(),
    RateLimitConfig:   announce.DefaultRateLimitConfig(),
    SpamConfig:        announce.DefaultSpamConfig(),
    ReputationConfig:  announce.DefaultReputationConfig(),
    SpamThreshold:     70,
    TrustRequired:     false,
})

// Check announcement (all security features)
err := manager.CheckAnnouncement(announcement, sourceID)

// Get source information
info, err := manager.GetSourceInfo(sourceID)
if err == nil {
    fmt.Printf("Trust level: %s\n", info.TrustLevel)
    fmt.Printf("Rate limit remaining: %d/hour\n", 
        info.RateLimit.HourRemaining)
}

// Get security report
report := manager.SecurityReport()
```

## Discovery Features

### Local Store

```go
// Create announcement store
store, err := store.NewStore(store.StoreConfig{
    DataDir:         "/path/to/announcements",
    MaxAge:          7 * 24 * time.Hour,
    MaxSize:         10000,
    CleanupInterval: 1 * time.Hour,
})

// Add announcement
err = store.Add(announcement, "dht")  // source: "dht" or "pubsub"

// Query announcements
byTopic, err := store.GetByTopic(topicHash)
recent, err := store.GetRecent(time.Now().Add(-24*time.Hour), 100)
byDescriptor, err := store.GetByDescriptor(descriptorCID)

// Search by tags
results, err := store.Search([]string{"res:4k", "genre:scifi"}, 50)

// Get statistics
stats := store.GetStats()
total := stats.Total
byTopicCount := stats.ByTopic
expired := stats.Expired
```

### Search Engine

```go
// Create search engine
engine := announce.NewSearchEngine(store, hierarchy)

// Build search query
sinceTime := time.Now().Add(-30*24*time.Hour)
query := announce.SearchQuery{
    Keywords:    []string{"complete", "edition"},
    IncludeTags: []string{"format:pdf", "public-domain"},
    ExcludeTags: []string{"draft"},
    Topics:      []string{"books/classic"},
    Categories:  []string{"document"},
    Since:       &sinceTime,
    SortBy:      announce.SortByRelevance,
    Limit:       50,
}

// Execute search
results, err := engine.Search(query)

// Find similar content
similar, err := engine.SearchSimilar(announcementID, 20)

// Get search suggestions
suggestions, err := engine.Suggest("sci", 10)
```

### Cross-Topic Discovery

```go
// Create cross-topic discovery
discovery := announce.NewCrossTopicDiscovery(
    hierarchy,
    5 * time.Minute,  // Cache TTL
)

// Register topic subscribers
discovery.RegisterSubscriber("content/books", subscriber)

// Discover from related topics
results, err := discovery.DiscoverRelated(
    "content/books",
    announce.DiscoveryOptions{
        TimeWindow:   7 * 24 * time.Hour,
        MaxResults:   100,
        MaxPerTopic:  20,
        Categories:   []string{"document"},
        RequiredTags: []string{"format:pdf"},
    },
)

// Discover across hierarchy
results, err = discovery.DiscoverAcrossHierarchy(
    "content/books",
    2,  // Depth
    options,
)
```

### Aggregator

```go
// Create aggregator
aggregator := announce.NewAggregator()

// Add sources
aggregator.AddSource("local", localSource)
aggregator.AddSource("remote", remoteSource)

// Add filters
aggregator.AddFilter(&QualityFilter{MinQuality: "720p"})
aggregator.AddFilter(&AgeFilter{MaxAge: 30 * 24 * time.Hour})

// Add transformers
aggregator.AddTransformer(&NormalizeTransformer{})

// Aggregate announcements
results, err := aggregator.Aggregate(announce.AggregationOptions{
    TimeWindow:       24 * time.Hour,
    MaxPerSource:     100,
    Limit:            500,
    SortBy:           "score",
    PreferLargeFiles: true,
    SourceTrust: map[string]float64{
        "local":  1.0,
        "remote": 0.8,
    },
})

// Get metrics
metrics := aggregator.GetMetrics()
```

## Configuration

### Loading Configuration

```go
// Get default config directory
configDir := config.GetConfigDir()
// Returns: ~/.config/noisefs or XDG_CONFIG_HOME/noisefs

// Load subscriptions
subs, err := config.LoadSubscriptions(
    filepath.Join(configDir, "subscriptions.json"),
)

// Add subscription
err = subs.Add(config.Subscription{
    Topic:     "documents/research",
    TopicHash: announce.HashTopic("documents/research"),
    Active:    true,
})

// Save subscriptions
err = config.SaveSubscriptions(path, subs)
```

## Error Handling

Common errors and their handling:

```go
// Validation errors
err := validator.ValidateAnnouncement(ann)
switch {
case errors.Is(err, announce.ErrInvalidVersion):
    // Handle version mismatch
case errors.Is(err, announce.ErrExpired):
    // Handle expired announcement
case errors.Is(err, announce.ErrInvalidCID):
    // Handle invalid descriptor
}

// Rate limit errors
err := limiter.CheckLimit(key)
if err != nil {
    // Extract retry time from error message
    // "rate limit exceeded: retry in 45s"
}

// Network errors
err := publisher.Publish(ctx, ann)
if errors.Is(err, context.DeadlineExceeded) {
    // Handle timeout
}
```

## Performance Considerations

### Bloom Filter Sizing

```go
// Calculate optimal bloom filter size
expectedTags := 20
falsePositiveRate := 0.01

params := announce.BloomParams{
    ExpectedItems: expectedTags,
    FalsePositiveRate: falsePositiveRate,
}
// Results in ~200 bits for 1% false positive rate

bloom := announce.NewBloomFilter(params)
```

### Batch Operations

```go
// Batch announcement processing
announcements := make([]*announce.Announcement, 0, 1000)

// Process in batches to avoid memory issues
for i := 0; i < len(announcements); i += 100 {
    end := i + 100
    if end > len(announcements) {
        end = len(announcements)
    }
    
    batch := announcements[i:end]
    processBatch(batch)
}
```

### Caching

```go
// Cache discovery results
cache := announce.NewAggregatorCache(5 * time.Minute)

// Check cache before expensive operation
if cached := cache.Get(cacheKey); cached != nil {
    return cached
}

// Perform operation and cache result
result := expensiveOperation()
cache.Set(cacheKey, result)
```

## Testing

### Creating Test Announcements

```go
// Create test announcement
ann := &announce.Announcement{
    Version:    "1.0",
    Descriptor: "QmTest...",
    TopicHash:  announce.HashTopic("test/topic"),
    TagBloom:   announce.CreateTagBloom([]string{"test:tag"}).Encode(),
    Category:   "video",
    SizeClass:  "medium",
    Timestamp:  time.Now().Unix(),
    TTL:        3600,
    Nonce:      "test123",
}

// Validate test announcement
err := ann.Validate()
```

### Mocking Components

```go
// Mock announcement store
type mockStore struct{}

func (m *mockStore) GetByTopic(hash string) ([]*announce.Announcement, error) {
    return []*announce.Announcement{testAnn}, nil
}

// Use in tests
engine := announce.NewSearchEngine(&mockStore{}, hierarchy)
```