# NoiseFS Tor Integration Guide

## Overview

NoiseFS now includes optional Tor support for enhanced privacy during file uploads, downloads, and announcement publishing. This integration provides network-layer anonymity to complement NoiseFS's content-layer privacy.

## Configuration

### Basic Setup

Tor integration is configured in your NoiseFS config file (`~/.noisefs/config.json`):

```json
{
  "tor": {
    "enabled": true,
    "socks_proxy": "127.0.0.1:9050",
    "control_port": "127.0.0.1:9051",
    "upload_enabled": true,
    "upload_jitter_min_seconds": 1,
    "upload_jitter_max_seconds": 5,
    "download_enabled": false,
    "announce_enabled": true
  }
}
```

### Configuration Options

- **enabled**: Master switch for all Tor functionality
- **socks_proxy**: Tor SOCKS5 proxy address (default: 127.0.0.1:9050)
- **control_port**: Tor control port for circuit management (default: 127.0.0.1:9051)
- **upload_enabled**: Use Tor for uploads (default: true for privacy)
- **upload_jitter_min/max**: Random delay range for timing anonymity (seconds)
- **download_enabled**: Use Tor for downloads (default: false for performance)
- **announce_enabled**: Use Tor for announcement publishing (default: true)

### Environment Variables

Override configuration via environment:

```bash
export NOISEFS_TOR_ENABLED=true
export NOISEFS_TOR_UPLOAD_ENABLED=true
export NOISEFS_TOR_DOWNLOAD_ENABLED=false
```

## Performance Impact

### Upload Performance

**Without Tor:**
- Direct IPFS upload: ~1-10 MB/s
- Latency: 50-200ms per block
- Concurrency: 10 parallel uploads

**With Tor:**
- Tor-routed upload: ~50-200 KB/s (3-20x slower)
- Latency: 2-5s per block
- Concurrency: 3 parallel circuits (configurable)
- Additional jitter: 1-5s per upload

### Download Performance

**Without Tor:**
- Direct IPFS download: ~1-10 MB/s
- Latency: 50-200ms per block

**With Tor:**
- Tor-routed download: ~100-400 KB/s (2.5-10x slower)
- Latency: 1-3s per block
- Circuit reuse helps performance

### Real-World Examples

```
1 MB file upload:
- Direct: ~1 second
- Via Tor: ~10-20 seconds (with jitter)

100 MB file upload:
- Direct: ~10-20 seconds  
- Via Tor: ~8-16 minutes

1 GB file upload:
- Direct: ~2-3 minutes
- Via Tor: ~1.5-3 hours
```

## Usage

### Command Line

NoiseFS CLI automatically uses Tor based on configuration:

```bash
# Upload with Tor (if enabled)
noisefs upload myfile.pdf

# Force direct upload (bypass Tor)
noisefs upload --no-tor myfile.pdf

# Force Tor for download (override config)
noisefs download --use-tor QmXyz...
```

### Programmatic Usage

```go
import (
    "github.com/TheEntropyCollective/noisefs/pkg/network/tor"
    "github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
)

// Create Tor-enabled client
cfg := config.DefaultConfig()
cfg.Tor.Enabled = true
cfg.Tor.UploadEnabled = true

torClient, err := tor.NewTorEnabledClient(ipfsClient, cfg)
if err != nil {
    log.Fatal(err)
}
defer torClient.Close()

// Upload automatically uses Tor
cid, err := torClient.StoreBlock(block)

// Check performance metrics
fmt.Println(torClient.GetPerformanceReport())
```

## Privacy Benefits

### What Tor Provides

1. **IP Address Protection**: Your IP is hidden from IPFS nodes
2. **Traffic Analysis Resistance**: Harder to correlate uploads/downloads
3. **Geographic Anonymity**: Appears to come from Tor exit nodes
4. **Timing Obfuscation**: Jitter prevents temporal correlation

### What Tor Doesn't Provide

1. **Content Privacy**: Already handled by NoiseFS XOR encryption
2. **Perfect Anonymity**: Advanced adversaries may still correlate
3. **Speed**: Significant performance penalty

## Architecture

### Circuit Pool

NoiseFS pre-establishes Tor circuits for better performance:

```
Circuit Pool:
├── Upload Circuits (3-10)
│   ├── Circuit rotation every 10 minutes
│   └── Different circuit per block for anonymity
├── Download Circuits (1-3)
│   └── Reused for performance
└── Announcement Circuit (1)
    └── Dedicated for DHT/PubSub
```

### Integration Points

1. **Block Storage**: `pkg/network/tor/noisefs_integration.go`
   - Wraps IPFS operations with Tor transport
   - Intelligent routing decisions

2. **Announcements**: `pkg/network/tor/announce_integration.go`
   - DHT publishing through Tor
   - PubSub with timing jitter

