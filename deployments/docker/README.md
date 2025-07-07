# NoiseFS Docker Deployment Guide

This guide covers various deployment scenarios for NoiseFS using Docker and Kubernetes.

## Quick Start

### Single Node Deployment

```bash
# Build the image
docker build -t noisefs .

# Run daemon only
docker run -d --name noisefs-daemon \
  -p 4001:4001 -p 5001:5001 \
  -v noisefs-data:/opt/noisefs/data \
  noisefs daemon

# Run with Web UI
docker-compose up -d
```

### Accessing Services

- **Web UI**: http://localhost:8080
- **IPFS API**: http://localhost:5001
- **IPFS Swarm**: Port 4001

## Deployment Scenarios

### 1. Development Environment

```bash
# Use development override
docker-compose -f docker-compose.yml -f docker-compose.override.yml up -d

# Access development shell
docker-compose exec noisefs-dev shell
```

Features:
- Debug logging enabled
- Source code mounted for development
- Hot reloading capabilities

### 2. Production Environment

```bash
# Use production configuration
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# With SSL and load balancing
cp docker/ssl/example.crt docker/ssl/cert.pem
cp docker/ssl/example.key docker/ssl/key.pem
```

Features:
- Resource limits and health checks
- Nginx load balancer with SSL
- Optimized logging and caching

### 3. FUSE Filesystem Support

```bash
# Check FUSE availability
docker run --rm noisefs config check-fuse

# Run with FUSE (requires privileges)
docker-compose --profile fuse up -d

# Manual FUSE container
docker run -d --name noisefs-mount \
  --device /dev/fuse \
  --cap-add SYS_ADMIN \
  -v /mnt/noisefs:/opt/noisefs/mount:shared \
  noisefs mount
```

FUSE Requirements:
- `/dev/fuse` device access
- `SYS_ADMIN` capability or privileged mode
- Proper mount point permissions

### 4. Multi-Node Cluster

```bash
# Deploy cluster with service discovery
docker-compose -f docker-compose.cluster.yml up -d

# Scale nodes
docker-compose -f docker-compose.cluster.yml up -d --scale noisefs-node=5

# Access cluster UI
open http://localhost:8080
```

Features:
- Automatic service discovery with Consul
- Load balanced access
- Cluster health monitoring

### 5. Monitoring and Metrics

```bash
# Enable monitoring stack
docker-compose --profile monitoring up -d

# Access dashboards
open http://localhost:3000  # Grafana (admin/admin)
open http://localhost:9090  # Prometheus
```

Monitoring includes:
- Performance metrics collection
- Health and availability monitoring
- Resource usage tracking
- Custom NoiseFS dashboards

## Kubernetes Deployment

### Prerequisites

```bash
# Create namespace
kubectl apply -f docker/kubernetes/namespace.yaml

# Apply configuration
kubectl apply -f docker/kubernetes/configmap.yaml
kubectl apply -f docker/kubernetes/persistentvolume.yaml
```

### Basic Deployment

```bash
# Deploy services
kubectl apply -f docker/kubernetes/deployment.yaml
kubectl apply -f docker/kubernetes/service.yaml

# Check status
kubectl get pods -n noisefs
kubectl get services -n noisefs
```

### FUSE Support in Kubernetes

```bash
# Deploy FUSE DaemonSet (requires privileged containers)
kubectl apply -f docker/kubernetes/fuse-daemonset.yaml

# Verify mounts on nodes
kubectl exec -n noisefs daemonset/noisefs-fuse -- mount | grep noisefs
```

### Accessing Services

```bash
# Get service URLs
kubectl get services -n noisefs

# Port forward for local access
kubectl port-forward -n noisefs service/noisefs-webui 8080:80

# Access Web UI
open http://localhost:8080
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NOISEFS_CONFIG_FILE` | Configuration file path | `/opt/noisefs/config/config.json` |
| `NOISEFS_DATA_DIR` | Data storage directory | `/opt/noisefs/data` |
| `NOISEFS_LOG_DIR` | Log output directory | `/opt/noisefs/logs` |
| `NOISEFS_CACHE_DIR` | Cache storage directory | `/opt/noisefs/cache` |
| `NOISEFS_MOUNT_POINT` | FUSE mount point | `/opt/noisefs/mount` |
| `NOISEFS_LOG_LEVEL` | Logging level | `info` |

### Volume Mounts

| Path | Purpose | Type |
|------|---------|------|
| `/opt/noisefs/data` | Persistent data storage | Persistent Volume |
| `/opt/noisefs/logs` | Log files | EmptyDir or HostPath |
| `/opt/noisefs/cache` | Cache storage | EmptyDir or Persistent |
| `/opt/noisefs/config` | Configuration files | ConfigMap |
| `/opt/noisefs/mount` | FUSE mount point | Shared or HostPath |

