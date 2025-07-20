# Phase 1: Critical Security Fixes - Detailed Execution Plan

**Timeline**: 1 Week  
**Priority**: CRITICAL - Must complete before any production deployment  
**Parallelization**: 4 independent work streams

## Overview

Phase 1 addresses the most critical security vulnerabilities identified in the code review. Each task is designed to be independent and can be executed by separate agents simultaneously.

## Task Breakdown

### Task 1: Path Traversal Protection 🔴
**Agent**: Security Agent A  
**Estimated Time**: 2 days  
**Risk**: HIGH - Directory traversal attacks

#### Scope
- **Files to modify**: 
  - `pkg/sync/sync_engine.go` (primary)
  - `pkg/sync/state_store.go` (secondary)
  - `cmd/noisefs/sync.go` (validation)

#### Specific Changes Required

1. **Create path validation utility** (`pkg/security/path_validator.go`):
```go
package security

import (
    "fmt"
    "path/filepath"
    "strings"
)

// ValidatePathInBounds ensures path is within allowed directory
func ValidatePathInBounds(path, allowedRoot string) error {
    cleanPath := filepath.Clean(path)
    cleanRoot := filepath.Clean(allowedRoot)
    
    // Convert to absolute paths
    absPath, err := filepath.Abs(cleanPath)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }
    
    absRoot, err := filepath.Abs(cleanRoot)
    if err != nil {
        return fmt.Errorf("invalid root: %w", err)
    }
    
    // Check if path is within root
    if !strings.HasPrefix(absPath, absRoot) {
        return fmt.Errorf("path outside allowed directory: %s", path)
    }
    
    return nil
}
```

2. **Update sync_engine.go operations**:
   - `executeUpload()` line 632: Add path validation
   - `executeDownload()` line 655: Add path validation  
   - `executeDelete()` line 680: Add path validation
   - `executeCreateDir()` line 704: Add path validation
   - `executeDeleteDir()` line 726: Add path validation

3. **Add validation to state store**:
   - `getStateFile()` line 232: Validate sync ID to prevent path traversal

#### Success Criteria
- [ ] All file operations validate paths before execution
- [ ] Path traversal attempts fail with security error
- [ ] Existing functionality remains intact
- [ ] Unit tests pass with malicious path inputs

#### Test Cases to Add
```go
func TestPathTraversalPrevention(t *testing.T) {
    // Test cases for malicious paths
    maliciousPaths := []string{
        "../../../etc/passwd",
        "/etc/passwd", 
        "..\\..\\windows\\system32",
        "valid/path/../../../etc/passwd",
    }
    // Verify all are rejected
}
```

---

### Task 2: Authentication System Implementation 🔴
**Agent**: Security Agent B  
**Estimated Time**: 3 days  
**Risk**: HIGH - Unauthorized access

#### Scope
- **Files to create**:
  - `pkg/auth/session.go`
  - `pkg/auth/middleware.go` 
  - `pkg/auth/types.go`
- **Files to modify**:
  - `pkg/sync/sync_engine.go`
  - `cmd/noisefs/sync.go`

#### Specific Changes Required

1. **Create authentication framework** (`pkg/auth/`):
```go
// pkg/auth/types.go
type Session struct {
    ID        string
    UserID    string
    CreatedAt time.Time
    ExpiresAt time.Time
    Permissions []Permission
}

type Permission string
const (
    PermissionSyncRead  Permission = "sync:read"
    PermissionSyncWrite Permission = "sync:write"
    PermissionSyncAdmin Permission = "sync:admin"
)

// pkg/auth/session.go
type SessionManager struct {
    sessions map[string]*Session
    mu       sync.RWMutex
}

func (sm *SessionManager) CreateSession(userID string, permissions []Permission) (*Session, error)
func (sm *SessionManager) ValidateSession(sessionID string) (*Session, error)
func (sm *SessionManager) HasPermission(sessionID string, permission Permission) bool
```

2. **Add auth middleware to sync operations**:
```go
// pkg/auth/middleware.go
func RequireAuth(permission Permission) func(next SyncHandler) SyncHandler {
    return func(next SyncHandler) SyncHandler {
        return func(ctx context.Context, req SyncRequest) error {
            session := ctx.Value("session").(*Session)
            if session == nil {
                return ErrUnauthorized
            }
            
            if !hasPermission(session, permission) {
                return ErrForbidden
            }
            
            return next(ctx, req)
        }
    }
}
```

3. **Update sync engine with auth checks**:
   - Add session validation to `StartSync()`
   - Add permission checks to sync operations
   - Integrate with existing sync session management

4. **Update CLI to handle authentication**:
   - Add auth flags to sync commands
   - Implement session token handling
   - Add login/logout commands

#### Success Criteria
- [ ] All sync operations require valid session
- [ ] Permission system prevents unauthorized actions
- [ ] Session expiration handled gracefully
- [ ] CLI supports authentication workflow

---

### Task 3: Error Message Sanitization 🟡
**Agent**: Security Agent C  
**Estimated Time**: 1.5 days  
**Risk**: MEDIUM - Information disclosure

#### Scope
- **Files to modify**:
  - `pkg/sync/sync_engine.go`
  - `pkg/sync/state_store.go`
  - `cmd/noisefs/commands.go`
  - `pkg/util/errors.go`

#### Specific Changes Required

