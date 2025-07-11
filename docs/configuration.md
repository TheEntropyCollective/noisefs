# NoiseFS Configuration Reference

## Overview

NoiseFS uses a JSON configuration file to control its behavior. The default location is `~/.noisefs/config.json`, but you can specify a custom location with the `--config` flag.

## Configuration File Structure

```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001",
    "timeout": 300,
    "max_connections": 100
  },
  "cache": {
    "enabled": true,
    "max_size": 1000,
    "memory_limit": 268435456,
    "eviction_policy": "lru",
    "ttl": 3600
  },
  "storage": {
    "default_backend": "ipfs",
    "distribution": {
      "strategy": "single",
      "replication": {
        "min_replicas": 1,
        "max_replicas": 3
      }
    }
  },
  "privacy": {
    "default_level": "medium",
    "relay_pool": {
      "enabled": false,
      "max_relays": 10,
      "min_relays": 3
    },
    "cover_traffic": {
      "enabled": false,
      "ratio": 0.5
    }
  },
  "blocks": {
    "size": 131072,
    "compression": false,
    "encryption": false
  },
  "fuse": {
    "allow_other": false,
    "default_permissions": true,
    "max_read": 131072,
    "debug": false
  },
  "logging": {
    "level": "info",
    "file": "",
    "format": "json",
    "max_size": 10,
    "max_backups": 3
  },
  "security": {
    "secure_delete": true,
    "memory_lock": false,
    "audit_log": false
  },
  "performance": {
    "parallel_uploads": 3,
    "parallel_downloads": 5,
    "prefetch": true,
    "write_buffer_size": 4194304
  },
  "webui": {
    "enabled": false,
    "address": "localhost:8080",
    "tls": {
      "enabled": true,
      "cert_file": "",
      "key_file": ""
    }
  }
}
```

## Configuration Sections

### IPFS Configuration (`ipfs`)

Controls IPFS backend settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `api_endpoint` | string | `"http://localhost:5001"` | IPFS API endpoint URL |
| `timeout` | int | `300` | Request timeout in seconds |
| `max_connections` | int | `100` | Maximum concurrent connections |

Example:
```json
"ipfs": {
  "api_endpoint": "http://192.168.1.100:5001",
  "timeout": 600,
  "max_connections": 200
}
```

### Cache Configuration (`cache`)

Controls local block caching behavior.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable/disable caching |
| `max_size` | int | `1000` | Maximum number of cached blocks |
| `memory_limit` | int | `268435456` | Maximum memory usage in bytes (256MB) |
| `eviction_policy` | string | `"lru"` | Eviction policy: "lru", "lfu", "fifo" |
| `ttl` | int | `3600` | Time-to-live in seconds |

Example:
```json
"cache": {
  "enabled": true,
  "max_size": 5000,
  "memory_limit": 1073741824,  // 1GB
  "eviction_policy": "lru",
  "ttl": 7200
}
```

### Storage Configuration (`storage`)

Controls storage backend behavior.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_backend` | string | `"ipfs"` | Default storage backend |
| `distribution.strategy` | string | `"single"` | Distribution strategy: "single", "replicate", "smart" |
| `distribution.replication.min_replicas` | int | `1` | Minimum number of replicas |
| `distribution.replication.max_replicas` | int | `3` | Maximum number of replicas |

### Privacy Configuration (`privacy`)

Controls privacy features.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_level` | string | `"medium"` | Default privacy level: "low", "medium", "high" |
| `relay_pool.enabled` | bool | `false` | Enable relay pool for anonymous routing |
| `relay_pool.max_relays` | int | `10` | Maximum number of relays in pool |
| `relay_pool.min_relays` | int | `3` | Minimum number of relays |
| `cover_traffic.enabled` | bool | `false` | Enable cover traffic generation |
| `cover_traffic.ratio` | float | `0.5` | Ratio of cover to real traffic |

Privacy levels:
- **low**: Basic anonymization, best performance
- **medium**: Balanced privacy and performance
- **high**: Maximum privacy with relay routing and cover traffic

### Block Configuration (`blocks`)

Controls block splitting and processing.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `size` | int | `131072` | Block size in bytes (128KB) |
| `compression` | bool | `false` | Enable block compression |
| `encryption` | bool | `false` | Enable additional encryption layer |

