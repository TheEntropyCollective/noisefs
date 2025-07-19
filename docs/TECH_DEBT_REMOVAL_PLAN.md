# NoiseFS Technical Debt Removal Plan

## Executive Summary

NoiseFS has accumulated significant technical debt that threatens the project's stability and development velocity. This plan outlines a systematic approach to eliminate tech debt across build systems, code duplication, incomplete features, and test infrastructure.

## Critical Issues Summary

- **Build Breaking**: Broken imports prevent compilation
- **Feature Incompleteness**: Major features are printf stubs
- **Code Duplication**: 15+ mock storage manager implementations
- **Test Infrastructure**: Heavy external dependencies, extensive skipping
- **Documentation Drift**: Claims of completed features that don't exist

## Phase 1: Emergency Stabilization (Week 1)

### 🔥 Critical Fixes - Must Complete Immediately

#### 1.1 Fix Build-Breaking Issues
```bash
# Target: go build ./... must succeed
```

**Actions**:
- [ ] Remove or implement missing `/pkg/storage/directory_processor` import
- [ ] Fix all broken import statements
- [ ] Ensure `go mod tidy` completes successfully
- [ ] Verify all packages compile

**Files to Fix**:
- `/Users/jconnuck/noisefs/pkg/fuse/directory_e2e_test.go:17`
- Any other broken imports found during analysis

#### 1.2 Document Reality vs Claims
```bash
# Target: Honest project status documentation
```

**Actions**:
- [ ] Update `/docs/archive/todo.md` to reflect actual implementation status
- [ ] Mark incomplete features clearly in documentation
- [ ] Remove claims of "COMPLETED" status for unimplemented features
- [ ] Create accurate roadmap of what actually works

#### 1.3 Stabilize Test Suite
```bash
# Target: go test ./... runs without crashes
```

**Actions**:
- [ ] Fix all test compilation errors
- [ ] Review and justify all `t.Skip()` calls
- [ ] Ensure basic unit tests can run without external dependencies

## Phase 2: Code Consolidation (Week 2)

### 🧹 Eliminate Duplication

#### 2.1 Consolidate Mock Infrastructure
**Problem**: 15+ different mock storage manager implementations

**Solution**: Create centralized mock infrastructure

```bash
# Target files to create/consolidate:
/Users/jconnuck/noisefs/pkg/testing/
├── mocks/
│   ├── storage_manager.go    # Single source of truth
│   ├── cache.go             # Centralized cache mocks  
│   └── backends.go          # Backend mocks
└── helpers/
    ├── setup.go             # Standard test setup
    └── data.go              # Test data generation
```

**Actions**:
- [ ] Audit all mock implementations and consolidate into `/pkg/testing/mocks/`
- [ ] Create standard test setup helpers in `/pkg/testing/helpers/`
- [ ] Update all tests to use centralized mocks
- [ ] Remove duplicate mock implementations

**Files to consolidate** (examples):
- `/Users/jconnuck/noisefs/pkg/storage/testing/mock_storage.go`
- `/Users/jconnuck/noisefs/pkg/storage/testing/test_helpers.go`
- `/Users/jconnuck/noisefs/pkg/infrastructure/workers/simple_pool_test.go`

#### 2.2 Standardize Test Helpers
**Problem**: 32 files with duplicate test creation functions

**Actions**:
- [ ] Create standard naming convention: `NewTestX()` for all test helpers
- [ ] Consolidate similar functionality into shared utilities
- [ ] Remove redundant helper functions
- [ ] Document standard test patterns in `/docs/TESTING.md`

## Phase 3: Feature Completion (Weeks 3-4)

### ⚡ Complete Half-Implemented Features

#### 3.1 Sync Engine Implementation
**Problem**: All sync operations are printf stubs

**File**: `/Users/jconnuck/noisefs/pkg/sync/sync_engine.go`

**Current State**:
```go
// TODO: Implement actual upload logic using DirectoryManager
fmt.Printf("Uploading: %s -> %s\n", op.LocalPath, op.RemotePath)
```

**Actions**:
- [ ] Implement actual upload logic using DirectoryManager
- [ ] Implement download logic with proper file handling
- [ ] Implement deletion with safety checks
- [ ] Add comprehensive error handling
- [ ] Write tests for all operations

#### 3.2 CLI Command Implementation  
**Problem**: Five major CLI commands print "implementation pending"

**File**: `/Users/jconnuck/noisefs/cmd/noisefs/commands.go`

**Actions**:
- [ ] Implement `announce` command with proper IPFS integration
- [ ] Implement `subscribe` command with event handling
- [ ] Implement `search` command with indexing support
- [ ] Implement `discover` command with peer discovery
- [ ] Add help text and validation for all commands

#### 3.3 Streaming Functionality
**Problem**: Advertised streaming returns "not yet implemented"

**File**: `/Users/jconnuck/noisefs/pkg/core/client/client.go`

**Actions**:
- [ ] Implement streaming upload with progress reporting
- [ ] Implement streaming download with resume capability
- [ ] Add streaming tests with large file scenarios
- [ ] Document streaming API usage

## Phase 4: Test Infrastructure Overhaul (Week 5)

