# Altruistic Caching Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           NoiseFS Node                                   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    User Interface Layer                          │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐     │   │
│  │  │     CLI      │  │   FUSE FS    │  │    Web UI        │     │   │
│  │  │  -stats      │  │              │  │  /cache-stats    │     │   │
│  │  │  -config     │  │              │  │                  │     │   │
│  │  └──────────────┘  └──────────────┘  └───────────────────┘     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                   │                                     │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                     Altruistic Cache Layer                       │   │
│  │                                                                  │   │
│  │  ┌────────────────────────────────────────────────────────┐     │   │
│  │  │              AltruisticCache Manager                    │     │   │
│  │  │  ┌─────────────────┐  ┌─────────────────┐              │     │   │
│  │  │  │ Space Manager   │  │ Block Categorizer│              │     │   │
│  │  │  │ - MinPersonal   │  │ - Personal      │              │     │   │
│  │  │  │ - Flex Pool     │  │ - Altruistic    │              │     │   │
│  │  │  └─────────────────┘  └─────────────────┘              │     │   │
│  │  │                                                         │     │   │
│  │  │  ┌─────────────────┐  ┌─────────────────┐              │     │   │
│  │  │  │Eviction Manager │  │ Metrics Tracker │              │     │   │
│  │  │  │ - LRU/LFU      │  │ - Hit Rates     │              │     │   │
│  │  │  │ - ValueBased   │  │ - Usage Stats   │              │     │   │
│  │  │  │ - Predictive   │  │ - Contribution  │              │     │   │
│  │  │  └─────────────────┘  └─────────────────┘              │     │   │
│  │  └────────────────────────────────────────────────────────┘     │   │
│  │                                                                  │   │
│  │  ┌────────────────────────────────────────────────────────┐     │   │
│  │  │            Network Health Integration                   │     │   │
│  │  │  ┌─────────────────┐  ┌─────────────────┐              │     │   │
│  │  │  │ Health Gossiper │  │ Bloom Exchanger │              │     │   │
│  │  │  │ - Diff Privacy  │  │ - Filter Sync   │              │     │   │
│  │  │  │ - Aggregation   │  │ - Coordination  │              │     │   │
│  │  │  └─────────────────┘  └─────────────────┘              │     │   │
│  │  │                                                         │     │   │
│  │  │  ┌─────────────────┐  ┌─────────────────┐              │     │   │
│  │  │  │ Health Tracker  │  │   Opportunistic │              │     │   │
│  │  │  │ - Block Value   │  │   Fetcher       │              │     │   │
│  │  │  │ - Replication   │  │ - Valuable Blocks│              │     │   │
│  │  │  └─────────────────┘  └─────────────────┘              │     │   │
│  │  └────────────────────────────────────────────────────────┘     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                   │                                     │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    Base Storage Layer                            │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐     │   │
│  │  │ Memory Cache │  │Adaptive Cache│  │   IPFS Client    │     │   │
│  │  └──────────────┘  └──────────────┘  └───────────────────┘     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ P2P Network
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         NoiseFS Network                                  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐       │
│  │   Node A   │  │   Node B   │  │   Node C   │  │   Node D   │       │
│  │            │◄─┤ Gossip &   ├─►│            │◄─┤            │       │
│  │ Altruistic │  │ Bloom      │  │ Altruistic │  │ Altruistic │       │
│  │  Enabled   │  │ Exchange   │  │  Enabled   │  │  Disabled  │       │
│  └────────────┘  └────────────┘  └────────────┘  └────────────┘       │
└─────────────────────────────────────────────────────────────────────────┘
```

## Component Details

### 1. Space Management

```
┌─────────────────────────────────────────────┐
│            Space Allocation                  │
│                                             │
│  Total Capacity                             │
│  ┌─────────────────────────────────────┐   │
│  │  MinPersonal  │    Flex Pool        │   │
│  │  (Guaranteed) │  (Dynamic)          │   │
│  └─────────────────────────────────────┘   │
│                                             │
│  Allocation Rules:                          │
│  1. Personal blocks always fit in total     │
│  2. MinPersonal always available            │
│  3. Flex pool shared by both types          │
│  4. Altruistic evicted for personal        │
└─────────────────────────────────────────────┘
```

### 2. Block Lifecycle

```
                Store Request
                     │
                     ▼
            ┌─────────────────┐
            │ Categorization  │
            │  - Personal?    │
            │  - Altruistic?  │
            └─────────────────┘
                     │
                     ▼
            ┌─────────────────┐
            │ Space Check     │
            │  - Available?   │
            │  - Need evict?  │
            └─────────────────┘
                     │
           ┌─────────┴─────────┐
           │                   │
           ▼                   ▼
    ┌─────────────┐    ┌─────────────┐
    │Store Direct │    │   Eviction   │
    │             │    │  - Strategy  │
    │             │    │  - Anti-thrash│
    └─────────────┘    └─────────────┘
                               │
                               ▼
                        ┌─────────────┐
                        │    Store    │
                        └─────────────┘
