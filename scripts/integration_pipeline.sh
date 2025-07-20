#!/bin/bash

# Tech Debt Cleanup - Integration Test Pipeline
# Automated testing pipeline for coordination and continuous integration

set -e

COORDINATION_BRANCH="feature/tech-debt-cleanup-week1"
AGENT_BRANCHES=("feature/agent-1-search" "feature/agent-2-config" "feature/agent-3-performance" "feature/agent-4-docs")

echo "=== Tech Debt Cleanup - Integration Pipeline ==="
echo "Starting integration test pipeline at $(date)"
echo ""

# Function to run tests on a specific branch
run_branch_tests() {
    local branch=$1
    local current_branch=$(git branch --show-current)
    
    echo "🧪 Testing branch: $branch"
    
    # Switch to branch
    if ! git checkout $branch >/dev/null 2>&1; then
        echo "❌ Failed to checkout $branch"
        return 1
    fi
    
    # Run build test
    echo "   - Build test..."
    if ! go build ./... >/dev/null 2>&1; then
        echo "❌ Build failed on $branch"
        git checkout $current_branch >/dev/null 2>&1
        return 1
    fi
    
    # Run unit tests
    echo "   - Unit tests..."
    if ! go test ./... -short -timeout=30s >/dev/null 2>&1; then
        echo "❌ Unit tests failed on $branch"
        git checkout $current_branch >/dev/null 2>&1
        return 1
    fi
    
    # Run integration tests
    echo "   - Integration tests..."
    if ! go test ./tests/integration/... -timeout=60s >/dev/null 2>&1; then
        echo "❌ Integration tests failed on $branch"
        git checkout $current_branch >/dev/null 2>&1
        return 1
    fi
    
    # Return to original branch
    git checkout $current_branch >/dev/null 2>&1
    echo "   ✅ All tests passed"
    return 0
}

# Function to test merge scenarios
test_merge_scenarios() {
    echo "🔄 Testing merge scenarios..."
    local current_branch=$(git branch --show-current)
    
    # Create temporary merge test branch
    local temp_branch="temp-merge-test-$(date +%s)"
    git checkout -b $temp_branch $COORDINATION_BRANCH >/dev/null 2>&1
    
    # Test merging each agent branch
    for branch in "${AGENT_BRANCHES[@]}"; do
        if git rev-parse --verify $branch >/dev/null 2>&1; then
            echo "   - Testing merge of $branch..."
            
            if ! git merge $branch --no-commit --no-ff >/dev/null 2>&1; then
                echo "❌ Merge conflict detected with $branch"
                git merge --abort >/dev/null 2>&1
                git checkout $current_branch >/dev/null 2>&1
                git branch -D $temp_branch >/dev/null 2>&1
                return 1
            fi
            
            # Test build after merge
            if ! go build ./... >/dev/null 2>&1; then
                echo "❌ Build failed after merging $branch"
                git reset --hard HEAD >/dev/null 2>&1
                git checkout $current_branch >/dev/null 2>&1
                git branch -D $temp_branch >/dev/null 2>&1
                return 1
            fi
            
            # Reset for next merge test
            git reset --hard HEAD >/dev/null 2>&1
        fi
    done
    
    # Cleanup
    git checkout $current_branch >/dev/null 2>&1
    git branch -D $temp_branch >/dev/null 2>&1
    echo "   ✅ All merge scenarios successful"
    return 0
}

# Function to run critical path tests
run_critical_path_tests() {
    echo "🔍 Running critical path tests..."
    
    # Test main application entry points
    echo "   - Testing main application..."
    if ! go test ./cmd/noisefs/... -timeout=30s >/dev/null 2>&1; then
        echo "❌ Main application tests failed"
        return 1
    fi
    
    # Test WebUI
    echo "   - Testing WebUI..."
    if ! go test ./cmd/noisefs-webui/... -timeout=30s >/dev/null 2>&1; then
        echo "❌ WebUI tests failed"
        return 1
    fi
    
    # Test core systems
    echo "   - Testing core systems..."
    if ! go test ./pkg/core/... -timeout=30s >/dev/null 2>&1; then
        echo "❌ Core system tests failed"
        return 1
    fi
    
    # Test storage systems
    echo "   - Testing storage systems..."
    if ! go test ./pkg/storage/... -timeout=30s >/dev/null 2>&1; then
        echo "❌ Storage system tests failed"
        return 1
    fi
    
    echo "   ✅ All critical path tests passed"
    return 0
}