### FUSE Configuration (`fuse`)

Controls FUSE filesystem mounting.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `allow_other` | bool | `false` | Allow other users to access mount |
| `default_permissions` | bool | `true` | Enable permission checking |
| `max_read` | int | `131072` | Maximum read size in bytes |
| `debug` | bool | `false` | Enable FUSE debug output |

### Logging Configuration (`logging`)

Controls logging behavior.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `"info"` | Log level: "debug", "info", "warn", "error" |
| `file` | string | `""` | Log file path (empty = stdout) |
| `format` | string | `"json"` | Log format: "json", "text" |
| `max_size` | int | `10` | Maximum log file size in MB |
| `max_backups` | int | `3` | Number of backup files to keep |

### Security Configuration (`security`)

Controls security features.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `secure_delete` | bool | `true` | Overwrite data on deletion |
| `memory_lock` | bool | `false` | Lock sensitive data in memory |
| `audit_log` | bool | `false` | Enable audit logging |

### Performance Configuration (`performance`)

Controls performance optimizations.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `parallel_uploads` | int | `3` | Concurrent upload operations |
| `parallel_downloads` | int | `5` | Concurrent download operations |
| `prefetch` | bool | `true` | Enable predictive prefetching |
| `write_buffer_size` | int | `4194304` | Write buffer size in bytes (4MB) |

### Web UI Configuration (`webui`)

Controls web interface settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable web UI server |
| `address` | string | `"localhost:8080"` | Listen address |
| `tls.enabled` | bool | `true` | Enable HTTPS |
| `tls.cert_file` | string | `""` | TLS certificate file path |
| `tls.key_file` | string | `""` | TLS key file path |

## Environment Variables

All configuration options can be overridden using environment variables with the prefix `NOISEFS_`:

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

Environment variable format: `NOISEFS_<SECTION>_<FIELD>` (uppercase, underscores for nesting)

## Configuration Profiles

### Development Profile

Optimized for development and testing:

```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001"
  },
  "cache": {
    "enabled": true,
    "max_size": 100
  },
  "privacy": {
    "default_level": "low"
  },
  "logging": {
    "level": "debug",
    "format": "text"
  }
}
```

### Production Profile

Optimized for production use:

```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001",
    "timeout": 600,
    "max_connections": 500
  },
  "cache": {
    "enabled": true,
    "max_size": 10000,
    "memory_limit": 2147483648
  },
  "privacy": {
    "default_level": "medium",
    "relay_pool": {
      "enabled": true,
      "max_relays": 20
    }
  },
  "logging": {
    "level": "warn",
    "file": "/var/log/noisefs/noisefs.log",
    "format": "json"
  },
  "security": {
    "secure_delete": true,
    "memory_lock": true,
    "audit_log": true
  }
}
```

### High Security Profile

Maximum security and privacy:

```json
{
  "privacy": {
    "default_level": "high",
    "relay_pool": {
      "enabled": true,
      "min_relays": 5,
      "max_relays": 20
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

### Manual Editing

You can edit the configuration file directly:

```bash
# Edit with your preferred editor
vim ~/.noisefs/config.json

# Validate after editing
noisefs-config validate
```

## Best Practices

1. **Start with defaults** - The default configuration works well for most users
2. **Adjust cache size** based on available memory
3. **Use appropriate privacy levels** for your threat model
4. **Enable logging** in production for troubleshooting
5. **Regular backups** of your configuration file
6. **Test changes** in development before production

## Troubleshooting

### Configuration Not Loading

```bash
# Check configuration location
noisefs config show --config-path

# Validate configuration syntax
noisefs-config validate

# Check permissions
ls -la ~/.noisefs/config.json
```

### Environment Variables Not Working

```bash
# Check variable is exported
echo $NOISEFS_CACHE_MAX_SIZE

# Run with debug to see configuration sources
noisefs --debug status
```

## See Also

- [CLI Usage Guide](cli-usage.md) - Using the command-line interface
- [Installation Guide](installation.md) - Installation and setup
- [Privacy Infrastructure](privacy-infrastructure.md) - Privacy configuration details