# Troubleshooting Guide

## Common Issues

### IPFS Connection Problems

#### Symptom: "Failed to connect to IPFS"

**Cause**: IPFS daemon not running or unreachable.

**Solutions**:
1. Start IPFS daemon: `ipfs daemon`
2. Check IPFS is listening: `curl http://127.0.0.1:5001/api/v0/version`
3. Verify NoiseFS endpoint: `export NOISEFS_IPFS_ENDPOINT="127.0.0.1:5001"`

#### Symptom: "Connection refused on port 5001"

**Cause**: IPFS API not accessible or firewall blocking.

**Solutions**:
1. Check IPFS config: `ipfs config Addresses.API`
2. Open firewall: `sudo ufw allow 5001`
3. Bind to all interfaces: `ipfs config Addresses.API /ip4/0.0.0.0/tcp/5001`

### FUSE Mount Issues

#### Symptom: "Permission denied" when mounting

**Cause**: User lacks FUSE permissions.

**Solutions**:
1. Install FUSE: `sudo apt install fuse` (Ubuntu) / `brew install macfuse` (macOS)
2. Add user to fuse group: `sudo usermod -a -G fuse $USER`
3. Logout and login again
4. Check device permissions: `ls -l /dev/fuse`

#### Symptom: "Transport endpoint is not connected"

**Cause**: Previous mount not properly unmounted.

**Solutions**:
1. Force unmount: `sudo fusermount -uz /mount/point`
2. Kill any hanging processes: `sudo pkill -f noisefs`
3. Try mounting again

### Upload/Download Failures

#### Symptom: "Block not found"

**Cause**: Required blocks not available in IPFS network.

**Solutions**:
1. Check IPFS connectivity: `ipfs swarm peers`
2. Try pinning important blocks: `ipfs pin add <block-cid>`
3. Verify descriptor integrity: Check CID format
4. Wait for network propagation (may take minutes)

#### Symptom: "Descriptor invalid"

**Cause**: Corrupted or malformed descriptor.

**Solutions**:
1. Verify descriptor CID format
2. Check if descriptor exists: `ipfs cat <descriptor-cid>`
3. Try downloading descriptor separately
4. Restore from backup if available

### Performance Issues

#### Symptom: Very slow uploads/downloads

**Cause**: Network congestion, poor IPFS connectivity, or cache misses.

**Solutions**:
1. Check IPFS peer count: `ipfs swarm peers | wc -l`
2. Connect to more peers: `ipfs bootstrap add <peer-address>`
3. Increase cache size: `export NOISEFS_CACHE_SIZE=500`
4. Use local IPFS node for better performance

#### Symptom: High memory usage

**Cause**: Large cache size or memory leaks.

**Solutions**:
1. Reduce cache size: `export NOISEFS_CACHE_SIZE=50`
2. Restart NoiseFS periodically
3. Monitor with: `ps aux | grep noisefs`
4. Check for memory leaks in logs

### Build and Installation Issues

#### Symptom: "Go version too old"

**Cause**: NoiseFS requires Go 1.19+.

**Solutions**:
1. Update Go: Download from https://golang.org/dl/
2. Verify version: `go version`
3. Update PATH if needed

#### Symptom: "Module not found" during build

**Cause**: Go modules not properly downloaded.

**Solutions**:
1. Clean module cache: `go clean -modcache`
2. Download dependencies: `go mod download`
3. Verify go.mod file exists
4. Try: `go mod tidy`

### Configuration Issues

#### Symptom: NoiseFS ignoring config file

**Cause**: Config file in wrong location or invalid format.

**Solutions**:
1. Check config location: `~/.noisefs/config.json`
2. Validate JSON syntax: `python -m json.tool ~/.noisefs/config.json`
3. Use environment variables instead
4. Check file permissions

#### Symptom: "Invalid block size"

**Cause**: Block size not power of 2 or too small/large.

**Solutions**:
1. Use default: `export NOISEFS_BLOCK_SIZE=131072`
2. Valid sizes: 32768, 65536, 131072, 262144
3. Larger blocks = better performance, worse privacy

## Diagnostic Commands

### Check System Status

```bash
# NoiseFS version and build info
noisefs version

# IPFS connectivity
noisefs status

# System statistics
noisefs stats

# Cache statistics
noisefs cache-stats
```

### Debug Logging

```bash
# Enable debug logging
export NOISEFS_LOG_LEVEL=debug
noisefs upload test.txt

# Log to file
noisefs upload test.txt 2> debug.log

# Analyze logs
grep ERROR debug.log
```

### Network Diagnostics

```bash
# Check IPFS peers
ipfs swarm peers

# Test IPFS API
curl -X POST http://127.0.0.1:5001/api/v0/version

# Check NoiseFS connectivity
noisefs ping
```

## Getting Help

### Log Analysis

When reporting issues, include:

1. **NoiseFS version**: `noisefs version`
2. **Go version**: `go version`
3. **IPFS version**: `ipfs version`
4. **Operating system**: `uname -a`
5. **Error logs**: Debug output showing the issue
6. **Reproduction steps**: Exact commands that trigger the problem

### Performance Benchmarks

```bash
# Basic performance test
time noisefs upload large-file.dat
time noisefs download <descriptor> recovered.dat

# Compare with IPFS
time ipfs add large-file.dat
time ipfs cat <ipfs-hash> > ipfs-recovered.dat
```

### Common Solutions Summary

| Issue | Quick Fix |
|-------|-----------|
| IPFS connection | `ipfs daemon` |
| FUSE permissions | `sudo usermod -a -G fuse $USER` |
| Slow performance | Increase cache size |
| Block not found | Wait, check IPFS peers |
| Build failure | Update Go version |
| Mount failure | `fusermount -u` then retry |

### When to File a Bug Report

File a bug report if:
- Issue persists after trying troubleshooting steps
- Error messages indicate internal NoiseFS problems
- Performance significantly worse than expected
- Data corruption or integrity issues occur

Include detailed logs, system information, and reproduction steps for faster resolution.