3. **Configuration**: `pkg/infrastructure/config/config.go`
   - Unified configuration system
   - Environment variable overrides

## Best Practices

### For Maximum Privacy

```json
{
  "tor": {
    "enabled": true,
    "upload_enabled": true,
    "upload_jitter_min_seconds": 5,
    "upload_jitter_max_seconds": 30,
    "download_enabled": true,
    "announce_enabled": true
  }
}
```

### For Balanced Usage

```json
{
  "tor": {
    "enabled": true,
    "upload_enabled": true,
    "upload_jitter_min_seconds": 1,
    "upload_jitter_max_seconds": 5,
    "download_enabled": false,  // Direct for performance
    "announce_enabled": true
  }
}
```

### For Performance Testing

```json
{
  "tor": {
    "enabled": true,
    "upload_enabled": false,  // Compare with/without
    "download_enabled": false,
    "announce_enabled": false
  }
}
```

## Troubleshooting

### Tor Not Connected

```
Error: Tor not accessible: connection test failed
```

**Solution:**
1. Ensure Tor is running: `tor` or `brew services start tor`
2. Check SOCKS proxy port: `netstat -an | grep 9050`
3. Verify no firewall blocking

### Slow Performance

```
Warning: High Tor latency detected: 8.5s
```

**Solutions:**
1. Check Tor network status: https://status.torproject.org/
2. Reduce concurrent operations
3. Increase timeouts in config
4. Consider selective Tor usage (uploads only)

### Circuit Failures

```
Warning: Circuit pool initialization took 45s
```

**Solutions:**
1. Reduce minimum circuits
2. Check Tor logs: `journalctl -u tor`
3. Try different Tor entry guards

## Security Considerations

### Operational Security

1. **Tor Traffic is Identifiable**: ISPs can see you're using Tor
   - Consider bridges if Tor usage is sensitive
   - Use VPN + Tor for additional layer

2. **Exit Node Visibility**: Exit nodes see IPFS traffic
   - Traffic is HTTPS encrypted
   - Content is already XOR encrypted

3. **Correlation Attacks**: Large files easier to correlate
   - Use random delays between blocks
   - Upload at different times

### Configuration Security

1. **Don't Share Tor Ports**: Keep SOCKS proxy local
2. **Regular Updates**: Keep Tor updated
3. **Monitor Performance**: Detect anomalies

## Performance Tuning

### Upload Optimization

```go
// Tor performance configuration
torConfig := &tor.Config{
    Performance: tor.PerformanceConfig{
        ConcurrentUploads: 5,      // More circuits
        UseCompression: true,      // Reduce bandwidth
        ParallelCircuits: 5,       // Split large uploads
        StreamBufferSize: 64*1024, // Larger buffers
    },
}
```

### Circuit Pool Tuning

```go
CircuitPool: tor.CircuitPoolConfig{
    MinCircuits: 5,                    // Pre-establish more
    MaxCircuits: 20,                   // Allow more concurrent
    CircuitLifetime: 20 * time.Minute, // Rotate less often
    BuildTimeout: 60 * time.Second,    // Allow slow builds
}
```

## Monitoring

### Performance Metrics

The Tor integration provides detailed metrics:

```go
metrics := torClient.GetMetrics()
fmt.Printf("Tor uploads: %d\n", metrics.TorUploads)
fmt.Printf("Average speed: %.1f KB/s\n", metrics.AvgTorSpeed/1024)
fmt.Printf("Circuit builds: %d\n", metrics.CircuitBuilds)
fmt.Printf("Circuit failures: %d\n", metrics.CircuitFailures)
```

### Performance Report

```
Tor Integration Performance Report:

Uploads:
  Via Tor:    145 (72.5%)
  Direct:     55 (27.5%)
  Avg Speed:  125.3 KB/s (Tor) vs ~1000 KB/s (Direct)
  
Downloads:
  Via Tor:    0 (0.0%)
  Direct:     89 (100.0%)
  
Performance Impact:
  Upload:     ~8.0x slower with Tor
  Download:   ~2.5x slower with Tor
  Privacy:    Significantly improved with Tor
```

## Future Enhancements

1. **Hidden Service Support**: Host NoiseFS nodes as .onion services
2. **Bridge Integration**: Support for obfuscated Tor bridges
3. **Onion Routing over IPFS**: Native implementation planned
4. **Performance Optimizations**: 
   - Persistent circuit caching
   - Predictive circuit building
   - Compression improvements

## Conclusion

Tor integration provides strong network-layer anonymity for NoiseFS operations, complementing the existing content-layer privacy. While performance impacts are significant (3-10x slower), the privacy benefits are substantial for users requiring maximum anonymity. The default configuration balances privacy and usability by enabling Tor for uploads and announcements while maintaining direct connections for downloads.