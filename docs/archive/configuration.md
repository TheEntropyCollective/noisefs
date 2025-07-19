# NoiseFS Configuration Reference

## Overview

NoiseFS uses a JSON configuration file to control its behavior. The default location is `~/.noisefs/config.json`, but you can specify a custom location with the `--config` flag.

## Configuration Structure

NoiseFS configuration is organized into logical sections, each controlling a specific aspect of the system. Here's what each section does:

- **ipfs**: Connection settings for the IPFS storage backend
- **cache**: Local block caching behavior and limits
- **storage**: Backend selection and distribution strategies
- **privacy**: Anonymity features and privacy levels
- **blocks**: Block processing parameters
- **fuse**: Filesystem mounting options
- **logging**: Log output configuration
- **security**: Security features and protections
- **performance**: Concurrency and optimization settings
- **webui**: Web interface configuration

## Configuration Sections

### IPFS Configuration (`ipfs`)

Controls how NoiseFS connects to IPFS:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `api_endpoint` | string | `"http://localhost:5001"` | IPFS API endpoint URL |
| `timeout` | int | `300` | Request timeout in seconds |
| `max_connections` | int | `100` | Maximum concurrent connections |

**Common Configurations:**
- Local IPFS: `"http://localhost:5001"`
- Remote IPFS: `"http://192.168.1.100:5001"`
- Docker IPFS: `"http://ipfs:5001"`

### Cache Configuration (`cache`)

Controls local block caching for performance:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable/disable caching |
| `max_size` | int | `1000` | Maximum number of cached blocks |
| `memory_limit` | int | `268435456` | Maximum memory usage in bytes (256MB) |
| `eviction_policy` | string | `"lru"` | Eviction policy: "lru", "lfu", "fifo" |
| `ttl` | int | `3600` | Time-to-live in seconds |

**Memory Limit Examples:**
- 256MB: `268435456`
- 512MB: `536870912`
- 1GB: `1073741824`
- 2GB: `2147483648`

### Storage Configuration (`storage`)

Controls storage backend behavior and distribution:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_backend` | string | `"ipfs"` | Primary storage backend |
| `distribution.strategy` | string | `"single"` | Distribution strategy |
| `distribution.replication.min_replicas` | int | `1` | Minimum replicas |
| `distribution.replication.max_replicas` | int | `3` | Maximum replicas |

**Distribution Strategies:**
- `"single"`: Use one backend (simplest)
- `"replicate"`: Store copies on multiple backends
- `"smart"`: Automatically choose based on availability

### Privacy Configuration (`privacy`)

Controls privacy and anonymity features:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_level` | string | `"medium"` | Default privacy level |
| `relay_pool.enabled` | bool | `false` | Enable relay routing |
| `relay_pool.max_relays` | int | `10` | Maximum relay nodes |
| `relay_pool.min_relays` | int | `3` | Minimum relay nodes |
| `cover_traffic.enabled` | bool | `false` | Enable cover traffic |
| `cover_traffic.ratio` | float | `0.5` | Cover to real traffic ratio |

**Privacy Levels:**
- `"low"`: Basic anonymization, best performance
- `"medium"`: Balanced privacy and performance (default)
- `"high"`: Maximum privacy with relay routing and cover traffic

### Block Configuration (`blocks`)

Controls block processing parameters:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `size` | int | `131072` | Block size in bytes (128KB) |
| `compression` | bool | `false` | Enable block compression |
| `encryption` | bool | `false` | Additional encryption layer |

**Block Size Options:**
- 64KB: `65536` (smaller files)
- 128KB: `131072` (default, recommended)
- 256KB: `262144` (larger files)
- 1MB: `1048576` (very large files)

### FUSE Configuration (`fuse`)

Controls filesystem mounting behavior:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `allow_other` | bool | `false` | Allow other users to access |
| `default_permissions` | bool | `true` | Enable permission checking |
| `max_read` | int | `131072` | Maximum read size |
| `debug` | bool | `false` | Enable FUSE debug output |

### Logging Configuration (`logging`)

Controls log output and verbosity:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `"info"` | Log level |
| `file` | string | `""` | Log file path (empty = stdout) |
| `format` | string | `"json"` | Output format |
| `max_size` | int | `10` | Max log file size in MB |
| `max_backups` | int | `3` | Number of old logs to keep |

**Log Levels:**
- `"debug"`: Very detailed output
- `"info"`: Normal operation logs
- `"warn"`: Warnings only
- `"error"`: Errors only

### Security Configuration (`security`)

