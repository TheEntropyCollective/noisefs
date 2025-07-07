#!/bin/bash
# NoiseFS Deployment Script

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENTS_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_DIR="$(dirname "$DEPLOYMENTS_DIR")"
IMAGE_NAME="noisefs"
IMAGE_TAG="latest"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    local deps=("docker" "docker-compose")
    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &> /dev/null; then
            log_error "$dep is not installed"
            exit 1
        fi
    done
    
    # Check Docker daemon
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi
    
    log_success "All dependencies are available"
}

# Function to build Docker image
build_image() {
    log_info "Building NoiseFS Docker image..."
    
    cd "$PROJECT_DIR"
    
    if docker build -t "${IMAGE_NAME}:${IMAGE_TAG}" .; then
        log_success "Docker image built successfully"
    else
        log_error "Failed to build Docker image"
        exit 1
    fi
}

# Function to deploy single node
deploy_single() {
    log_info "Deploying single node NoiseFS..."
    
    cd "$PROJECT_DIR"
    
    # Create necessary directories
    mkdir -p config data logs cache
    
    # Copy example configuration if needed
    if [ ! -f "config/config.json" ]; then
        if [ -f "config.example.json" ]; then
            cp config.example.json config/config.json
            log_info "Created configuration from example"
        fi
    fi
    
    # Deploy using docker-compose
    if docker-compose up -d; then
        log_success "Single node deployment completed"
        log_info "Web UI available at: http://localhost:8080"
        log_info "IPFS API available at: http://localhost:5001"
    else
        log_error "Deployment failed"
        exit 1
    fi
}

# Function to deploy cluster
deploy_cluster() {
    log_info "Deploying NoiseFS cluster..."
    
    cd "$PROJECT_DIR"
    
    local nodes=${1:-3}
    log_info "Deploying cluster with $nodes nodes"
    
    if docker-compose -f docker-compose.cluster.yml up -d --scale noisefs-node="$nodes"; then
        log_success "Cluster deployment completed"
        log_info "Cluster load balancer available at: http://localhost:8080"
        log_info "Consul UI available at: http://localhost:8500"
    else
        log_error "Cluster deployment failed"
        exit 1
    fi
}

# Function to deploy with FUSE
deploy_fuse() {
    log_info "Deploying NoiseFS with FUSE support..."
    
    # Check FUSE availability
    if [ ! -c /dev/fuse ]; then
        log_error "/dev/fuse device not found"
        log_error "FUSE is not available on this system"
        exit 1
    fi
    
    cd "$PROJECT_DIR"
    
    if docker-compose --profile fuse up -d; then
        log_success "FUSE deployment completed"
        log_info "Filesystem will be mounted at: /opt/noisefs/mount"
    else
        log_error "FUSE deployment failed"
        exit 1
    fi
}

# Function to deploy monitoring
deploy_monitoring() {
    log_info "Deploying NoiseFS with monitoring..."
    
    cd "$PROJECT_DIR"
    
    if docker-compose --profile monitoring up -d; then
        log_success "Monitoring deployment completed"
        log_info "Grafana available at: http://localhost:3000 (admin/admin)"
        log_info "Prometheus available at: http://localhost:9090"
    else
        log_error "Monitoring deployment failed"
        exit 1
    fi
}

# Function to deploy to Kubernetes
deploy_kubernetes() {
    log_info "Deploying NoiseFS to Kubernetes..."
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        exit 1
    fi
    
    # Check cluster connection
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    cd "$PROJECT_DIR"
    
    # Apply manifests
    local manifests=(
        "docker/kubernetes/namespace.yaml"
        "docker/kubernetes/configmap.yaml"
        "docker/kubernetes/persistentvolume.yaml"
        "docker/kubernetes/deployment.yaml"
        "docker/kubernetes/service.yaml"
    )
    
    for manifest in "${manifests[@]}"; do
        if [ -f "$manifest" ]; then
            log_info "Applying $manifest"
            kubectl apply -f "$manifest"
        else
            log_warning "Manifest $manifest not found"
        fi
    done
    
    # Wait for deployments
    log_info "Waiting for deployments to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/noisefs-daemon -n noisefs
    kubectl wait --for=condition=available --timeout=300s deployment/noisefs-webui -n noisefs
    
    log_success "Kubernetes deployment completed"
    log_info "Use 'kubectl port-forward -n noisefs service/noisefs-webui 8080:80' to access Web UI"
}

