# Altruistic Caching Quick Start Guide

This guide will help you get started with NoiseFS's altruistic caching in 5 minutes.

## What is Altruistic Caching?

Altruistic caching allows your NoiseFS node to automatically contribute spare storage to improve network health while guaranteeing you always have the storage you need.

**Benefits:**
- 🌐 Improves network performance for everyone
- 🛡️ Increases data resilience through distributed caching  
- ⚡ Speeds up popular content access
- 🔒 Maintains complete privacy
- 🤖 Fully automatic - no management needed

## Quick Setup

### 1. Enable with Default Settings

NoiseFS has altruistic caching **enabled by default** with sensible settings:
- 50% of your cache is guaranteed for personal use
- 50% flexibly helps the network when you don't need it

No configuration needed - it just works!

### 2. Check Your Contribution

See how much you're helping the network:
```bash
noisefs -stats
```

Look for the "Altruistic Cache" section to see your contribution.

### 3. Customize Your Contribution (Optional)

#### Set Your Guaranteed Personal Space
```bash
# Reserve 100GB for personal files
noisefs -min-personal-cache 102400
```

#### Temporarily Disable
```bash
# Upload without altruistic caching
noisefs -disable-altruistic -upload myfile.dat
```

## Common Scenarios

### Home User (1TB disk)
```json
{
  "cache": {
    "memory_limit_mb": 1024000,      // 1TB total
    "min_personal_cache_mb": 204800   // 200GB personal
  }
}
```
Result: 200GB always available for you, up to 800GB helps network

### Power User (10TB disk)
```json
{
  "cache": {
    "memory_limit_mb": 10240000,     // 10TB total
    "min_personal_cache_mb": 2048000  // 2TB personal
  }
}
```
Result: 2TB guaranteed personal, up to 8TB network contribution

### Minimal Contribution
```json
{
  "cache": {
    "memory_limit_mb": 512000,       // 500GB total
    "min_personal_cache_mb": 450000   // 450GB personal
  }
}
```
Result: 450GB personal, only 50GB for network

## Visual Feedback

The `-stats` command shows a visual representation:
```
Cache Utilization:
Total: [████████████████████████▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒░░░░░░░░░░] 48.5%
       █ Personal (24.2%)  ▒ Altruistic (24.3%)  ░ Free (51.5%)

Flex Pool: [▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░░░░░░░░░░░░░░░░░] 42.1%
                    ↑ Min Personal (20.0%)
```

## Understanding the Display

- **Personal Blocks** (█): Your files
- **Altruistic Blocks** (▒): Network contribution  
- **Free Space** (░): Available capacity
- **Flex Pool** (▓): How much of the flexible space is used
- **Min Personal** (↑): Your guaranteed minimum

## FAQ

**Q: Will this slow down my file access?**
A: No. Personal blocks are prioritized and never evicted for network blocks.

**Q: Can I see what files are being cached?**
A: No. For privacy, the system only knows about blocks, not files.

**Q: What happens if I need more space?**
A: Network blocks are automatically evicted to make room for your files.

**Q: Is my contribution anonymous?**
A: Yes. There's no way to link cached blocks to users or files.

**Q: Can I limit bandwidth usage?**
A: Yes, use `-altruistic-bandwidth 50` to limit to 50 MB/s.

## Next Steps

- Read the [full documentation](altruistic-caching.md) for advanced features
- Monitor your contribution with `noisefs -stats`
- Join the community to see network-wide impact

Thank you for contributing to a more resilient and performant network!