# Function to generate test report
generate_test_report() {
    local status=$1
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local report_file="test_reports/integration_report_$(date +%Y%m%d_%H%M%S).md"
    
    mkdir -p test_reports
    
    cat > "$report_file" << EOF
# Integration Test Report

**Generated**: $timestamp
**Status**: $status
**Pipeline**: Tech Debt Cleanup Week 1

## Branch Status

| Branch | Build | Unit Tests | Integration Tests | Status |
|--------|-------|------------|-------------------|---------|
EOF

    for branch in "${AGENT_BRANCHES[@]}"; do
        if git rev-parse --verify $branch >/dev/null 2>&1; then
            # Test each component and record results
            local build_status="❌"
            local unit_status="❌" 
            local integration_status="❌"
            local overall_status="🔴 Failed"
            
            git checkout $branch >/dev/null 2>&1
            
            if go build ./... >/dev/null 2>&1; then
                build_status="✅"
                if go test ./... -short >/dev/null 2>&1; then
                    unit_status="✅"
                    if go test ./tests/integration/... >/dev/null 2>&1; then
                        integration_status="✅"
                        overall_status="🟢 Passed"
                    fi
                fi
            fi
            
            echo "| $branch | $build_status | $unit_status | $integration_status | $overall_status |" >> "$report_file"
        else
            echo "| $branch | N/A | N/A | N/A | 🟡 Not Created |" >> "$report_file"
        fi
    done
    
    git checkout $COORDINATION_BRANCH >/dev/null 2>&1
    
    cat >> "$report_file" << EOF

## Test Summary

- **Total Branches Tested**: ${#AGENT_BRANCHES[@]}
- **Pipeline Status**: $status
- **Critical Path Tests**: $([ "$status" = "✅ SUCCESS" ] && echo "✅ Passed" || echo "❌ Failed")

## Recommendations

EOF

    if [ "$status" = "✅ SUCCESS" ]; then
        echo "- All systems operational - agents can proceed with work" >> "$report_file"
        echo "- Continue regular integration testing" >> "$report_file"
        echo "- Monitor for merge conflicts as work progresses" >> "$report_file"
    else
        echo "- ⚠️ Issues detected - resolve before proceeding" >> "$report_file"
        echo "- Run conflict detection script for detailed analysis" >> "$report_file"
        echo "- Coordinate with affected agents" >> "$report_file"
    fi
    
    echo ""
    echo "📊 Test report generated: $report_file"
}

# Main pipeline execution
main() {
    local overall_status="✅ SUCCESS"
    
    # Run conflict detection first
    echo "🔍 Running conflict detection..."
    if ! ./scripts/conflict_detection.sh; then
        overall_status="❌ CONFLICTS DETECTED"
    fi
    
    # Test each agent branch
    for branch in "${AGENT_BRANCHES[@]}"; do
        if git rev-parse --verify $branch >/dev/null 2>&1; then
            if ! run_branch_tests $branch; then
                overall_status="❌ TESTS FAILED"
            fi
        else
            echo "⚠️ Branch $branch not yet created"
        fi
    done
    
    # Test merge scenarios
    if [ "$overall_status" = "✅ SUCCESS" ]; then
        if ! test_merge_scenarios; then
            overall_status="❌ MERGE CONFLICTS"
        fi
    fi
    
    # Run critical path tests
    if [ "$overall_status" = "✅ SUCCESS" ]; then
        if ! run_critical_path_tests; then
            overall_status="❌ CRITICAL PATH FAILED"
        fi
    fi
    
    # Generate report
    generate_test_report "$overall_status"
    
    echo ""
    echo "=== Pipeline Complete ==="
    echo "Status: $overall_status"
    
    if [ "$overall_status" != "✅ SUCCESS" ]; then
        exit 1
    fi
}

# Help message
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Usage: $0"
    echo ""
    echo "Runs comprehensive integration testing pipeline for tech debt cleanup."
    echo "Tests all agent branches, merge scenarios, and critical paths."
    echo ""
    echo "Generated reports are stored in test_reports/ directory."
    exit 0
fi

main "$@"