Controls security features:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `secure_delete` | bool | `true` | Overwrite data on deletion |
| `memory_lock` | bool | `false` | Lock sensitive data in memory |
| `audit_log` | bool | `false` | Enable audit logging |

### Performance Configuration (`performance`)

Controls concurrency and optimization:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `parallel_uploads` | int | `3` | Concurrent upload operations |
| `parallel_downloads` | int | `5` | Concurrent download operations |
| `prefetch` | bool | `true` | Enable predictive prefetching |
| `write_buffer_size` | int | `4194304` | Write buffer size (4MB) |

### Web UI Configuration (`webui`)

Controls the web interface:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable web interface |
| `address` | string | `"localhost:8080"` | Listen address |
| `tls.enabled` | bool | `true` | Use HTTPS |
| `tls.cert_file` | string | `""` | TLS certificate path |
| `tls.key_file` | string | `""` | TLS key path |

## Environment Variables

All configuration options can be overridden using environment variables. The format is:

```
NOISEFS_<SECTION>_<FIELD>
```

Examples:
```bash
# Override IPFS endpoint
export NOISEFS_IPFS_API_ENDPOINT="http://remote-ipfs:5001"

# Set cache size
export NOISEFS_CACHE_MAX_SIZE="5000"

# Enable debug logging
export NOISEFS_LOGGING_LEVEL="debug"

# Set privacy level
export NOISEFS_PRIVACY_DEFAULT_LEVEL="high"
```

For nested fields, use underscores:
```bash
# Enable relay pool
export NOISEFS_PRIVACY_RELAY_POOL_ENABLED="true"

# Set replication strategy
export NOISEFS_STORAGE_DISTRIBUTION_STRATEGY="replicate"
```

## Configuration Profiles

### Minimal Configuration

For basic usage with defaults:
```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001"
  }
}
```

### Development Configuration

Optimized for testing and development:
```json
{
  "cache": {
    "max_size": 100
  },
  "logging": {
    "level": "debug",
    "format": "text"
  },
  "privacy": {
    "default_level": "low"
  }
}
```

### Production Configuration

For production deployments:
```json
{
  "cache": {
    "max_size": 10000,
    "memory_limit": 2147483648
  },
  "logging": {
    "level": "warn",
    "file": "/var/log/noisefs/noisefs.log"
  },
  "security": {
    "memory_lock": true,
    "audit_log": true
  },
  "performance": {
    "parallel_uploads": 10,
    "parallel_downloads": 20
  }
}
```

### High Security Configuration

Maximum privacy and security:
```json
{
  "privacy": {
    "default_level": "high",
    "relay_pool": {
      "enabled": true,
      "min_relays": 5
    },
    "cover_traffic": {
      "enabled": true,
      "ratio": 1.0
    }
  },
  "blocks": {
    "encryption": true
  },
  "security": {
    "secure_delete": true,
    "memory_lock": true,
    "audit_log": true
  }
}
```

## Managing Configuration

### Using noisefs-config Tool

```bash
# Initialize default configuration
noisefs-config init

# Show current configuration
noisefs-config show

# Set a configuration value
noisefs-config set cache.max_size 5000

# Get a configuration value
noisefs-config get privacy.default_level

# Validate configuration
noisefs-config validate

# Reset to defaults
noisefs-config reset
```

### Configuration Precedence

Configuration values are applied in this order (later overrides earlier):
1. Built-in defaults
2. Configuration file
3. Environment variables
4. Command-line flags

## Best Practices

1. **Start Simple**: Use minimal configuration and add options as needed
2. **Monitor Performance**: Adjust cache and parallelism based on usage
3. **Security vs Performance**: Higher privacy levels impact performance
4. **Resource Limits**: Set appropriate limits for your hardware
5. **Regular Backups**: Keep backups of your configuration file

## Troubleshooting Configuration

### Common Issues

**Configuration Not Loading**
- Check file permissions: `ls -la ~/.noisefs/config.json`
- Validate JSON syntax: `noisefs-config validate`
- Check for typos in field names

**Environment Variables Not Working**
- Ensure variables are exported: `export NOISEFS_...`
- Check capitalization (must be uppercase)
- Verify underscore placement for nested fields

**Performance Issues**
- Increase cache size if you have memory
- Adjust parallel operations based on network
- Consider lowering privacy level

## See Also

- [CLI Usage Guide](cli-usage.md) - Command-line options
- [Installation Guide](installation.md) - Initial setup
- [Privacy Infrastructure](privacy-infrastructure.md) - Privacy level details
- [Troubleshooting Guide](troubleshooting.md) - Common problems