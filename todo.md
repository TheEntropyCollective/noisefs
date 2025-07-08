# NoiseFS Todo

## Current Sprint: Security Hardening

### Security Analysis Summary
Based on comprehensive code review, NoiseFS currently has the following security implementation status:

#### Already Implemented ✅
1. **Content Security**
   - Strong 3-tuple XOR anonymization (data ⊕ randomizer1 ⊕ randomizer2)
   - Cryptographically secure random block generation
   - Content-addressed storage via IPFS (integrity protection)
   - Block integrity verification via SHA256 hashes

2. **Privacy Features**
   - Plausible deniability through anonymized blocks
   - No direct storage of original content
   - Block reuse for storage efficiency

3. **Basic Input Validation**
   - Block size validation
   - CID validation
   - File path validation
   - Configuration validation

#### Critical Security Gaps ❌

1. **Authentication/Authorization**
   - NO authentication system implemented
   - NO access control for descriptors
   - NO user identity management
   - NO permission system for files

2. **Network Security**
   - NO TLS/HTTPS for web UI (plain HTTP on port 8080)
   - NO encrypted communication channels
   - NO certificate management
   - NO secure API endpoints
   - Relies entirely on IPFS for network transport

3. **Descriptor Security**
   - Descriptors stored in plaintext JSON
   - NO encryption of file metadata
   - File sizes and block counts exposed
   - NO access control mechanisms

4. **Key Management**
   - NO key generation or storage system
   - NO encryption keys for descriptors
   - NO secure key derivation

5. **Local Security**
   - Index files stored unencrypted at ~/.noisefs/index.json
   - Cache stored in plaintext memory
   - NO secure memory handling
   - NO protection against memory dumps

6. **Streaming/Media Security**
   - NO streaming protocol implementation
   - NO DRM or content protection
   - NO secure media delivery
   - NO chunked/progressive download

### Security Implementation Plan

#### Phase 1: Network Security (High Priority)
- [ ] Add HTTPS support to web UI
  - [ ] Generate self-signed certificates for development
  - [ ] Add TLS configuration options
  - [ ] Implement certificate management
  - [ ] Force HTTPS redirects
- [ ] Secure IPFS communication
  - [ ] Implement IPFS private networks
  - [ ] Add peer authentication
  - [ ] Configure secure bootstrapping

#### Phase 2: Authentication & Authorization
- [ ] Implement user authentication system
  - [ ] Add user registration/login
  - [ ] Implement JWT or session-based auth
  - [ ] Add password hashing (bcrypt/argon2)
  - [ ] Implement secure session management
- [ ] Add descriptor access control
  - [ ] Implement ACL system for descriptors
  - [ ] Add owner/group/permissions model
  - [ ] Implement sharing mechanisms

#### Phase 3: Encryption & Key Management
- [ ] Implement descriptor encryption
  - [ ] Add AES-256-GCM encryption for descriptors
  - [ ] Implement key derivation from user passwords
  - [ ] Add public key encryption option
- [ ] Secure local storage
  - [ ] Encrypt index files
  - [ ] Implement secure cache storage
  - [ ] Add memory protection

#### Phase 4: Streaming Media Security
- [ ] Implement secure streaming
  - [ ] Add HLS/DASH protocol support
  - [ ] Implement chunked encryption
  - [ ] Add DRM integration hooks
  - [ ] Implement secure media delivery

#### Phase 5: Input Validation & Hardening
- [ ] Comprehensive input validation
  - [ ] Add file type validation
  - [ ] Implement upload size limits
  - [ ] Add rate limiting
  - [ ] Sanitize all user inputs
- [ ] Security headers and CORS
  - [ ] Add security headers (CSP, HSTS, etc.)
  - [ ] Configure CORS properly
  - [ ] Implement CSRF protection

### Implementation Priority
1. **HTTPS for Web UI** - Critical for any production use
2. **Descriptor Encryption** - Protects metadata privacy
3. **User Authentication** - Required for access control
4. **Local Storage Encryption** - Protects cached data
5. **Streaming Support** - For media use cases

## Next Steps
1. Review and approve this security plan
2. Begin Phase 1 implementation (Network Security)
3. Update threat model with mitigations
4. Create security documentation for users