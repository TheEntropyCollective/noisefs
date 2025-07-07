#!/bin/bash
set -e

# NoiseFS Docker Entrypoint Script

# Function to log messages
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# Function to check if FUSE is available
check_fuse() {
    if [ ! -c /dev/fuse ]; then
        log "WARNING: /dev/fuse not available. FUSE functionality will be disabled."
        log "To enable FUSE, run container with: --device /dev/fuse --cap-add SYS_ADMIN"
        return 1
    fi
    return 0
}

# Function to initialize configuration
init_config() {
    local config_file="$NOISEFS_CONFIG_FILE"
    
    if [ ! -f "$config_file" ]; then
        log "Creating default configuration at $config_file"
        
        # Create config directory if it doesn't exist
        mkdir -p "$(dirname "$config_file")"
        
        # Generate configuration using noisefs-config tool
        /opt/noisefs/bin/noisefs-config generate \
            --output "$config_file" \
            --data-dir "$NOISEFS_DATA_DIR" \
            --log-dir "$NOISEFS_LOG_DIR" \
            --cache-dir "$NOISEFS_CACHE_DIR"
        
        log "Configuration created successfully"
    else
        log "Using existing configuration at $config_file"
    fi
}

# Function to ensure directories exist
ensure_directories() {
    local dirs=(
        "$NOISEFS_DATA_DIR"
        "$NOISEFS_LOG_DIR" 
        "$NOISEFS_CACHE_DIR"
        "$NOISEFS_MOUNT_POINT"
    )
    
    for dir in "${dirs[@]}"; do
        if [ ! -d "$dir" ]; then
            log "Creating directory: $dir"
            mkdir -p "$dir"
        fi
    done
}

# Function to run daemon mode
run_daemon() {
    log "Starting NoiseFS daemon"
    
    # Initialize
    init_config
    ensure_directories
    
    # Check FUSE availability
    FUSE_AVAILABLE=false
    if check_fuse; then
        FUSE_AVAILABLE=true
        log "FUSE is available"
    fi
    
    # Start the main daemon
    exec /opt/noisefs/bin/noisefs daemon \
        --config "$NOISEFS_CONFIG_FILE" \
        --log-level info
}

# Function to run mount mode
run_mount() {
    log "Starting NoiseFS FUSE mount"
    
    # Check FUSE availability
    if ! check_fuse; then
        log "ERROR: FUSE is required for mount mode"
        exit 1
    fi
    
    init_config
    ensure_directories
    
    local mount_point="${2:-$NOISEFS_MOUNT_POINT}"
    
    log "Mounting NoiseFS at $mount_point"
    exec /opt/noisefs/bin/noisefs-mount \
        --config "$NOISEFS_CONFIG_FILE" \
        --mount-point "$mount_point" \
        --foreground
}

# Function to run webui mode
run_webui() {
    log "Starting NoiseFS Web UI"
    
    init_config
    ensure_directories
    
    # Start the web UI
    exec /opt/noisefs/bin/webui \
        --config "$NOISEFS_CONFIG_FILE" \
        --port 8080
}

# Function to run benchmark mode
run_benchmark() {
    log "Running NoiseFS benchmarks"
    
    init_config
    ensure_directories
    
    # Run benchmarks
    exec /opt/noisefs/bin/noisefs-benchmark \
        --config "$NOISEFS_CONFIG_FILE" \
        "${@:2}"
}

# Function to run config mode
run_config() {
    log "Running NoiseFS configuration tool"
    
    ensure_directories
    
    # Run config tool
    exec /opt/noisefs/bin/noisefs-config "${@:2}"
}

# Function to run shell
run_shell() {
    log "Starting interactive shell"
    exec /bin/bash
}

# Function to show help
show_help() {
    cat << EOF
NoiseFS Docker Container

Usage:
  docker run noisefs [COMMAND] [OPTIONS]

Commands:
  daemon      Start NoiseFS daemon (default)
  mount       Mount NoiseFS filesystem via FUSE
  webui       Start Web UI interface
  benchmark   Run performance benchmarks
  config      Configuration management tool
  shell       Start interactive shell
  help        Show this help message

Environment Variables:
  NOISEFS_CONFIG_FILE   Configuration file path (default: /opt/noisefs/config/config.json)
  NOISEFS_DATA_DIR      Data directory (default: /opt/noisefs/data)
  NOISEFS_LOG_DIR       Log directory (default: /opt/noisefs/logs)
  NOISEFS_CACHE_DIR     Cache directory (default: /opt/noisefs/cache)
  NOISEFS_MOUNT_POINT   Mount point for FUSE (default: /opt/noisefs/mount)

Examples:
  # Run daemon
  docker run noisefs daemon
  
  # Mount filesystem (requires --device /dev/fuse --cap-add SYS_ADMIN)
  docker run --device /dev/fuse --cap-add SYS_ADMIN noisefs mount
  
  # Start Web UI
  docker run -p 8080:8080 noisefs webui
  
  # Run benchmarks
  docker run noisefs benchmark --type basic
  
  # Generate configuration
  docker run noisefs config generate --output /tmp/config.json

For FUSE functionality, run with:
  --device /dev/fuse --cap-add SYS_ADMIN
EOF
}

# Main script logic
main() {
    local command="${1:-daemon}"
    
    case "$command" in
        daemon)
            run_daemon
            ;;
        mount)
            run_mount "$@"
            ;;
        webui)
            run_webui
            ;;
        benchmark)
            run_benchmark "$@"
            ;;
        config)
            run_config "$@"
            ;;
        shell)
            run_shell
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log "Unknown command: $command"
            show_help
            exit 1
            ;;
    esac
}

# Handle signals gracefully
trap 'log "Received signal, shutting down..."; exit 0' SIGTERM SIGINT

# Run main function
main "$@"