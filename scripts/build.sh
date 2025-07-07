#!/bin/bash
# NoiseFS Build Script

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="${PROJECT_DIR}/bin"
DIST_DIR="${PROJECT_DIR}/dist"

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

# Function to show usage
usage() {
    cat << EOF
NoiseFS Build Script

Usage: $0 [COMMAND] [OPTIONS]

Commands:
  build                 Build all binaries (default)
  clean                 Clean build artifacts
  test                  Run tests
  bench                 Run benchmarks
  lint                  Run linters
  dist                  Create distribution packages
  install               Install binaries to system
  help                  Show this help message

Options:
  --target TARGET       Build target (linux, darwin, windows)
  --arch ARCH          Target architecture (amd64, arm64)
  --race               Enable race detection
  --tags TAGS          Build tags (e.g., fuse)
  --ldflags FLAGS      Additional linker flags
  --output DIR         Output directory (default: bin/)

Examples:
  $0 build                           # Build all binaries
  $0 build --race                    # Build with race detection
  $0 build --tags fuse               # Build with FUSE support
  $0 build --target linux --arch amd64  # Cross-compile for Linux
  $0 test                            # Run all tests
  $0 dist                           # Create distribution packages

Environment Variables:
  GOOS                 Target operating system
  GOARCH               Target architecture
  CGO_ENABLED          Enable/disable CGO (default: 0)
  NOISEFS_VERSION      Version string for binaries

EOF
}

# Function to check Go installation
check_go() {
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        exit 1
    fi
    
    local go_version
    go_version=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "Using Go version: $go_version"
}

# Function to get version information
get_version() {
    local version="dev"
    local commit="unknown"
    local date
    date=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Try to get version from git
    if command -v git &> /dev/null && git rev-parse --git-dir &> /dev/null; then
        commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        
        # Check if we're on a tag
        if git describe --tags --exact-match &> /dev/null; then
            version=$(git describe --tags --exact-match)
        else
            # Use branch name and commit
            local branch
            branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
            version="${branch}-${commit}"
        fi
    fi
    
    # Override with environment variable if set
    version="${NOISEFS_VERSION:-$version}"
    
    echo "$version" "$commit" "$date"
}

# Function to build a single binary
build_binary() {
    local cmd_name="$1"
    local target_os="${2:-$(go env GOOS)}"
    local target_arch="${3:-$(go env GOARCH)}"
    
    local cmd_dir="${PROJECT_DIR}/cmd/${cmd_name}"
    if [ ! -d "$cmd_dir" ]; then
        log_error "Command directory not found: $cmd_dir"
        return 1
    fi
    
    # Determine output filename
    local output_name="$cmd_name"
    if [ "$target_os" = "windows" ]; then
        output_name="${cmd_name}.exe"
    fi
    
    local output_path="${BUILD_DIR}/${output_name}"
    if [ "$target_os" != "$(go env GOOS)" ] || [ "$target_arch" != "$(go env GOARCH)" ]; then
        output_path="${BUILD_DIR}/${target_os}-${target_arch}/${output_name}"
        mkdir -p "$(dirname "$output_path")"
    fi
    
    # Get version information
    local version commit date
    read -r version commit date <<< "$(get_version)"
    
    # Build ldflags
    local ldflags="-s -w"
    ldflags="$ldflags -X 'main.Version=$version'"
    ldflags="$ldflags -X 'main.GitCommit=$commit'"
    ldflags="$ldflags -X 'main.BuildDate=$date'"
    
    # Add custom ldflags if provided
    if [ -n "${CUSTOM_LDFLAGS:-}" ]; then
        ldflags="$ldflags $CUSTOM_LDFLAGS"
    fi
    
    # Build tags
    local build_tags="${BUILD_TAGS:-}"
    local tags_flag=""
    if [ -n "$build_tags" ]; then
        tags_flag="-tags $build_tags"
    fi
    
    # Race detection
    local race_flag=""
    if [ "${ENABLE_RACE:-}" = "true" ]; then
        race_flag="-race"
    fi
    
    log_info "Building $cmd_name for $target_os/$target_arch..."
    
    # Set environment for cross-compilation
    export GOOS="$target_os"
    export GOARCH="$target_arch"
    export CGO_ENABLED="${CGO_ENABLED:-0}"
    
    if go build \
        $race_flag \
        $tags_flag \
        -ldflags "$ldflags" \
        -o "$output_path" \
        "$cmd_dir"; then
        log_success "Built $output_path"
        
        # Show binary info
        local size
        size=$(du -h "$output_path" | cut -f1)
        log_info "Binary size: $size"
        
        return 0
    else
        log_error "Failed to build $cmd_name"
        return 1
    fi
}

# Function to build all binaries
build_all() {
    local target_os="${1:-$(go env GOOS)}"
    local target_arch="${2:-$(go env GOARCH)}"
    
    log_info "Building all NoiseFS binaries for $target_os/$target_arch"
    
    # Create build directory
    mkdir -p "$BUILD_DIR"
    
    # List of commands to build
    local commands=(
        "noisefs"
        "noisefs-mount"
        "noisefs-benchmark" 
        "noisefs-config"
        "webui"
    )
    
    local failed=0
    for cmd in "${commands[@]}"; do
        if ! build_binary "$cmd" "$target_os" "$target_arch"; then
            failed=$((failed + 1))
        fi
    done
    
    if [ $failed -eq 0 ]; then
        log_success "All binaries built successfully"
    else
        log_error "$failed binaries failed to build"
        return 1
    fi
}

