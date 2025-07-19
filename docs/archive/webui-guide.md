# NoiseFS Web UI Guide

## Overview

NoiseFS includes a web-based user interface for managing files through your browser. The web UI provides a graphical alternative to the command-line interface with support for drag-and-drop uploads, file browsing, and system monitoring.

## Starting the Web UI

### Basic Usage

```bash
# Start web UI with defaults (https://localhost:8080)
noisefs-webui

# Start on custom port
noisefs-webui --port 9000

# Start with HTTP (not recommended)
noisefs-webui --no-tls
```

### Configuration

Enable web UI in configuration:

```json
{
  "webui": {
    "enabled": true,
    "address": "localhost:8080",
    "tls": {
      "enabled": true,
      "cert_file": "",
      "key_file": ""
    }
  }
}
```

Or use environment variables:
```bash
export NOISEFS_WEBUI_ENABLED=true
export NOISEFS_WEBUI_ADDRESS="0.0.0.0:8080"
```

## Features

### File Management

- **Upload Files**: Drag and drop or click to browse
- **Download Files**: Click to download any file
- **Delete Files**: Remove files with confirmation
- **File Preview**: View images and text files
- **Batch Operations**: Select multiple files for bulk actions

### File Browser

The main interface shows:
- File list with names, sizes, and upload dates
- Search and filter options
- Sort by name, size, or date
- Directory navigation

### System Status

The dashboard displays:
- IPFS connection status
- Cache statistics and hit rate
- Active operations
- Storage usage
- Network peers

## Security

### TLS/HTTPS

The web UI uses HTTPS by default with self-signed certificates:

```bash
# Generate custom certificates
noisefs-webui --generate-cert

# Use existing certificates
noisefs-webui --cert server.crt --key server.key
```

### Authentication

Currently, the web UI relies on network-level security. Only bind to localhost unless you implement additional authentication:

```bash
# Safe: localhost only (default)
noisefs-webui --address localhost:8080

# Unsafe without authentication
noisefs-webui --address 0.0.0.0:8080  # Accessible from network
```

### Access Control

For production use:
1. Use a reverse proxy (nginx, Apache) with authentication
2. Implement firewall rules
3. Use VPN for remote access
4. Never expose directly to the internet

## API Endpoints

The web UI exposes REST API endpoints:

### File Operations

```bash
# Upload file
curl -X POST https://localhost:8080/api/upload \
  -F "file=@document.pdf" \
  -H "Accept: application/json"

# List files
curl https://localhost:8080/api/files

# Download file
curl https://localhost:8080/api/download/document.pdf -o downloaded.pdf

# Delete file
curl -X DELETE https://localhost:8080/api/files/document.pdf
```

### System Information

```bash
# Get status
curl https://localhost:8080/api/status

# Get cache stats
curl https://localhost:8080/api/cache/stats

# Get IPFS info
curl https://localhost:8080/api/ipfs/status
```

## Advanced Usage

### Custom Themes

Place custom CSS in `~/.noisefs/webui/custom.css`:

```css
/* Example: Dark theme override */
body {
  background-color: #1a1a1a;
  color: #ffffff;
}
```

### Reverse Proxy Setup

Example nginx configuration:

```nginx
server {
  listen 443 ssl;
  server_name noisefs.example.com;
  
  ssl_certificate /path/to/cert.pem;
  ssl_certificate_key /path/to/key.pem;
  
  # Basic authentication
  auth_basic "NoiseFS Web UI";
  auth_basic_user_file /etc/nginx/.htpasswd;
  
  location / {
    proxy_pass https://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
  }
}
```

### Docker Deployment

```dockerfile
FROM theentropycollective/noisefs

# Expose web UI port
EXPOSE 8080

# Start with web UI enabled
CMD ["noisefs", "webui", "--address", "0.0.0.0:8080"]
```

## Troubleshooting

### Certificate Errors

**Problem**: Browser shows certificate warning

**Solution**: 
1. Accept the self-signed certificate
2. Or generate a proper certificate:
   ```bash
   noisefs-webui --generate-cert --hostname noisefs.local
   ```

### Connection Refused

**Problem**: Cannot connect to web UI

**Solutions**:
1. Check if web UI is running: `ps aux | grep "noisefs-webui"`
2. Check port availability: `lsof -i :8080`
3. Check firewall rules
4. Try different port: `noisefs-webui --port 9090`

### Upload Failures

**Problem**: Large file uploads fail

**Solutions**:
1. Increase upload timeout in nginx (if using reverse proxy)
2. Check available disk space
3. Monitor browser console for errors
4. Use CLI for very large files

## Performance Tips

1. **Use modern browsers** - Chrome, Firefox, Safari latest versions
2. **Limit concurrent uploads** - Too many parallel uploads can overwhelm IPFS
3. **Enable compression** - Use gzip in reverse proxy
4. **Monitor resources** - Check CPU/memory usage during heavy operations

## Limitations

- No built-in authentication (use reverse proxy)
- File size limited by browser capabilities
- No folder upload support (use CLI for directories)
- Real-time updates require page refresh

## Future Enhancements

Planned features for the web UI:
- User authentication system
- Real-time file sync
- Folder upload support
- Advanced search capabilities
- Mobile-responsive design
- File sharing links
- Activity logs

## See Also

- [Configuration Reference](configuration.md#web-ui-configuration) - Web UI configuration
- [CLI Usage Guide](cli-usage.md) - Command-line alternative
- [Troubleshooting Guide](troubleshooting.md#web-ui-issues) - Common issues