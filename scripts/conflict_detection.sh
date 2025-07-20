#!/bin/bash

# Tech Debt Cleanup - Automated Conflict Detection
# Monitors file modifications across agent branches to prevent conflicts

set -e

COORDINATION_BRANCH="feature/tech-debt-cleanup-week1"
AGENT_BRANCHES=("feature/agent-1-search" "feature/agent-2-config" "feature/agent-3-performance" "feature/agent-4-docs")

echo "=== Tech Debt Cleanup - Conflict Detection ==="
echo "Coordination Branch: $COORDINATION_BRANCH"
echo "Agent Branches: ${AGENT_BRANCHES[*]}"
echo ""

# Function to get modified files in a branch
get_modified_files() {
    local branch=$1
    git diff --name-only $COORDINATION_BRANCH..$branch 2>/dev/null || echo ""
}

# Function to check for file conflicts between branches
check_conflicts() {
    echo "Checking for file conflicts between agent branches..."
    
    declare -A file_branches
    local conflicts_found=false
    
    for branch in "${AGENT_BRANCHES[@]}"; do
        # Check if branch exists
        if ! git rev-parse --verify $branch >/dev/null 2>&1; then
            echo "⚠️  Branch $branch does not exist yet"
            continue
        fi
        
        modified_files=$(get_modified_files $branch)
        if [ -z "$modified_files" ]; then
            echo "✅ $branch: No modifications yet"
            continue
        fi
        
        echo "📁 $branch modified files:"
        while IFS= read -r file; do
            if [ -n "$file" ]; then
                echo "   - $file"
                
                # Check if this file is modified in another branch
                if [ -n "${file_branches[$file]}" ]; then
                    echo "🔴 CONFLICT DETECTED: $file modified in both $branch and ${file_branches[$file]}"
                    conflicts_found=true
                else
                    file_branches[$file]=$branch
                fi
            fi
        done <<< "$modified_files"
        echo ""
    done
    
    if [ "$conflicts_found" = true ]; then
        echo "❌ CONFLICTS DETECTED - Coordination required!"
        echo "Update TECH_DEBT_CLEANUP_PROGRESS.md with coordination requests"
        return 1
    else
        echo "✅ No conflicts detected between agent branches"
        return 0
    fi
}

# Function to verify critical tests exist
check_critical_tests() {
    echo "Verifying critical tests implementation..."
    
    local tests_missing=false
    
    # Agent 1 - Search system tests
    if [ ! -f "tests/integration/search_removal_test.go" ] && [ ! -f "pkg/core/search/removal_test.go" ]; then
        echo "🔴 Agent 1: Missing search system removal tests"
        tests_missing=true
    fi
    
    # Agent 2 - Config system tests  
    if [ ! -f "tests/integration/config_migration_test.go" ] && [ ! -f "pkg/common/config/migration_test.go" ]; then
        echo "🔴 Agent 2: Missing config system migration tests"
        tests_missing=true
    fi
    
    # Agent 3 - Performance analyzer tests
    if [ ! -f "tests/integration/performance_removal_test.go" ]; then
        echo "🔴 Agent 3: Missing performance analyzer removal tests"  
        tests_missing=true
    fi
    
    # Agent 4 - Documentation tests
    if [ ! -f "tests/integration/documentation_test.go" ]; then
        echo "🔴 Agent 4: Missing documentation verification tests"
        tests_missing=true
    fi
    
    if [ "$tests_missing" = true ]; then
        echo "❌ CRITICAL TESTS MISSING - Work cannot proceed"
        echo "Agents must implement blocking tests before beginning primary work"
        return 1
    else
        echo "✅ All critical tests verified"
        return 0
    fi
}

# Function to run integration tests
run_integration_tests() {
    echo "Running integration test suite..."
    
    # Save current branch
    current_branch=$(git branch --show-current)
    
    # Test each agent branch
    for branch in "${AGENT_BRANCHES[@]}"; do
        if git rev-parse --verify $branch >/dev/null 2>&1; then
            echo "Testing $branch..."
            git checkout $branch >/dev/null 2>&1
            
            if ! go test ./tests/integration/... -v -timeout=60s; then
                echo "❌ Integration tests failed on $branch"
                git checkout $current_branch >/dev/null 2>&1
                return 1
            fi
        fi
    done
    
    # Return to original branch
    git checkout $current_branch >/dev/null 2>&1
    echo "✅ All integration tests passed"
    return 0
}

# Function to update progress tracking
update_progress() {
    local status=$1
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    echo "## Automated Conflict Detection - $timestamp" >> TECH_DEBT_CLEANUP_PROGRESS.md
    echo "" >> TECH_DEBT_CLEANUP_PROGRESS.md
    
    if [ "$status" = "success" ]; then
        echo "✅ **Status**: No conflicts detected, all systems operational" >> TECH_DEBT_CLEANUP_PROGRESS.md
    else
        echo "🔴 **Status**: Issues detected - see details above" >> TECH_DEBT_CLEANUP_PROGRESS.md
    fi
    
    echo "" >> TECH_DEBT_CLEANUP_PROGRESS.md
    echo "---" >> TECH_DEBT_CLEANUP_PROGRESS.md
    echo "" >> TECH_DEBT_CLEANUP_PROGRESS.md
}

# Main execution
main() {
    echo "Starting conflict detection at $(date)"
    echo ""
    
    # Check for conflicts between branches
    if ! check_conflicts; then
        update_progress "conflict"
        exit 1
    fi
    
    # Check critical tests
    if ! check_critical_tests; then
        update_progress "missing_tests"
        exit 1
    fi
    
    # Run integration tests if requested
    if [ "$1" = "--run-tests" ]; then
        if ! run_integration_tests; then
            update_progress "test_failure"
            exit 1
        fi
    fi
    
    update_progress "success"
    echo "✅ Conflict detection completed successfully"
}

# Help message
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Usage: $0 [--run-tests]"
    echo ""
    echo "Options:"
    echo "  --run-tests    Also run integration tests on all branches"
    echo "  --help, -h     Show this help message"
    echo ""
    echo "This script monitors for conflicts between agent branches and verifies"
    echo "that critical tests are implemented before allowing work to proceed."
    exit 0
fi

main "$@"