# Function to run tests
run_tests() {
    log_info "Running tests..."
    
    cd "$PROJECT_DIR"
    
    local test_flags="-v"
    if [ "${ENABLE_RACE:-}" = "true" ]; then
        test_flags="$test_flags -race"
    fi
    
    # Build tags for tests
    local build_tags="${BUILD_TAGS:-}"
    local tags_flag=""
    if [ -n "$build_tags" ]; then
        tags_flag="-tags $build_tags"
    fi
    
    if go test $test_flags $tags_flag ./...; then
        log_success "All tests passed"
    else
        log_error "Some tests failed"
        return 1
    fi
}

# Function to run benchmarks
run_benchmarks() {
    log_info "Running benchmarks..."
    
    cd "$PROJECT_DIR"
    
    local bench_flags="-bench=. -benchmem"
    
    # Build tags for benchmarks
    local build_tags="${BUILD_TAGS:-}"
    local tags_flag=""
    if [ -n "$build_tags" ]; then
        tags_flag="-tags $build_tags"
    fi
    
    if go test $bench_flags $tags_flag ./...; then
        log_success "Benchmarks completed"
    else
        log_error "Benchmark execution failed"
        return 1
    fi
}

# Function to run linters
run_lint() {
    log_info "Running linters..."
    
    # Check if golangci-lint is available
    if command -v golangci-lint &> /dev/null; then
        cd "$PROJECT_DIR"
        if golangci-lint run; then
            log_success "Linting passed"
        else
            log_error "Linting failed"
            return 1
        fi
    else
        log_warning "golangci-lint not found, running basic checks..."
        
        # Run basic checks
        if go vet ./... && go fmt ./...; then
            log_success "Basic checks passed"
        else
            log_error "Basic checks failed"
            return 1
        fi
    fi
}

# Function to clean build artifacts
clean() {
    log_info "Cleaning build artifacts..."
    
    rm -rf "$BUILD_DIR" "$DIST_DIR"
    
    # Clean test cache
    go clean -testcache
    
    log_success "Build artifacts cleaned"
}

# Function to create distribution packages
create_dist() {
    log_info "Creating distribution packages..."
    
    mkdir -p "$DIST_DIR"
    
    # Get version for package naming
    local version commit date
    read -r version commit date <<< "$(get_version)"
    
    # Platforms to build for
    local platforms=(
        "linux amd64"
        "linux arm64"
        "darwin amd64"
        "darwin arm64"
        "windows amd64"
    )
    
    for platform in "${platforms[@]}"; do
        local os arch
        read -r os arch <<< "$platform"
        
        log_info "Building distribution for $os/$arch..."
        
        # Build binaries for this platform
        if build_all "$os" "$arch"; then
            # Create package
            local package_name="noisefs-${version}-${os}-${arch}"
            local package_dir="${DIST_DIR}/${package_name}"
            
            mkdir -p "$package_dir"
            
            # Copy binaries
            cp -r "${BUILD_DIR}/${os}-${arch}/"* "$package_dir/" 2>/dev/null || \
            cp -r "${BUILD_DIR}/"* "$package_dir/" 2>/dev/null
            
            # Copy additional files
            cp "${PROJECT_DIR}/README.md" "$package_dir/"
            cp "${PROJECT_DIR}/CLAUDE.md" "$package_dir/"
            cp -r "${PROJECT_DIR}/configs" "$package_dir/"
            
            # Create archive
            cd "$DIST_DIR"
            if [ "$os" = "windows" ]; then
                zip -r "${package_name}.zip" "$package_name"
            else
                tar czf "${package_name}.tar.gz" "$package_name"
            fi
            rm -rf "$package_name"
            
            log_success "Created package for $os/$arch"
        else
            log_error "Failed to build for $os/$arch"
        fi
    done
    
    log_success "Distribution packages created in $DIST_DIR"
}

# Function to install binaries
install_binaries() {
    local install_dir="${1:-/usr/local/bin}"
    
    log_info "Installing binaries to $install_dir..."
    
    if [ ! -d "$BUILD_DIR" ]; then
        log_error "No binaries found. Run 'build' first."
        return 1
    fi
    
    # Check if we have permission to write to install directory
    if [ ! -w "$install_dir" ]; then
        log_error "No write permission to $install_dir. Try running with sudo."
        return 1
    fi
    
    # Install binaries
    for binary in "$BUILD_DIR"/*; do
        if [ -f "$binary" ] && [ -x "$binary" ]; then
            local basename
            basename=$(basename "$binary")
            cp "$binary" "$install_dir/"
            log_success "Installed $basename"
        fi
    done
    
    log_success "Installation completed"
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --target)
                export TARGET_OS="$2"
                shift 2
                ;;
            --arch)
                export TARGET_ARCH="$2"
                shift 2
                ;;
            --race)
                export ENABLE_RACE="true"
                shift
                ;;
            --tags)
                export BUILD_TAGS="$2"
                shift 2
                ;;
            --ldflags)
                export CUSTOM_LDFLAGS="$2"
                shift 2
                ;;
            --output)
                BUILD_DIR="$2"
                shift 2
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

# Main script logic
main() {
    local command="${1:-build}"
    shift || true
    
    # Parse remaining arguments
    parse_args "$@"
    
    # Check Go installation
    check_go
    
    case "$command" in
        build)
            build_all "${TARGET_OS:-}" "${TARGET_ARCH:-}"
            ;;
        clean)
            clean
            ;;
        test)
            run_tests
            ;;
        bench)
            run_benchmarks
            ;;
        lint)
            run_lint
            ;;
        dist)
            create_dist
            ;;
        install)
            install_binaries "${TARGET_OS:-}"
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"