1. **Create error sanitization utility** (`pkg/security/error_sanitizer.go`):
```go
package security

import (
    "fmt"
    "path/filepath"
    "regexp"
    "strings"
)

var sensitivePatterns = []*regexp.Regexp{
    regexp.MustCompile(`/[^/\s]+/[^/\s]+/[^/\s]+`), // Unix paths
    regexp.MustCompile(`[A-Z]:\\[^\\s]+\\[^\\s]+`),  // Windows paths
    regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), // IP addresses
}

func SanitizeError(err error, publicPath string) error {
    if err == nil {
        return nil
    }
    
    message := err.Error()
    
    // Replace sensitive paths with generic indicators
    for _, pattern := range sensitivePatterns {
        message = pattern.ReplaceAllString(message, "[PATH]")
    }
    
    // If public path provided, allow it through
    if publicPath != "" {
        message = strings.ReplaceAll(message, "[PATH]", publicPath)
    }
    
    return fmt.Errorf("operation failed: %s", message)
}
```

2. **Update sync engine error handling**:
   - Sanitize all fmt.Printf error messages
   - Use SanitizeError for user-facing errors
   - Keep detailed errors for internal logging

3. **Update CLI error display**:
   - Sanitize before showing to user
   - Add verbose flag for debugging

#### Success Criteria
- [ ] No internal paths exposed in user errors
- [ ] Debugging information available with verbose flag
- [ ] Error messages remain helpful for users
- [ ] Logging preserves full error details

---

### Task 4: Secure Key Derivation 🟡
**Agent**: Security Agent D  
**Estimated Time**: 1 day  
**Risk**: MEDIUM - Weak cryptographic security

#### Scope
- **Files to modify**:
  - `cmd/noisefs/sync.go` (line 429)
  - `pkg/core/crypto/` (enhance key generation)

#### Specific Changes Required

1. **Enhance crypto package** (`pkg/core/crypto/keys.go`):
```go
func GenerateSecureSyncKey(sessionID string, userSalt []byte) ([]byte, error) {
    // Use PBKDF2 with proper parameters
    salt := append(userSalt, []byte(sessionID)...)
    
    // Generate random entropy
    entropy := make([]byte, 32)
    if _, err := rand.Read(entropy); err != nil {
        return nil, fmt.Errorf("failed to generate entropy: %w", err)
    }
    
    // Derive key using PBKDF2
    key := pbkdf2.Key(entropy, salt, 100000, 32, sha256.New)
    return key, nil
}
```

2. **Update sync.go key generation**:
   - Replace hardcoded "sync-key" parameter
   - Use secure random generation
   - Store key derivation parameters securely

3. **Add key rotation support**:
   - Periodic key regeneration
   - Backward compatibility for existing syncs

#### Success Criteria
- [ ] No hardcoded key parameters
- [ ] Cryptographically secure key generation
- [ ] Proper entropy and salt usage
- [ ] Key rotation mechanism in place

## Integration Plan

### Day 1-2: Foundation
- Task 1 (Path validation) - Agent A
- Task 4 (Key derivation) - Agent D  

### Day 3-4: Core Security  
- Task 2 (Authentication) - Agent B
- Task 3 (Error sanitization) - Agent C

### Day 5: Integration & Testing
- Integrate all security fixes
- Run comprehensive security tests
- Verify no functionality regression

## Testing Strategy

### Security Tests Required
1. **Path Traversal Tests**:
   - Malicious path inputs
   - Edge cases (symlinks, relative paths)
   - Cross-platform path handling

2. **Authentication Tests**:
   - Unauthorized access attempts
   - Session expiration handling
   - Permission boundary testing

3. **Information Disclosure Tests**:
   - Error message content analysis
   - Log sanitization verification
   - Debug information leakage

4. **Cryptographic Tests**:
   - Key randomness testing  
   - Entropy verification
   - Key derivation consistency

### Regression Tests
- All existing sync functionality must pass
- Performance benchmarks must maintain baseline
- Integration tests with IPFS backend

## Success Metrics

### Security Metrics
- [ ] Zero path traversal vulnerabilities (verified by automated testing)
- [ ] All sync operations require authentication
- [ ] No sensitive information in user-facing errors
- [ ] Cryptographic keys pass randomness tests

### Quality Metrics  
- [ ] All existing tests pass
- [ ] New security tests achieve 100% coverage
- [ ] No performance degradation >5%
- [ ] Code review approval from security expert

## Risk Mitigation

### Potential Issues
1. **Breaking Changes**: New authentication may break existing workflows
   - **Mitigation**: Backward compatibility mode with deprecation warnings

2. **Performance Impact**: Security checks may slow operations
   - **Mitigation**: Optimize hot paths, benchmark critical operations

3. **Integration Complexity**: Auth system may conflict with existing code
   - **Mitigation**: Gradual rollout, feature flags for new security

### Rollback Plan
- Maintain feature flags for each security enhancement
- Automated rollback triggers if tests fail
- Manual override for emergency situations

## Deliverables

### Code Deliverables
- [ ] Security utilities package (`pkg/security/`)
- [ ] Authentication framework (`pkg/auth/`)
- [ ] Updated sync operations with security checks
- [ ] Comprehensive security test suite

### Documentation Deliverables
- [ ] Security implementation guide
- [ ] Authentication usage documentation  
- [ ] Security best practices for developers
- [ ] Incident response procedures

---

**Ready for Review**: This plan is ready for your approval before deploying parallel agents to execute Phase 1.