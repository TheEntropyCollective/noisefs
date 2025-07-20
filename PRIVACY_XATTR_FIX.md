# NoiseFS Extended Attributes Privacy Fix

## Problem

The original NoiseFS FUSE implementation exposed sensitive metadata through extended attributes that compromised the system's core anonymity guarantees:

### Sensitive Attributes Exposed:
- `user.noisefs.descriptor_cid` - Content identifiers that break anonymity
- `user.noisefs.created_at` - Timestamps enabling timing correlation attacks  
- `user.noisefs.modified_at` - Additional timing metadata for correlation
- `user.noisefs.file_size` - Size information aiding fingerprinting attacks
- `user.noisefs.directory` - Path information breaking privacy

### Privacy Impact:
1. **Content Identification**: Descriptor CIDs could be used to identify and track specific files across the network
2. **Timing Correlation**: Timestamps could enable correlation attacks linking user activities
3. **Fingerprinting**: File sizes and paths could aid in content fingerprinting
4. **Path Disclosure**: Directory information exposed internal file organization

## Solution

Implemented privacy-preserving extended attributes that maintain functionality while protecting anonymity:

### Changes Made:

#### 1. GetXAttr() Function (lines 574-606)
**Before:**
```go
switch attribute {
case "user.noisefs.descriptor_cid":
    return []byte(entry.DescriptorCID), fuse.OK
case "user.noisefs.created_at":
    return []byte(entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00")), fuse.OK
// ... other sensitive attributes
}
```

**After:**
```go
switch attribute {
case "user.noisefs.type":
    return []byte("noisefs-file"), fuse.OK
case "user.noisefs.version":
    return []byte("1.0"), fuse.OK
case "user.noisefs.encrypted":
    return []byte("true"), fuse.OK
default:
    // All sensitive attributes blocked
    return nil, fuse.ENODATA
}
```

#### 2. ListXAttr() Function (lines 608-629)
**Before:**
```go
attrs := []string{
    "user.noisefs.descriptor_cid",
    "user.noisefs.created_at",
    "user.noisefs.modified_at", 
    "user.noisefs.file_size",
    "user.noisefs.directory",
}
```

**After:**
```go
attrs := []string{
    "user.noisefs.type",
    "user.noisefs.version", 
    "user.noisefs.encrypted",
}
```

#### 3. Enhanced Comments
Added explicit privacy documentation and reasoning for the changes.

### Privacy-Safe Attributes:

| Attribute | Value | Privacy Impact |
|-----------|--------|----------------|
| `user.noisefs.type` | `"noisefs-file"` | ✅ Safe - Only indicates file is managed by NoiseFS |
| `user.noisefs.version` | `"1.0"` | ✅ Safe - NoiseFS version information |
| `user.noisefs.encrypted` | `"true"` | ✅ Safe - Expected in privacy-focused system |

### Removed Attributes:

| Attribute | Risk | Reason for Removal |
|-----------|------|-------------------|
| `descriptor_cid` | 🔴 Critical | Enables content identification and tracking |
| `created_at` | 🔴 High | Enables timing correlation attacks |
| `modified_at` | 🔴 High | Additional timing correlation vector |
| `file_size` | 🟡 Medium | May aid in content fingerprinting |
| `directory` | 🔴 High | Exposes internal file organization |

## Implementation Details

### Files Modified:
- `/pkg/fuse/mount.go` - Core privacy fix
- `/pkg/fuse/privacy_xattr_test.go` - Test coverage
- `/pkg/fuse/privacy_verification.go` - Privacy compliance checking

### Testing:
```bash
go test -v -tags fuse ./pkg/fuse -run TestExtendedAttributesPrivacy
```

### Verification:
The `VerifyPrivacyCompliance()` function can be used to audit NoiseFS instances for privacy violations.

## Impact Assessment

### ✅ Benefits:
- **Preserved Anonymity**: Content identifiers no longer exposed
- **Prevented Correlation**: Timing metadata removed
- **Reduced Fingerprinting**: Size and path information blocked  
- **Maintained Functionality**: Basic file operations still work
- **User Transparency**: Safe metadata still available

### ⚠️ Considerations:
- **Reduced Debugging**: Less metadata available for troubleshooting
- **Application Compatibility**: Apps expecting specific attributes may need updates
- **User Expectation**: Users accustomed to rich metadata may notice change

## Alternative Approaches Considered

1. **Encryption**: Encrypt sensitive attributes before exposure
   - **Rejected**: Still provides attack vectors through encrypted data patterns
   
2. **Hashing**: Hash sensitive data before exposure  
   - **Rejected**: Hash collisions could still enable correlation
   
3. **Access Control**: Restrict based on user/process
   - **Rejected**: Too complex and error-prone for privacy-critical system

4. **Complete Removal**: Remove all extended attributes
   - **Rejected**: Breaks compatibility with standard filesystem expectations

## Conclusion

The implemented solution strikes the optimal balance between:
- **Privacy Protection**: Eliminates all known metadata leakage vectors
- **Functional Compatibility**: Maintains basic extended attribute interface
- **System Integrity**: Preserves NoiseFS core anonymity guarantees
- **User Experience**: Provides expected filesystem behavior

This fix ensures NoiseFS maintains its privacy-first design principles while remaining compatible with standard FUSE filesystem expectations.