# Function to check deployment status
check_status() {
    log_info "Checking deployment status..."
    
    if docker-compose ps 2>/dev/null | grep -q "Up"; then
        log_success "Docker Compose services are running"
        docker-compose ps
    fi
    
    if command -v kubectl &> /dev/null; then
        if kubectl get pods -n noisefs 2>/dev/null | grep -q "Running"; then
            log_success "Kubernetes pods are running"
            kubectl get pods -n noisefs
        fi
    fi
}

# Function to clean up deployment
cleanup() {
    log_info "Cleaning up NoiseFS deployment..."
    
    # Stop Docker Compose services
    if [ -f "$PROJECT_DIR/docker-compose.yml" ]; then
        cd "$PROJECT_DIR"
        docker-compose down -v
        docker-compose -f docker-compose.cluster.yml down -v 2>/dev/null || true
        log_info "Docker Compose services stopped"
    fi
    
    # Clean up Kubernetes resources
    if command -v kubectl &> /dev/null; then
        if kubectl get namespace noisefs &> /dev/null; then
            kubectl delete namespace noisefs
            log_info "Kubernetes resources cleaned up"
        fi
    fi
    
    # Remove Docker image
    if docker images | grep -q "$IMAGE_NAME"; then
        docker rmi "${IMAGE_NAME}:${IMAGE_TAG}" 2>/dev/null || true
        log_info "Docker image removed"
    fi
    
    log_success "Cleanup completed"
}

# Function to show usage
usage() {
    cat << EOF
NoiseFS Deployment Script

Usage: $0 [COMMAND] [OPTIONS]

Commands:
  build                 Build Docker image
  deploy [TYPE]         Deploy NoiseFS
    single              Single node deployment (default)
    cluster [nodes]     Multi-node cluster deployment
    fuse                Deployment with FUSE support
    monitoring          Deployment with monitoring stack
    kubernetes          Deploy to Kubernetes cluster
  status                Check deployment status
  cleanup               Clean up all deployments
  help                  Show this help message

Examples:
  $0 build                    # Build Docker image
  $0 deploy single            # Deploy single node
  $0 deploy cluster 5         # Deploy 5-node cluster
  $0 deploy fuse              # Deploy with FUSE
  $0 deploy monitoring        # Deploy with monitoring
  $0 deploy kubernetes        # Deploy to Kubernetes
  $0 status                   # Check status
  $0 cleanup                  # Clean up everything

Environment Variables:
  IMAGE_NAME              Docker image name (default: noisefs)
  IMAGE_TAG               Docker image tag (default: latest)

EOF
}

# Main script logic
main() {
    case "${1:-}" in
        build)
            check_dependencies
            build_image
            ;;
        deploy)
            check_dependencies
            build_image
            case "${2:-single}" in
                single)
                    deploy_single
                    ;;
                cluster)
                    deploy_cluster "${3:-3}"
                    ;;
                fuse)
                    deploy_fuse
                    ;;
                monitoring)
                    deploy_monitoring
                    ;;
                kubernetes)
                    deploy_kubernetes
                    ;;
                *)
                    log_error "Unknown deployment type: ${2}"
                    usage
                    exit 1
                    ;;
            esac
            ;;
        status)
            check_status
            ;;
        cleanup)
            cleanup
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: ${1:-}"
            usage
            exit 1
            ;;
    esac
}

# Handle signals
trap 'log_warning "Deployment interrupted"; exit 130' INT TERM

# Run main function
main "$@"