```

### 3. Network Health Protocol

```
┌─────────────────────────────────────────────────────┐
│                Node A                                │
│  ┌─────────────────────────────────────────────┐    │
│  │         Local Health Tracking                │    │
│  │  - Block replication levels                 │    │
│  │  - Access patterns (anonymized)             │    │
│  │  - Geographic distribution                  │    │
│  └─────────────────────────────────────────────┘    │
│                      │                               │
│                      ▼                               │
│  ┌─────────────────────────────────────────────┐    │
│  │      Gossip Message Creation                │    │
│  │  - Aggregate statistics                     │    │
│  │  - Add differential privacy noise           │    │
│  │  - Create Bloom filters                     │    │
│  └─────────────────────────────────────────────┘    │
│                      │                               │
└──────────────────────┼───────────────────────────────┘
                       │
                       ▼ PubSub Broadcast
        ┌──────────────┴──────────────┐
        │                             │
        ▼                             ▼
┌───────────────┐            ┌───────────────┐
│    Node B     │            │    Node C     │
│               │            │               │
│ Process Gossip│            │ Process Gossip│
│ Update Health │            │ Update Health │
└───────────────┘            └───────────────┘
```

### 4. Eviction Decision Tree

```
                Need Space
                     │
                     ▼
            ┌─────────────────┐
            │ Check Origin    │
            └─────────────────┘
                     │
         ┌───────────┴───────────┐
         │                       │
         ▼                       ▼
    Personal Block          Altruistic Block
         │                       │
         ▼                       ▼
  ┌──────────────┐        ┌──────────────┐
  │Check Min     │        │ Select       │
  │Personal      │        │ Strategy     │
  └──────────────┘        └──────────────┘
         │                       │
         ▼                       ▼
  ┌──────────────┐        ┌──────────────┐
  │Evict         │        │   LRU        │
  │Altruistic    │        ├──────────────┤
  │Blocks        │        │   LFU        │
  └──────────────┘        ├──────────────┤
                          │ ValueBased   │
                          ├──────────────┤
                          │ Adaptive     │
                          ├──────────────┤
                          │ Predictive   │
                          └──────────────┘
```

### 5. Privacy-Preserving Features

```
┌─────────────────────────────────────────────────────┐
│              Privacy Mechanisms                      │
│                                                     │
│  1. Differential Privacy for Statistics             │
│     ┌─────────────────────────────────┐            │
│     │ True Count: 42                  │            │
│     │ + Laplace Noise (ε=1.0)         │            │
│     │ = Reported Count: 44            │            │
│     └─────────────────────────────────┘            │
│                                                     │
│  2. Bloom Filters for Set Membership                │
│     ┌─────────────────────────────────┐            │
│     │ Blocks: [A, B, C]               │            │
│     │ → Bloom Filter (no reversal)    │            │
│     │ False positive rate: 1%         │            │
│     └─────────────────────────────────┘            │
│                                                     │
│  3. Temporal Quantization                           │
│     ┌─────────────────────────────────┐            │
│     │ Access: 14:32:17                │            │
│     │ → Rounded to: 14:00:00          │            │
│     └─────────────────────────────────┘            │
│                                                     │
│  4. Anonymized Peer IDs                             │
│     ┌─────────────────────────────────┐            │
│     │ Real: peer-12345-xyz            │            │
│     │ → Anonymous: peer-a8f2c9        │            │
│     └─────────────────────────────────┘            │
└─────────────────────────────────────────────────────┘
```

## Data Flow

### Upload Flow
1. User uploads file
2. File split into blocks
3. Blocks marked as PersonalBlock
4. Stored with MinPersonal guarantee
5. Metrics updated

### Altruistic Caching Flow
1. Network health monitor identifies valuable blocks
2. Opportunistic fetcher queues blocks
3. Space availability checked
4. Blocks fetched and stored as AltruisticBlock
5. Health metrics updated

### Eviction Flow
1. Personal block needs space
2. Eviction strategy selected
3. Altruistic blocks scored
4. Lowest value blocks evicted
5. Anti-thrashing cooldown applied

### Network Coordination Flow
1. Local health calculated
2. Gossip message created with privacy
3. Broadcast via PubSub
4. Peers process and update estimates
5. Coordination hints generated
6. Caching decisions adjusted

## Configuration Points

### User-Controlled
- `min_personal_cache_mb`: Guaranteed personal space
- `enable_altruistic`: Master on/off switch
- `altruistic_bandwidth_mb`: Bandwidth limit

### Advanced
- `eviction_strategy`: Algorithm selection
- `eviction_cooldown`: Anti-thrashing delay
- `enable_predictive`: Predictive eviction
- `pre_evict_threshold`: When to pre-evict

### Network Health
- `gossip_interval`: How often to gossip
- `bloom_filter_size`: Filter accuracy
- `privacy_epsilon`: Privacy level
- `coordination_threshold`: Coordination sensitivity