### Port Mappings

| Port | Service | Protocol | Description |
|------|---------|----------|-------------|
| 4001 | IPFS Swarm | TCP | P2P communication |
| 5001 | IPFS API | TCP | IPFS API access |
| 8080 | Web UI/Metrics | TCP | Web interface and metrics |
| 80/443 | Load Balancer | TCP | HTTP/HTTPS access |

## Security Considerations

### FUSE Security

- FUSE requires elevated privileges (`SYS_ADMIN` capability)
- Consider using `--cap-add SYS_ADMIN` instead of `--privileged`
- Isolate FUSE containers with proper namespaces
- Use read-only root filesystems where possible

### Network Security

- Use firewalls to restrict port access
- Enable TLS for all external communications
- Implement proper authentication for Web UI
- Use network policies in Kubernetes

### Data Security

- Encrypt data at rest using volume encryption
- Use secrets management for sensitive configuration
- Implement backup and disaster recovery procedures
- Monitor access logs and audit trails

## Troubleshooting

### Common Issues

#### FUSE Not Working

```bash
# Check FUSE availability
docker run --rm noisefs docker/fuse-setup.sh check

# Verify permissions
docker run --rm --device /dev/fuse --cap-add SYS_ADMIN noisefs docker/fuse-setup.sh test
```

#### Container Won't Start

```bash
# Check logs
docker logs noisefs-daemon

# Verify configuration
docker run --rm -v $(pwd)/config:/config noisefs config validate /config/config.json
```

#### Performance Issues

```bash
# Run benchmarks
docker run --rm noisefs benchmark --type basic

# Check resource usage
docker stats noisefs-daemon
```

### Health Checks

```bash
# Check daemon health
curl http://localhost:8080/health

# Verify IPFS connectivity
curl http://localhost:5001/api/v0/version

# Test FUSE mount
ls /mnt/noisefs
```

### Log Analysis

```bash
# View real-time logs
docker-compose logs -f noisefs-daemon

# Export logs for analysis
docker run --rm -v noisefs-logs:/logs alpine tar czf - -C /logs . > noisefs-logs.tar.gz
```

## Performance Tuning

### Resource Allocation

- **CPU**: Minimum 0.5 cores, recommended 1+ cores per node
- **Memory**: Minimum 512MB, recommended 1GB+ per node
- **Storage**: SSD recommended for cache and data directories
- **Network**: High bandwidth for IPFS swarm communication

### Cache Configuration

```json
{
  "cache": {
    "max_size": 1000000,
    "eviction_policy": "adaptive",
    "read_ahead_size": 8,
    "write_back_buffer": 200
  }
}
```

### IPFS Optimization

```json
{
  "ipfs": {
    "timeout": "30s",
    "retry_attempts": 3,
    "max_connections": 100,
    "bootstrap_peers": [
      "/ip4/node1.example.com/tcp/4001/p2p/...",
      "/ip4/node2.example.com/tcp/4001/p2p/..."
    ]
  }
}
```

## Scaling and High Availability

### Horizontal Scaling

```bash
# Scale daemon nodes
docker-compose -f docker-compose.cluster.yml up -d --scale noisefs-node=5

# Scale Web UI
kubectl scale deployment noisefs-webui --replicas=3 -n noisefs
```

### Load Balancing

- Use Nginx or HAProxy for HTTP load balancing
- Implement health checks for automatic failover
- Consider geographic distribution for global access

### Data Replication

- Configure IPFS for appropriate replication factors
- Use persistent volumes with replication in Kubernetes
- Implement regular backups and restore procedures

## Backup and Recovery

### Data Backup

```bash
# Backup data volume
docker run --rm -v noisefs-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/noisefs-data-$(date +%Y%m%d).tar.gz -C /data .

# Backup configuration
docker run --rm -v noisefs-config:/config -v $(pwd):/backup alpine \
  tar czf /backup/noisefs-config-$(date +%Y%m%d).tar.gz -C /config .
```

### Disaster Recovery

```bash
# Restore data volume
docker run --rm -v noisefs-data:/data -v $(pwd):/backup alpine \
  tar xzf /backup/noisefs-data-20240101.tar.gz -C /data

# Restart services
docker-compose restart
```

## Support and Documentation

- [NoiseFS GitHub Repository](https://github.com/TheEntropyCollective/noisefs)
- [Docker Documentation](https://docs.docker.com/)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [IPFS Documentation](https://docs.ipfs.io/)

For issues and support, please check the GitHub issues or create a new issue with detailed information about your deployment environment.