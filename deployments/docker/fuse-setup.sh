#!/bin/bash
# FUSE setup script for NoiseFS containers

set -e

# Function to log messages
log() {
    echo "[FUSE-SETUP] $*"
}

# Function to check if running in container
is_container() {
    [ -f /.dockerenv ] || grep -q 'docker\|lxc' /proc/1/cgroup 2>/dev/null
}

# Function to check FUSE availability
check_fuse_available() {
    if [ ! -c /dev/fuse ]; then
        log "ERROR: /dev/fuse device not available"
        log "Make sure to run container with: --device /dev/fuse"
        return 1
    fi
    
    if ! command -v fusermount >/dev/null 2>&1; then
        log "ERROR: fusermount command not available"
        log "FUSE userspace tools are not properly installed"
        return 1
    fi
    
    return 0
}

# Function to check container permissions
check_permissions() {
    local has_sys_admin=false
    local is_privileged=false
    
    # Check for SYS_ADMIN capability
    if capsh --print 2>/dev/null | grep -q "sys_admin"; then
        has_sys_admin=true
    fi
    
    # Check if running in privileged mode
    if [ -r /proc/self/status ]; then
        if grep -q "^CapEff:[[:space:]]*[0-9a-f]*f[0-9a-f]*" /proc/self/status 2>/dev/null; then
            is_privileged=true
        fi
    fi
    
    if [ "$has_sys_admin" = false ] && [ "$is_privileged" = false ]; then
        log "ERROR: Insufficient permissions for FUSE operations"
        log "Run container with: --cap-add SYS_ADMIN or --privileged"
        return 1
    fi
    
    log "Container has sufficient permissions for FUSE"
    return 0
}

# Function to configure FUSE
configure_fuse() {
    local fuse_conf="/etc/fuse.conf"
    
    # Enable user_allow_other if not already set
    if [ -f "$fuse_conf" ]; then
        if ! grep -q "^user_allow_other" "$fuse_conf"; then
            log "Enabling user_allow_other in $fuse_conf"
            echo "user_allow_other" >> "$fuse_conf"
        fi
    else
        log "Creating $fuse_conf with user_allow_other"
        echo "user_allow_other" > "$fuse_conf"
    fi
    
    # Set appropriate permissions
    chmod 644 "$fuse_conf"
}

# Function to test FUSE functionality
test_fuse() {
    local test_dir="/tmp/fuse_test"
    local mount_point="/tmp/fuse_mount"
    
    # Clean up any previous test
    cleanup_test() {
        if mountpoint -q "$mount_point" 2>/dev/null; then
            fusermount -u "$mount_point" 2>/dev/null || true
        fi
        rm -rf "$test_dir" "$mount_point"
    }
    
    trap cleanup_test EXIT
    
    # Create test directories
    mkdir -p "$test_dir" "$mount_point"
    
    # Try to mount a simple FUSE filesystem (using bindfs if available)
    if command -v bindfs >/dev/null 2>&1; then
        log "Testing FUSE with bindfs"
        if bindfs "$test_dir" "$mount_point" 2>/dev/null; then
            if mountpoint -q "$mount_point"; then
                log "FUSE test successful"
                fusermount -u "$mount_point"
                return 0
            fi
        fi
    fi
    
    # Alternative test: check if we can access /dev/fuse
    if [ -r /dev/fuse ] && [ -w /dev/fuse ]; then
        log "FUSE device is accessible"
        return 0
    fi
    
    log "WARNING: FUSE functionality test failed"
    return 1
}

# Function to provide setup instructions
show_instructions() {
    cat << EOF

=== NoiseFS FUSE Setup Instructions ===

To enable FUSE functionality in NoiseFS containers:

1. Docker Run:
   docker run --device /dev/fuse --cap-add SYS_ADMIN noisefs mount

2. Docker Compose:
   Add to service definition:
   ```yaml
   privileged: true
   devices:
     - /dev/fuse:/dev/fuse
   cap_add:
     - SYS_ADMIN
   ```

3. For Kubernetes:
   ```yaml
   securityContext:
     privileged: true
     capabilities:
       add: ["SYS_ADMIN"]
   volumeMounts:
     - name: dev-fuse
       mountPath: /dev/fuse
   volumes:
     - name: dev-fuse
       hostPath:
         path: /dev/fuse
   ```

4. Mount propagation:
   For shared mounts, use:
   ```yaml
   volumes:
     - type: bind
       source: /path/to/mount
       target: /opt/noisefs/mount
       bind:
         propagation: shared
   ```

Security Considerations:
- SYS_ADMIN capability provides broad privileges
- Consider using --cap-add SYS_ADMIN instead of --privileged
- Limit container access to necessary resources only
- Use mount namespaces to isolate mount points

EOF
}

# Main function
main() {
    local command="${1:-check}"
    
    case "$command" in
        check)
            log "Checking FUSE setup..."
            
            if ! is_container; then
                log "Not running in a container"
                exit 0
            fi
            
            if check_fuse_available && check_permissions; then
                configure_fuse
                if test_fuse; then
                    log "FUSE is properly configured and functional"
                    exit 0
                else
                    log "FUSE configuration complete but test failed"
                    exit 1
                fi
            else
                log "FUSE setup failed"
                show_instructions
                exit 1
            fi
            ;;
        
        configure)
            log "Configuring FUSE..."
            configure_fuse
            log "FUSE configuration complete"
            ;;
            
        test)
            log "Testing FUSE functionality..."
            if test_fuse; then
                log "FUSE test passed"
                exit 0
            else
                log "FUSE test failed"
                exit 1
            fi
            ;;
            
        instructions)
            show_instructions
            ;;
            
        *)
            echo "Usage: $0 {check|configure|test|instructions}"
            echo "  check        - Check and configure FUSE (default)"
            echo "  configure    - Configure FUSE settings"
            echo "  test         - Test FUSE functionality"
            echo "  instructions - Show setup instructions"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"