### 🧪 Reduce External Dependencies

#### 4.1 Mock External Systems
**Problem**: Tests require IPFS daemon, Docker, FUSE

**Actions**:
- [ ] Create mock IPFS backend for unit tests
- [ ] Implement in-memory storage backend for testing
- [ ] Create mock FUSE interface for filesystem tests
- [ ] Separate integration tests from unit tests

#### 4.2 Enable Skipped Tests
**Problem**: Extensive use of `t.Skip()` throughout test suite

**Strategy**:
```bash
# Phase 4.2.1: Categorize skipped tests
grep -r "t.Skip" --include="*.go" . > skipped_tests_audit.txt

# Phase 4.2.2: Enable tests by category
# - Unit tests: Enable with mocks
# - Integration tests: Enable with docker-compose
# - System tests: Keep skipped but document requirements
```

**Actions**:
- [ ] Audit all skipped tests and categorize by reason
- [ ] Enable unit tests with proper mocking
- [ ] Create docker-compose setup for integration tests
- [ ] Document requirements for system tests
- [ ] Set up CI/CD to run appropriate test categories

## Phase 5: Consistency and Standards (Week 6)

### 📏 Standardize Patterns

#### 5.1 Error Handling Standardization
**Problem**: Mixed error handling approaches

**Standard Pattern**:
```go
// Good: Contextual error wrapping
if err != nil {
    return fmt.Errorf("failed to create storage manager: %w", err)
}

// Bad: Generic error return
if err != nil {
    return err
}
```

**Actions**:
- [ ] Define error handling standards in `/docs/CODING_STANDARDS.md`
- [ ] Update all packages to use consistent error wrapping
- [ ] Create error handling linting rules
- [ ] Add error handling examples to documentation

#### 5.2 Naming Convention Enforcement
**Problem**: Inconsistent naming across packages

**Actions**:
- [ ] Document naming conventions for test helpers, mocks, utilities
- [ ] Rename inconsistent functions to follow standards
- [ ] Create linting rules to enforce conventions
- [ ] Update code review checklist

## Phase 6: Documentation Alignment (Week 7)

### 📚 Sync Documentation with Reality

#### 6.1 API Documentation Cleanup
**Actions**:
- [ ] Review all public APIs and ensure they have documentation
- [ ] Update documentation to match actual implementation
- [ ] Remove documentation for non-existent features
- [ ] Add examples for working features

#### 6.2 User Guide Accuracy
**Actions**:
- [ ] Test all installation instructions
- [ ] Verify all code examples work
- [ ] Update performance claims with actual benchmark results
- [ ] Create troubleshooting guide for real issues

## Success Metrics

### Phase 1 (Emergency) - Success Criteria:
- [ ] `go build ./...` succeeds
- [ ] `go test ./...` runs without compilation errors
- [ ] Documentation accurately reflects implementation status

### Phase 2 (Consolidation) - Success Criteria:
- [ ] Single mock storage manager implementation
- [ ] <5 test helper functions (down from 32)
- [ ] All duplicate code identified and removed

### Phase 3 (Completion) - Success Criteria:
- [ ] No TODOs in core business logic
- [ ] All advertised features actually work
- [ ] CLI commands perform real operations

### Phase 4 (Testing) - Success Criteria:
- [ ] Unit tests run without external dependencies
- [ ] <10% of tests skipped (down from current high percentage)
- [ ] Integration tests automated with docker-compose

### Phases 5-6 (Standards) - Success Criteria:
- [ ] Consistent error handling patterns
- [ ] Standardized naming conventions
- [ ] Documentation matches implementation

## Risk Mitigation

### High-Risk Areas:
1. **Build Breaking Changes**: All changes must maintain compilation
2. **Feature Regressions**: Existing working features must continue to work
3. **Test Coverage Loss**: Removing duplicate tests must not reduce actual coverage

### Mitigation Strategies:
1. **Incremental Changes**: Small, verifiable changes with immediate testing
2. **Feature Flags**: Use feature flags for major changes during transition
3. **Rollback Plan**: Git tags at each phase for quick rollback if needed

## Resource Requirements

### Time Estimate: 7 weeks (assuming 1 developer full-time)
- Week 1: Emergency fixes (critical)
- Week 2: Code consolidation 
- Weeks 3-4: Feature completion
- Week 5: Test infrastructure
- Week 6: Standards enforcement
- Week 7: Documentation alignment

### Prerequisites:
- Access to build and test environments
- Docker setup for integration testing
- IPFS node for integration testing

## Monitoring and Validation

### Weekly Check-ins:
1. Build status verification
2. Test suite health metrics
3. Code duplication analysis
4. Documentation accuracy review

### Automated Validation:
```bash
# Build health check
go build ./... && echo "✅ Build OK" || echo "❌ Build FAIL"

# Test compilation check  
go test -compile-only ./... && echo "✅ Tests compile" || echo "❌ Tests broken"

# Duplication analysis
find . -name "*.go" -exec grep -l "MockStorage" {} \; | wc -l
```

This systematic approach will transform NoiseFS from a debt-laden codebase into a maintainable, professional project ready for continued development.