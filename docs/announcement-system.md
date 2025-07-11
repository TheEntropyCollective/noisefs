# NoiseFS Announcement System Documentation

## Overview

The NoiseFS announcement system provides decentralized, privacy-preserving content discovery while maintaining protocol neutrality and minimizing legal liability. This document covers the technical architecture, privacy features, and legal design considerations.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Core Components](#core-components)
3. [Privacy Features](#privacy-features)
4. [Legal Design Considerations](#legal-design-considerations)
5. [Technical Implementation](#technical-implementation)
6. [Usage Guide](#usage-guide)
7. [Security Framework](#security-framework)
8. [Advanced Features](#advanced-features)

## Architecture Overview

The announcement system operates on three key principles:

1. **Protocol Neutrality**: The system doesn't understand or interpret content
2. **Decentralized Operation**: No central authority or control point
3. **Privacy by Design**: User privacy protected at every layer

### High-Level Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Publisher     │     │   Subscriber    │     │   Discoverer    │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                         │
         ▼                       ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Announcement Protocol Layer                   │
├─────────────────┬───────────────────────┬───────────────────────┤
│   DHT Storage   │   PubSub Channels     │   Local Store        │
└─────────────────┴───────────────────────┴───────────────────────┘
         │                       │                         │
         ▼                       ▼                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                          IPFS Network                            │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Announcement Structure

Announcements are minimal, protocol-neutral messages:

```go
type Announcement struct {
    Version    string `json:"v"`      // Protocol version
    Descriptor string `json:"d"`      // File descriptor CID
    TopicHash  string `json:"t"`      // SHA-256 hash of topic
    TagBloom   string `json:"tb"`     // Bloom filter of tags
    Category   string `json:"c"`      // Generic category
    SizeClass  string `json:"s"`      // Size classification
    Timestamp  int64  `json:"ts"`     // Unix timestamp
    TTL        int64  `json:"ttl"`    // Time-to-live in seconds
    Nonce      string `json:"nonce"`  // Unique identifier
}
```

### 2. Topic Hashing

Topics are hashed using SHA-256 to maintain protocol neutrality:

```go
// User creates human-readable topic
topic := "documentaries/nature/ocean"

// System hashes it - protocol only sees hash
topicHash := sha256.Sum256([]byte(topic))
// Result: "e3b0c44298fc1c149afbf4c8996fb92..."
```

**Privacy Benefit**: The protocol cannot determine what content is being shared, only that announcements exist for certain opaque identifiers.

### 3. Tag System with Bloom Filters

Tags enable rich discovery while preserving privacy:

```go
// User tags
tags := []string{"res:4k", "type:educational", "year:2024"}

// Converted to bloom filter
bloomFilter := CreateTagBloom(tags)
// Result: Base64-encoded bloom filter

// Discovery - probabilistic matching
matches := bloomFilter.TestMultiple([]string{"res:4k"})
```

**Key Properties**:
- Tags cannot be extracted from bloom filter
- False positives possible (1-5% rate)
- No false negatives
- Compact representation

### 4. Distribution Mechanisms

#### DHT Storage
- Announcements stored in IPFS DHT
- Key: `/noisefs/announce/{topicHash}/{timestamp}`
- Distributed across network nodes
- No single point of control

#### PubSub Real-time Channels
- Topic-specific channels
- Instant notification to subscribers
- Ephemeral - no persistent storage
- Optional for real-time updates

## Privacy Features

### 1. Plausible Deniability

Users can credibly claim they don't know what content they're helping distribute:

- **Hashed Topics**: Users subscribe to hashes, not readable topics
- **Bloom Filters**: Tag matching is probabilistic
- **No Content Inspection**: System never examines actual files
- **Anonymous Blocks**: File blocks appear as random data

### 2. Metadata Minimization

The system collects minimal metadata:

```go
// What the system stores
- Topic hash (not the topic itself)
- Generic categories (video, audio, document)
- Size classes (small, medium, large)
- Bloom filter (not actual tags)

// What the system does NOT store
- Actual topic names
- File names or titles
- User identities
- IP addresses
- Download history
```

### 3. Tag Privacy

Tags use multiple privacy techniques:

1. **Normalization**: Tags are normalized before hashing
2. **Bloom Filters**: Prevent tag enumeration
3. **No Exact Matching**: Only probabilistic matching
4. **Local Processing**: Tag matching happens locally

Example:
```go
// Original tags
userTags := []string{"Documentary: Ocean Life", "RES:4K", "year:2024"}

// After normalization and bloom filter
// Only this opaque data is shared:
"AWFzZGZhc2RmYXNkZmFzZGZhc2Rm..."
```

## Legal Design Considerations

### 1. Protocol Neutrality

The system is designed to be legally neutral:

- **No Content Knowledge**: Protocol doesn't understand announcements
- **No Curation**: System doesn't recommend or promote content
- **User-Driven**: All discovery is initiated by users
- **Decentralized**: No central entity controls the system

### 2. DMCA Compliance Strategy

While maintaining protocol neutrality:

```go
// Descriptor-level takedowns (not block-level)
if dmcaTakedownReceived {
    // Remove announcement for specific descriptor
    removeAnnouncement(descriptorCID)
    
    // Blocks remain untouched (they serve multiple files)
    // Protocol remains neutral (only removes pointers)
}
```

### 3. Safe Harbor Provisions

The design aligns with safe harbor principles:

1. **No Direct Infringement**: System doesn't copy/distribute content
2. **No Knowledge**: Protocol can't determine content nature
3. **No Financial Benefit**: Open protocol, no monetization
4. **Response Mechanism**: Can remove announcements if needed

### 4. Avoiding Liability Triggers

The system carefully avoids features that could create liability:

**What we DON'T do**:
- Search functionality that returns specific files
- Recommendation algorithms
- Featured or promoted content
- User ratings or reviews
- Direct file previews
- Centralized index

**What we DO**:
- Provide topic subscription
- Enable tag-based filtering
- Allow user-driven discovery
- Maintain protocol neutrality

## Technical Implementation

### 1. Publishing Flow

```go
// 1. User uploads file to NoiseFS
descriptorCID := noiseFS.Upload(file)

// 2. Create announcement
announcement := announce.Create(
    descriptorCID,
    topic: "books/classic/shakespeare",
    tags: ["format:pdf", "lang:en", "public-domain"],
)

// 3. Publish to network
publisher.Publish(announcement)
// - Stores in DHT
// - Broadcasts to PubSub
// - Topics are hashed automatically
```

### 2. Discovery Flow

```go
// 1. Subscribe to topics
subscriber.Subscribe("books/classic/shakespeare")
// Actually subscribes to: sha256("books/classic/shakespeare")

// 2. Receive announcements
handler := func(ann *Announcement) {
    // Validate announcement
    if security.Validate(ann) {
        // Store locally
        store.Add(ann)
    }
}

// 3. Search stored announcements
results := discover.Search(
    tags: ["format:pdf", "public-domain"],
    since: 24*time.Hour,
)
```

### 3. Tag Conventions

Standardized namespaces for interoperability:

```
Media Properties:
- res:720p, res:1080p, res:4k
- fps:24, fps:30, fps:60
- aspect:16:9, aspect:21:9
- vcodec:h264, vcodec:h265
- acodec:aac, acodec:flac

Content Metadata:
- year:2024, decade:2020s
- subject:science, subject:history
- lang:en, lang:es
- source:scan, source:digital

File Properties:
- size:small, size:large
- duration:short, duration:long
- type:video, type:audio
```

## Usage Guide

### Command Line Interface

```bash
# Announce a file
noisefs announce <file> --topic "documents/research" --tags "format:pdf,year:2024"

# Subscribe to topics
noisefs subscribe --add "documents/research"
noisefs subscribe --list
noisefs subscribe --monitor  # Real-time monitoring

# Discover content
noisefs discover --tags "format:pdf,subject:science" --since 7d
noisefs discover --topic "documents/research" --limit 50
```

### Programmatic API

```go
// Create announcement system client
client := announce.NewClient(ipfsClient)

// Subscribe to topics
client.Subscribe("documents/research", func(ann *Announcement) {
    fmt.Printf("New announcement: %s\n", ann.Descriptor)
})

// Search with filters
results, err := client.Search(announce.Query{
    Tags: []string{"format:pdf", "subject:science"},
    Since: time.Now().Add(-7*24*time.Hour),
    Limit: 100,
})
```

## Security Framework

### 1. Validation

All announcements are validated:

```go
validator := announce.NewValidator(config)

// Checks performed:
- Version compatibility
- Descriptor CID format
- Topic hash format (64 hex chars)
- Timestamp reasonableness
- TTL limits (min 1 hour, max 7 days)
- Size limits (max 4KB)
- Required fields presence
```

### 2. Rate Limiting

Prevents spam and abuse:

```go
rateLimiter := announce.NewRateLimiter(config)

// Default limits:
- 10 announcements per minute
- 100 per hour  
- 500 per day
- Burst allowance: 5

// Per-source tracking
sourceID := topicHash + ":" + nonce
if err := rateLimiter.Check(sourceID); err != nil {
    return err // Rate limit exceeded
}
```

### 3. Spam Detection

Multi-layered spam detection:

```go
spamDetector := announce.NewSpamDetector(config)

// Detection methods:
1. Duplicate detection (same content hash)
2. Rapid reannouncement detection
3. Suspicious pattern matching
4. Cross-topic spam identification
5. Anomaly detection (unusual TTL, future timestamps)

// Spam scoring
score := spamDetector.SpamScore(announcement)
if score > 70 {
    reject(announcement)
}
```

### 4. Reputation System

Trust-based filtering:

```go
reputation := announce.NewReputationSystem(config)

// Reputation tracking:
- Initial score: 50 (neutral)
- Valid announcement: +1 point
- Spam/invalid: -5 points
- Time decay: -0.1 points/day inactive

// Trust levels:
- 80-100: Trusted
- 60-79: Good
- 40-59: Neutral
- 20-39: Suspicious
- 0-19: Untrusted

// Usage
if reputation.IsBlacklisted(sourceID) {
    reject(announcement)
}
```

## Advanced Features

### 1. Topic Hierarchy

Organize topics in tree structure:

```go
hierarchy := announce.NewTopicHierarchy()

// Build hierarchy
hierarchy.AddTopic("content")
hierarchy.AddTopic("content/books")
hierarchy.AddTopic("content/books/technical")

// Navigate relationships
children := hierarchy.GetChildren("content")
ancestors := hierarchy.GetAncestors("content/books/technical")
related := hierarchy.GetRelated("content/books/technical")
```

### 2. Cross-Topic Discovery

Find content across related topics:

```go
discovery := announce.NewCrossTopicDiscovery(hierarchy)

// Discover from related topics
results := discovery.DiscoverRelated("content/books/technical", 
    announce.DiscoveryOptions{
        TimeWindow: 7*24*time.Hour,
        MaxResults: 100,
        EnabledRules: []string{"siblings", "semantic"},
    })

// Results include:
- Announcements from content/books/educational (sibling)
- Announcements from content/papers/technical (semantic relation)
```

### 3. Advanced Search

Rich search capabilities:

```go
searchEngine := announce.NewSearchEngine(store, hierarchy)

// Complex queries
results := searchEngine.Search(announce.SearchQuery{
    Keywords: []string{"complete", "annotated"},
    IncludeTags: []string{"format:pdf", "public-domain"},
    ExcludeTags: []string{"draft", "incomplete"},
    Topics: []string{"books/classic"},
    Since: time.Now().Add(-30*24*time.Hour),
    SortBy: announce.SortByRelevance,
    Limit: 50,
})

// Find similar content
similar := searchEngine.SearchSimilar(announcementID, 20)
```

### 4. Aggregation

Combine multiple sources:

```go
aggregator := announce.NewAggregator()

// Add sources
aggregator.AddSource("dht", dhtSource)
aggregator.AddSource("pubsub", pubsubSource)
aggregator.AddSource("federation", federationSource)

// Add filters
aggregator.AddFilter(announce.NewQualityFilter(minQuality))
aggregator.AddFilter(announce.NewLanguageFilter([]string{"en", "es"}))

// Aggregate with deduplication
results := aggregator.Aggregate(announce.AggregationOptions{
    TimeWindow: 24*time.Hour,
    MaxPerSource: 100,
    SortBy: "relevance",
})
```

## Best Practices

### For Users

1. **Use Specific Topics**: More specific topics reduce noise
   - Good: `books/technical/programming/golang`
   - Bad: `books`

2. **Tag Consistently**: Follow conventions for better discovery
   - Use standard namespaces (res:, genre:, year:)
   - Be specific but not identifying

3. **Respect Rate Limits**: Avoid aggressive announcing
   - Announce once per file
   - Use reasonable TTLs

### For Developers

1. **Maintain Neutrality**: Never interpret content
   ```go
   // Bad: Parsing title from announcement
   title := extractTitle(announcement) 
   
   // Good: Display opaque descriptor
   fmt.Printf("File: %s\n", announcement.Descriptor)
   ```

2. **Preserve Privacy**: Process locally when possible
   ```go
   // Bad: Server-side tag matching
   server.SearchByTags(tags)
   
   // Good: Local bloom filter matching
   localStore.MatchTags(announcement.TagBloom, tags)
   ```

3. **Handle Failures Gracefully**: Network is unreliable
   ```go
   // Always have fallbacks
   if pubsub.Failed() {
       fallbackToDHT()
   }
   ```

## Security Considerations

### Attack Vectors and Mitigations

1. **Spam Attacks**
   - Mitigation: Rate limiting, reputation, spam detection
   - Result: Spammers quickly identified and blocked

2. **Sybil Attacks**
   - Mitigation: Reputation requires history
   - Result: New identities start untrusted

3. **Privacy Attacks**
   - Mitigation: Bloom filters, topic hashing
   - Result: Cannot enumerate topics or tags

4. **Censorship**
   - Mitigation: Decentralized architecture
   - Result: No single point to censor

## Conclusion

The NoiseFS announcement system provides a careful balance between functionality and legal safety. By maintaining protocol neutrality, minimizing metadata, and avoiding curation features, the system enables content discovery while protecting both users and operators from legal liability.

The decentralized architecture ensures no single point of failure or control, while privacy features like bloom filters and topic hashing protect user privacy. The comprehensive security framework prevents abuse while maintaining the open nature of the protocol.

This design allows NoiseFS to function as a true peer-to-peer system where users maintain full control over their discovery experience without relying on centralized authorities or recommendation systems.