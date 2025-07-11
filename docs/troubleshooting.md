# NoiseFS Troubleshooting Guide

This guide helps resolve common issues with NoiseFS. For additional help, use `noisefs --debug` to enable verbose logging.

## Common Issues

### IPFS Connection Errors

**Problem**: `Error: cannot connect to IPFS API: connection refused`

**Solutions**:

1. **Start IPFS daemon**
   ```bash
   ipfs daemon &
   ```

2. **Check IPFS is running**
   ```bash
   ipfs id
   curl http://localhost:5001/api/v0/id
   ```

3. **Use custom endpoint**
   ```bash
   noisefs --ipfs-endpoint http://192.168.1.100:5001 upload file.txt
   ```

4. **Configure endpoint permanently**
   ```bash
   noisefs-config set ipfs.api_endpoint "http://192.168.1.100:5001"
   ```

### FUSE Mount Failures

**Problem**: `Mount failed: operation not permitted`

**Solutions**:

1. **Check FUSE is installed**
   ```bash
   # Linux
   ls /dev/fuse
   
   # macOS
   ls /Library/Filesystems/macfuse.fs
   ```

2. **Install FUSE**
   ```bash
   # Ubuntu/Debian
   sudo apt-get install fuse
   
   # macOS
   brew install --cask macfuse
   ```

3. **Add user to fuse group (Linux)**
   ```bash
   sudo usermod -a -G fuse $USER
   # Log out and back in
   ```

4. **Run with privileges**
   ```bash
   sudo noisefs-mount /mnt/noisefs
   ```

### File Upload Failures

**Problem**: `Error: failed to upload block: timeout`

**Solutions**:

1. **Increase timeout**
   ```bash
   noisefs-config set ipfs.timeout 600
   ```

2. **Check IPFS storage**
   ```bash
   ipfs repo stat
   ```

3. **Clear IPFS garbage**
   ```bash
   ipfs repo gc
   ```

4. **Reduce parallel uploads**
   ```bash
   noisefs-config set performance.parallel_uploads 1
   ```

### Cache Issues

**Problem**: `Warning: cache full, performance degraded`

**Solutions**:

1. **Increase cache size**
   ```bash
   noisefs cache resize 5000
   ```

2. **Clear cache**
   ```bash
   noisefs cache clear
   ```

3. **Check cache stats**
   ```bash
   noisefs cache stats
   ```

4. **Disable cache temporarily**
   ```bash
   noisefs --no-cache upload large-file.iso
   ```

### Index Corruption

**Problem**: `Error: failed to load index: invalid format`

**Solutions**:

1. **Backup corrupted index**
   ```bash
   mv ~/.noisefs/index.json ~/.noisefs/index.json.backup
   ```

2. **Rebuild from descriptors**
   ```bash
   # List known descriptor CIDs
   ls ~/.noisefs/descriptors/
   
   # Re-import files
   noisefs import --descriptor <cid> --name "recovered-file"
   ```

3. **Start fresh**
   ```bash
   rm ~/.noisefs/index.json
   noisefs list  # Creates new index
   ```

### Performance Issues

**Problem**: Slow upload/download speeds

**Solutions**:

1. **Check IPFS peers**
   ```bash
   ipfs swarm peers | wc -l
   ```

2. **Connect to more peers**
   ```bash
   ipfs bootstrap add default
   ```

3. **Optimize for local network**
   ```bash
   noisefs-config set privacy.default_level low
   ```

4. **Enable performance metrics**
   ```bash
   noisefs --debug status --detailed
   ```

### Web UI Issues

**Problem**: `Error: TLS handshake error`

**Solutions**:

1. **Generate certificates**
   ```bash
   noisefs webui --generate-cert
   ```

2. **Use HTTP for local testing**
   ```bash
   noisefs-config set webui.tls.enabled false
   ```

3. **Specify custom certificates**
   ```bash
   noisefs webui --cert server.crt --key server.key
   ```

## Diagnostic Commands

### System Health Check

```bash
# Full system status
noisefs status --detailed

# Test IPFS connectivity
noisefs test-ipfs

# Verify configuration
noisefs-config validate
```

### Debug Mode

```bash
# Enable debug logging
export NOISEFS_LOGGING_LEVEL=debug

# Run with debug output
noisefs --debug upload test.txt

# Save debug log
noisefs --debug upload test.txt 2> debug.log
```

### Performance Analysis

```bash
# Run benchmark
noisefs benchmark

# Check cache performance
noisefs cache stats

# Monitor operations
watch -n 1 'noisefs status'
```

## Error Messages

### Common Errors and Meanings

| Error | Meaning | Solution |
|-------|---------|----------|
| `no such file or directory` | File not found | Check file path |
| `permission denied` | Insufficient permissions | Check file/directory permissions |
| `address already in use` | Port conflict | Change web UI port |
| `no space left on device` | Disk full | Free disk space |
| `context deadline exceeded` | Operation timeout | Increase timeout setting |
| `block not found` | Missing IPFS block | Check IPFS connectivity |

## Getting More Help

### Enable Verbose Logging

```bash
# Maximum debug output
noisefs-config set logging.level debug
noisefs-config set logging.format text
```

### Collect Diagnostic Information

```bash
# System information
noisefs version --verbose
noisefs status --detailed > diagnostics.txt
noisefs cache stats >> diagnostics.txt
ipfs version >> diagnostics.txt
ipfs id >> diagnostics.txt
```

### Report Issues

When reporting issues, include:

1. NoiseFS version: `noisefs version`
2. Operating system and version
3. IPFS version: `ipfs version`
4. Error messages and debug logs
5. Steps to reproduce the issue

Report issues at: https://github.com/TheEntropyCollective/noisefs/issues

## FAQ

**Q: Can I use NoiseFS without IPFS?**
A: No, IPFS is currently the only supported storage backend.

**Q: Why is my upload slow?**
A: Check your privacy level. High privacy adds overhead. Use `--privacy low` for better performance.

**Q: Can I recover deleted files?**
A: Only if you have the descriptor CID. NoiseFS doesn't store deletion history.

**Q: Is my data encrypted?**
A: Data is anonymized through XOR operations. For additional encryption, enable `blocks.encryption` in config.

**Q: Can I use NoiseFS on Windows?**
A: Windows is not currently supported due to FUSE requirements. Use WSL2 as a workaround.