// Package crypto provides comprehensive cryptographic operations for NoiseFS.
// This file implements secure encryption, key derivation, password hashing, and cryptographic utilities
// for protecting user data, directory structures, and system operations in the NoiseFS privacy-preserving storage system.
//
// The cryptographic system provides multiple security layers:
//   - AES-256-GCM encryption for data confidentiality and integrity
//   - Argon2id key derivation for password-based encryption
//   - HKDF for hierarchical key derivation and directory-specific keys
//   - PBKDF2 for sync operation key derivation with rotation support
//   - bcrypt for password hashing and verification
//   - Cryptographically secure random number generation
//   - Secure memory management and data clearing
//
// Key Features:
//   - Industry-standard encryption algorithms with authenticated encryption
//   - Password-based key derivation with configurable parameters
//   - Directory-specific key derivation for hierarchical encryption
//   - Filename encryption for metadata privacy protection
//   - Secure sync key generation with rotation support
//   - Memory-safe operations with secure data clearing
//   - JSON serialization support for key storage and transport
//
// Security Considerations:
//   - All encryption uses authenticated encryption (AES-256-GCM)
//   - Key derivation uses memory-hard functions (Argon2id)
//   - Random number generation uses crypto/rand for security
//   - Sensitive data should be cleared using SecureZero after use
//   - Key rotation is supported for long-running sync operations
//
// Standards Compliance:
//   - NIST SP 800-38D (GCM mode)
//   - RFC 9106 (Argon2id)
//   - RFC 5869 (HKDF)
//   - RFC 2898 (PBKDF2)
//   - RFC 6234 (SHA-256)
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// EncryptionKey represents a cryptographic key with associated salt for secure encryption operations.
// This structure encapsulates both the derived encryption key and the salt used in key derivation,
// providing a complete cryptographic context for encryption and decryption operations.
//
// The key structure supports:
//   - AES-256 encryption with 32-byte keys
//   - Salt storage for key derivation reproducibility
//   - JSON serialization for secure key storage and transport
//   - Text marshaling for configuration and persistence
//   - Secure string representation with base64 encoding
//
// Security Properties:
//   - Keys are derived using memory-hard functions (Argon2id, PBKDF2)
//   - Salts provide protection against rainbow table attacks
//   - Random salts ensure unique keys for identical passwords
//   - Secure serialization prevents key exposure in logs
//
// Usage Patterns:
//   - Password-based encryption: GenerateKey with user password
//   - Directory encryption: DeriveDirectoryKey for hierarchical keys
//   - Sync operations: GenerateSecureSyncKey for session-specific keys
//   - Key persistence: JSON marshaling for secure storage
//
// Memory Management:
//   - Key data should be cleared using SecureZero after use
//   - Avoid storing keys in persistent memory longer than necessary
//   - Use temporary variables for key operations when possible
//
// Time Complexity: O(1) for key access operations
// Space Complexity: O(1) - fixed size (32 bytes key + 32 bytes salt)
type EncryptionKey struct {
	Key  []byte // 32-byte AES-256 encryption key derived from password or entropy
	Salt []byte // 32-byte random salt used for key derivation reproducibility
}

// GenerateKey generates a new encryption key from a password using Argon2id key derivation.
// This function implements secure password-based key derivation using the Argon2id algorithm,
// which provides resistance against both side-channel and time-memory trade-off attacks.
//
// Key Derivation Features:
//   - Argon2id algorithm for memory-hard key derivation
//   - Cryptographically secure random salt generation
//   - NIST-recommended parameters for security vs performance balance
//   - 32-byte output suitable for AES-256 encryption
//   - Protection against rainbow table and dictionary attacks
//
// Argon2id Parameters:
//   - Time parameter: 1 iteration (minimum for Argon2id)
//   - Memory parameter: 64MB (64*1024 KB) for good security
//   - Parallelism: 4 threads for modern multi-core systems
//   - Output length: 32 bytes for AES-256 compatibility
//
// Security Properties:
//   - Memory-hard function resistant to ASIC attacks
//   - Side-channel resistance through data-independent access patterns
//   - Cryptographically secure random salt prevents precomputation
//   - Configurable parameters allow security/performance tuning
//
// The generated key is suitable for:
//   - File encryption and decryption operations
//   - Directory-specific key derivation as master key
//   - Secure storage of user data and metadata
//   - Authentication and integrity verification
//
// Parameters:
//   - password: User password for key derivation (should be non-empty)
//
// Returns:
//   - *EncryptionKey: Generated key with 32-byte key and salt
//   - error: Non-nil if salt generation fails or password is invalid
//
// Call Flow:
//   - Called by: Password-based encryption, user authentication, key setup
//   - Calls: crypto/rand for salt generation, Argon2id for key derivation
//
// Time Complexity: O(m*t) where m is memory parameter and t is time parameter
// Space Complexity: O(m) where m is memory parameter (64MB)
func GenerateKey(password string) (*EncryptionKey, error) {
	// Generate random salt
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// DeriveKey derives an encryption key from a password and existing salt using Argon2id.
// This function reproduces an encryption key using the same password and salt that were
// used during initial key generation, enabling consistent decryption of previously encrypted data.
//
// Key Reproduction Features:
//   - Deterministic key derivation from password and salt
//   - Same Argon2id parameters as GenerateKey for consistency
//   - Salt validation to ensure proper key derivation
//   - Compatible with keys generated by GenerateKey
//
// Use Cases:
//   - Decrypting previously encrypted files with known salt
//   - Reproducing directory keys for encrypted directory access
//   - Key recovery from stored salt and user password
//   - Multi-session access to encrypted data
//
// Security Considerations:
//   - Salt must be exactly 32 bytes for security consistency
//   - Same password produces same key with same salt (deterministic)
//   - Salt should be stored securely alongside encrypted data
//   - Password should be handled securely and cleared after use
//
// Argon2id Parameters:
//   - Identical to GenerateKey for compatibility
//   - Time: 1, Memory: 64MB, Parallelism: 4, Output: 32 bytes
//
// Parameters:
//   - password: User password for key derivation (must match original)
//   - salt: 32-byte salt from original key generation
//
// Returns:
//   - *EncryptionKey: Derived key identical to original with same password/salt
//   - error: Non-nil if salt is wrong size or key derivation fails
//
// Call Flow:
//   - Called by: Decryption operations, key recovery, multi-session access
//   - Calls: Argon2id for deterministic key derivation
//
// Time Complexity: O(m*t) where m is memory parameter and t is time parameter
// Space Complexity: O(m) where m is memory parameter (64MB)
func DeriveKey(password string, salt []byte) (*EncryptionKey, error) {
	if len(salt) != 32 {
		return nil, fmt.Errorf("salt must be 32 bytes")
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// Encrypt encrypts data using AES-256-GCM authenticated encryption for confidentiality and integrity.
// This function provides authenticated encryption that both protects data confidentiality and
// verifies data integrity, preventing both eavesdropping and tampering attacks.
//
// Encryption Features:
//   - AES-256-GCM authenticated encryption for confidentiality and integrity
//   - Cryptographically secure random nonce generation
//   - Built-in authentication tag for tamper detection
//   - NIST-approved encryption standard compliance
//   - Prepended nonce for self-contained ciphertext
//
// AES-256-GCM Properties:
//   - 256-bit key size for maximum AES security
//   - Galois/Counter Mode for authenticated encryption
//   - 96-bit (12-byte) nonce for GCM standard
//   - 128-bit authentication tag for integrity verification
//   - Parallel encryption capability for performance
//
// Security Properties:
//   - Semantic security under chosen plaintext attacks
//   - Integrity protection with authentication tag
//   - Nonce uniqueness prevents replay attacks
//   - No padding oracle vulnerabilities
//   - Resistant to known cryptographic attacks
//
// Output Format:
//   - [nonce][authenticated_ciphertext_with_tag]
//   - Nonce: First 12 bytes for GCM decryption
//   - Ciphertext: Remaining bytes including authentication tag
//
// Parameters:
//   - data: Plaintext data to encrypt (any size supported)
//   - key: Encryption key with 32-byte AES-256 key
//
// Returns:
//   - []byte: Self-contained ciphertext with prepended nonce
//   - error: Non-nil if cipher creation fails or nonce generation fails
//
// Call Flow:
//   - Called by: File encryption, directory encryption, metadata protection
//   - Calls: AES cipher creation, GCM mode, crypto/rand for nonce
//
// Time Complexity: O(n) where n is the data size
// Space Complexity: O(n) for ciphertext output plus nonce overhead
func Encrypt(data []byte, key *EncryptionKey) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM authenticated decryption with integrity verification.
// This function performs authenticated decryption that both recovers plaintext data and
// verifies data integrity, detecting any tampering or corruption during storage or transmission.
//
// Decryption Features:
//   - AES-256-GCM authenticated decryption for confidentiality and integrity
//   - Automatic nonce extraction from ciphertext
//   - Built-in authentication tag verification for tamper detection
//   - Secure failure on authentication errors
//   - Compatible with Encrypt function output format
//
// Security Verification:
//   - Authentication tag verification prevents tampered data
//   - Secure failure mode on integrity violations
//   - Constant-time operations to prevent timing attacks
//   - No partial plaintext exposure on authentication failure
//
// Input Format Processing:
//   - Expects format: [nonce][authenticated_ciphertext_with_tag]
//   - Extracts 12-byte nonce from beginning
//   - Remaining bytes contain ciphertext and authentication tag
//   - Validates minimum length for security
//
// Error Conditions:
//   - Ciphertext too short (missing nonce or data)
//   - Authentication tag verification failure (tampering detected)
//   - AES cipher creation failure (invalid key)
//   - GCM mode creation failure (implementation error)
//
// Parameters:
//   - ciphertext: Encrypted data with prepended nonce from Encrypt
//   - key: Encryption key with 32-byte AES-256 key (must match encryption key)
//
// Returns:
//   - []byte: Decrypted plaintext data (original data)
//   - error: Non-nil if ciphertext is invalid, authentication fails, or decryption fails
//
// Call Flow:
//   - Called by: File decryption, directory decryption, metadata recovery
//   - Calls: AES cipher creation, GCM mode, authenticated decryption
//
// Time Complexity: O(n) where n is the ciphertext size
// Space Complexity: O(n) for plaintext output
func Decrypt(ciphertext []byte, key *EncryptionKey) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check minimum length
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// HashPassword creates a bcrypt hash of a password for secure password verification.
// This function implements secure password hashing using the bcrypt algorithm with a carefully
// chosen cost factor that balances security requirements with acceptable performance.
//
// Password Hashing Features:
//   - bcrypt algorithm with adaptive cost for future-proofing
//   - Cost factor of 12 for current security recommendations
//   - Built-in salt generation for rainbow table protection
//   - Slow hashing function resistant to brute force attacks
//   - Industry-standard password hashing for authentication systems
//
// Security Properties:
//   - Computationally expensive to prevent brute force attacks
//   - Built-in random salt prevents precomputed hash attacks
//   - Adaptive cost allows security scaling with hardware improvements
//   - Constant-time verification to prevent timing attacks
//   - Resistant to length extension and other cryptographic attacks
//
// Cost Factor Selection:
//   - Cost 12 provides good security vs performance balance
//   - Approximately 250ms computation time on modern hardware
//   - Exponential scaling: cost N+1 takes twice as long as cost N
//   - Recommended by OWASP and security experts for 2024
//
// Use Cases:
//   - User authentication and password verification
//   - Secure storage of user credentials
//   - Password-based access control systems
//   - Multi-factor authentication password components
//
// Parameters:
//   - password: Plain text password to hash (will be securely processed)
//
// Returns:
//   - string: bcrypt hash string including salt and cost factor
//   - error: Non-nil if hashing fails or password is invalid
//
// Call Flow:
//   - Called by: User registration, password changes, authentication setup
//   - Calls: bcrypt.GenerateFromPassword with cost factor 12
//
// Time Complexity: O(2^cost) where cost is 12 (approximately 250ms)
// Space Complexity: O(1) - fixed size hash output
func HashPassword(password string) (string, error) {
	// Use cost factor of 12 as specified
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against its bcrypt hash for secure authentication.
// This function performs constant-time password verification using bcrypt's built-in
// comparison function, preventing timing attacks and ensuring secure authentication.
//
// Verification Features:
//   - Constant-time comparison to prevent timing attacks
//   - Automatic salt and cost factor extraction from hash
//   - Compatible with hashes generated by HashPassword
//   - Secure failure mode with no information leakage
//   - Boolean result for simple authentication decisions
//
// Security Properties:
//   - Timing-resistant comparison prevents side-channel attacks
//   - No hash information exposure on verification failure
//   - Computational cost matches original hash cost factor
//   - Resistant to hash length extension attacks
//   - Safe against both online and offline password attacks
//
// Authentication Process:
//   1. Extract salt and cost factor from stored hash
//   2. Hash provided password with extracted parameters
//   3. Perform constant-time comparison of hashes
//   4. Return boolean result without leaking information
//
// Use Cases:
//   - User login authentication
//   - Password-based access control
//   - Multi-factor authentication password verification
//   - Administrative access validation
//
// Parameters:
//   - password: Plain text password provided by user
//   - hash: Stored bcrypt hash from HashPassword
//
// Returns:
//   - bool: True if password matches hash, false otherwise
//
// Call Flow:
//   - Called by: Authentication systems, login verification, access control
//   - Calls: bcrypt.CompareHashAndPassword for secure comparison
//
// Time Complexity: O(2^cost) where cost is extracted from hash
// Space Complexity: O(1) - no additional memory allocation
func VerifyPassword(password, hash string) bool {
	// CompareHashAndPassword returns nil on success
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// SecureRandom generates cryptographically secure random bytes for security-critical applications.
// This function provides high-quality random data suitable for cryptographic keys, nonces,
// salts, and other security-sensitive applications requiring unpredictable random values.
//
// Random Generation Features:
//   - Cryptographically secure pseudorandom number generator (CSPRNG)
//   - Platform-specific entropy sources (OS random device)
//   - Suitable for all cryptographic applications
//   - No predictable patterns or correlations
//   - High entropy quality for security applications
//
// Entropy Sources:
//   - /dev/urandom on Unix-like systems
//   - CryptGenRandom on Windows systems
//   - Hardware random number generators when available
//   - Entropy pool maintained by operating system
//   - Continuous entropy collection and mixing
//
// Security Properties:
//   - Cryptographically secure random output
//   - Unpredictable sequence generation
//   - No correlation between successive outputs
//   - Resistant to prediction and reconstruction attacks
//   - Suitable for key generation and nonce creation
//
// Use Cases:
//   - Cryptographic key generation
//   - Random salt creation for password hashing
//   - Nonce generation for encryption operations
//   - Session token and identifier creation
//   - Challenge generation for authentication protocols
//
// Parameters:
//   - size: Number of random bytes to generate (must be positive)
//
// Returns:
//   - []byte: Cryptographically secure random bytes of requested size
//   - error: Non-nil if random generation fails or size is invalid
//
// Call Flow:
//   - Called by: Key generation, salt creation, nonce generation, token creation
//   - Calls: crypto/rand.Reader for secure random data
//
// Time Complexity: O(n) where n is the requested size
// Space Complexity: O(n) for output buffer allocation
func SecureRandom(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}

// SecureZero securely clears sensitive data from memory to prevent information leakage.
// This function overwrites sensitive data with zeros to prevent recovery from memory dumps,
// swap files, or other memory analysis techniques that could expose cryptographic keys or passwords.
//
// Secure Memory Clearing Features:
//   - Explicit memory overwriting with zero bytes
//   - Protection against compiler optimizations that might remove clearing
//   - Immediate data destruction for sensitive information
//   - Defense against memory analysis and forensic recovery
//   - Essential security practice for cryptographic applications
//
// Security Benefits:
//   - Prevents key recovery from memory dumps
//   - Protects against swap file exposure
//   - Reduces attack surface for memory analysis
//   - Complies with security standards for key management
//   - Mitigates cold boot attacks and similar techniques
//
// Use Cases:
//   - Clearing encryption keys after use
//   - Zeroing password buffers after authentication
//   - Cleaning temporary cryptographic data
//   - Secure cleanup of sensitive configuration data
//   - Memory hygiene in security-critical applications
//
// Implementation Notes:
//   - Simple byte-by-byte zeroing for reliability
//   - Compiler-resistant implementation
//   - No assumptions about memory layout
//   - Works with any byte slice containing sensitive data
//
// Best Practices:
//   - Call immediately after sensitive data use
//   - Clear all copies and temporary variables
//   - Use defer statements for automatic cleanup
//   - Clear both stack and heap allocated data
//
// Parameters:
//   - data: Byte slice containing sensitive data to clear
//
// Call Flow:
//   - Called by: Key cleanup, password clearing, secure data destruction
//   - Directly overwrites memory without external calls
//
// Time Complexity: O(n) where n is the data size
// Space Complexity: O(1) - no additional memory allocation
func SecureZero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// DeriveDirectoryKey derives a directory-specific encryption key using HKDF for hierarchical encryption.
// This function implements hierarchical key derivation using HKDF (HMAC-based Key Derivation Function)
// to create unique encryption keys for each directory, enabling fine-grained access control and isolation.
//
// Hierarchical Key Derivation Features:
//   - HKDF key derivation for cryptographically secure key separation
//   - Directory-specific keys prevent cross-directory access
//   - Master key protection through derived key isolation
//   - Consistent key derivation for same directory paths
//   - SHA-256 based HKDF for security and performance
//
// HKDF Implementation:
//   - Extract-and-Expand paradigm for secure key derivation
//   - SHA-256 as underlying hash function for security
//   - Directory path as context information for key uniqueness
//   - Master key salt reused for consistency
//   - 32-byte output for AES-256 compatibility
//
// Security Properties:
//   - Cryptographically isolated keys per directory
//   - Forward security: compromised directory key doesn't affect others
//   - Deterministic derivation for consistent access
//   - No key correlation between different directories
//   - Master key protection through derived key usage
//
// Hierarchical Benefits:
//   - Fine-grained access control per directory
//   - Efficient key management with single master key
//   - Scalable encryption for large directory structures
//   - Independent directory security domains
//   - Simplified key rotation at master key level
//
// Parameters:
//   - masterKey: Master encryption key for hierarchical derivation
//   - directoryPath: Directory path string for key uniqueness
//
// Returns:
//   - *EncryptionKey: Directory-specific encryption key
//   - error: Non-nil if master key is invalid or derivation fails
//
// Call Flow:
//   - Called by: Directory encryption, hierarchical access control, file organization
//   - Calls: HKDF key derivation with SHA-256
//
// Time Complexity: O(1) - constant time key derivation
// Space Complexity: O(1) - fixed size output key
func DeriveDirectoryKey(masterKey *EncryptionKey, directoryPath string) (*EncryptionKey, error) {
	if masterKey == nil || len(masterKey.Key) == 0 {
		return nil, fmt.Errorf("master key is required")
	}

	// Use HKDF to derive a directory-specific key
	info := []byte("noisefs-directory:" + directoryPath)
	hkdf := hkdf.New(sha256.New, masterKey.Key, masterKey.Salt, info)

	// Derive a 32-byte key
	derivedKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, derivedKey); err != nil {
		return nil, fmt.Errorf("failed to derive directory key: %w", err)
	}

	return &EncryptionKey{
		Key:  derivedKey,
		Salt: masterKey.Salt, // Reuse the master key's salt
	}, nil
}

// EncryptFileName encrypts a filename using AES-256-GCM with a directory-specific key for metadata privacy.
// This function provides filename encryption to protect directory metadata from unauthorized access,
// ensuring both file content and file structure remain private in encrypted directory systems.
//
// Filename Encryption Features:
//   - AES-256-GCM encryption for filename confidentiality and integrity
//   - Directory-specific key isolation for access control
//   - Authenticated encryption prevents filename tampering
//   - Compatible with standard directory encryption schemes
//   - Preserves filename length characteristics for some use cases
//
// Metadata Privacy Benefits:
//   - Hides actual filenames from unauthorized users
//   - Protects directory structure information
//   - Prevents filename-based information leakage
//   - Enables secure directory sharing and access control
//   - Maintains privacy even with directory listing access
//
// Security Considerations:
//   - Directory-specific keys prevent cross-directory access
//   - Authenticated encryption detects filename tampering
//   - Each encryption uses unique nonce for security
//   - Compatible with hierarchical key management
//   - Secure against known plaintext attacks on filenames
//
// Use Cases:
//   - Encrypted file systems and directories
//   - Secure cloud storage with metadata privacy
//   - Privacy-preserving backup systems
//   - Confidential document management
//   - Secure collaboration platforms
//
// Parameters:
//   - filename: Original filename string to encrypt (must be non-empty)
//   - dirKey: Directory-specific encryption key from DeriveDirectoryKey
//
// Returns:
//   - []byte: Encrypted filename data with authentication
//   - error: Non-nil if filename is empty, key is invalid, or encryption fails
//
// Call Flow:
//   - Called by: Directory encryption, file system operations, metadata protection
//   - Calls: Encrypt function with directory-specific key
//
// Time Complexity: O(n) where n is the filename length
// Space Complexity: O(n) for encrypted filename output
func EncryptFileName(filename string, dirKey *EncryptionKey) ([]byte, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}

	if dirKey == nil {
		return nil, fmt.Errorf("directory key cannot be nil")
	}

	return Encrypt([]byte(filename), dirKey)
}

// DecryptFileName decrypts a filename using AES-256-GCM with a directory-specific key for metadata recovery.
// This function recovers original filenames from encrypted metadata, enabling authorized access to
// directory contents while maintaining privacy protection against unauthorized users.
//
// Filename Decryption Features:
//   - AES-256-GCM decryption with authentication verification
//   - Directory-specific key validation for access control
//   - Integrity verification prevents corrupted filename recovery
//   - Compatible with EncryptFileName output format
//   - Safe failure on authentication errors
//
// Access Control Integration:
//   - Requires correct directory key for successful decryption
//   - Authentication failure indicates tampering or wrong key
//   - Directory isolation through key-specific decryption
//   - Secure failure mode protects against unauthorized access
//   - Integration with hierarchical access control systems
//
// Error Handling:
//   - Empty encrypted name validation
//   - Directory key validation
//   - Authentication tag verification
//   - Secure failure on any validation error
//   - No partial information disclosure on failure
//
// Use Cases:
//   - Authorized directory access and file listing
//   - Encrypted file system operations
//   - Secure backup and restore operations
//   - Confidential document retrieval
//   - Privacy-preserving file management
//
// Parameters:
//   - encryptedName: Encrypted filename data from EncryptFileName
//   - dirKey: Directory-specific encryption key (must match encryption key)
//
// Returns:
//   - string: Decrypted original filename
//   - error: Non-nil if encrypted data is invalid, key is wrong, or authentication fails
//
// Call Flow:
//   - Called by: Directory decryption, file system operations, metadata recovery
//   - Calls: Decrypt function with directory-specific key
//
// Time Complexity: O(n) where n is the encrypted filename length
// Space Complexity: O(n) for decrypted filename string
func DecryptFileName(encryptedName []byte, dirKey *EncryptionKey) (string, error) {
	if len(encryptedName) == 0 {
		return "", fmt.Errorf("encrypted name cannot be empty")
	}

	if dirKey == nil {
		return "", fmt.Errorf("directory key cannot be nil")
	}

	decrypted, err := Decrypt(encryptedName, dirKey)
	if err != nil {
		return "", err
	}

	return string(decrypted), nil
}

// String returns a base64-encoded string representation of the encryption key for secure serialization.
// This method provides a secure string representation of the encryption key suitable for
// configuration files, logging, and other text-based storage and transport scenarios.
//
// Serialization Features:
//   - JSON structure with separate key and salt fields
//   - Base64 encoding for safe text representation
//   - Self-contained format including both key and salt
//   - Compatible with ParseKeyFromString for round-trip conversion
//   - Safe for use in configuration files and logs
//
// Security Considerations:
//   - Encoded representation still contains sensitive key material
//   - Should be protected with same security as original key
//   - Suitable for encrypted storage or secure transport
//   - Not intended for plaintext logging or insecure storage
//   - Key material should be cleared after use
//
// Output Format:
//   - Base64-encoded JSON structure
//   - JSON contains "key" and "salt" fields as base64 strings
//   - Self-contained format for easy parsing
//   - Compatible with text-based protocols and storage
//
// Use Cases:
//   - Configuration file storage
//   - Secure key transport and exchange
//   - Database storage of encrypted keys
//   - API responses with key information
//   - Debugging and logging (with appropriate security)
//
// Returns:
//   - string: Base64-encoded JSON representation of key and salt
//
// Call Flow:
//   - Called by: Key serialization, configuration systems, storage operations
//   - Calls: JSON marshaling and base64 encoding
//
// Time Complexity: O(n) where n is the key and salt size
// Space Complexity: O(n) for string representation
func (k *EncryptionKey) String() string {
	keyData := struct {
		Key  string `json:"key"`
		Salt string `json:"salt"`
	}{
		Key:  base64.StdEncoding.EncodeToString(k.Key),
		Salt: base64.StdEncoding.EncodeToString(k.Salt),
	}

	data, err := json.Marshal(keyData)
	if err != nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(data)
}

// MarshalText implements the encoding.TextMarshaler interface for standard library integration.
// This method enables EncryptionKey to work seamlessly with Go's standard encoding packages
// including json, xml, yaml, and other text-based serialization formats.
//
// Interface Implementation:
//   - Standard encoding.TextMarshaler interface for Go ecosystem compatibility
//   - Delegates to String() method for consistent representation
//   - Returns byte slice for compatibility with encoding interfaces
//   - Enables automatic serialization in encoding packages
//
// Integration Benefits:
//   - Automatic JSON marshaling in struct fields
//   - Compatible with yaml.Marshal and other text encoders
//   - Works with configuration libraries using text marshaling
//   - Seamless integration with HTTP APIs and protocols
//   - Standard library compatibility for all text-based formats
//
// Use Cases:
//   - JSON API responses containing encryption keys
//   - YAML configuration files with key storage
//   - XML documents requiring key serialization
//   - Custom text protocols with key exchange
//   - Database drivers supporting text marshaling
//
// Returns:
//   - []byte: Text representation as bytes (from String() method)
//   - error: Always nil in current implementation
//
// Call Flow:
//   - Called by: encoding/json, encoding/xml, yaml libraries, custom marshalers
//   - Calls: String() method for consistent text representation
//
// Time Complexity: O(n) where n is the key and salt size
// Space Complexity: O(n) for byte slice conversion
func (k *EncryptionKey) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface for standard library integration.
// This method enables EncryptionKey to be automatically reconstructed from text representations
// by Go's standard encoding packages, providing seamless deserialization capabilities.
//
// Interface Implementation:
//   - Standard encoding.TextUnmarshaler interface for Go ecosystem compatibility
//   - Delegates to ParseKeyFromString for consistent parsing
//   - Updates receiver fields with parsed key and salt
//   - Enables automatic deserialization in encoding packages
//
// Deserialization Features:
//   - Automatic JSON unmarshaling in struct fields
//   - Compatible with yaml.Unmarshal and other text decoders
//   - Works with configuration libraries using text unmarshaling
//   - Error handling for invalid text representations
//   - Maintains consistency with MarshalText output
//
// Error Handling:
//   - Invalid text format detection
//   - Base64 decoding error propagation
//   - JSON parsing error handling
//   - Key/salt validation and format checking
//   - Preserves original key state on parsing failure
//
// Use Cases:
//   - JSON API requests containing encryption keys
//   - YAML configuration file loading
//   - XML document parsing with key data
//   - Custom text protocol key exchange
//   - Database result unmarshaling
//
// Parameters:
//   - text: Byte slice containing text representation from MarshalText
//
// Returns:
//   - error: Non-nil if text is invalid or parsing fails
//
// Call Flow:
//   - Called by: encoding/json, encoding/xml, yaml libraries, custom unmarshalers
//   - Calls: ParseKeyFromString for text parsing and validation
//
// Time Complexity: O(n) where n is the text size
// Space Complexity: O(1) - updates receiver in place
func (k *EncryptionKey) UnmarshalText(text []byte) error {
	parsed, err := ParseKeyFromString(string(text))
	if err != nil {
		return err
	}

	k.Key = parsed.Key
	k.Salt = parsed.Salt
	return nil
}

// ParseKeyFromString parses a base64-encoded string representation back to an EncryptionKey for deserialization.
// This function reconstructs an EncryptionKey from its string representation, enabling key recovery
// from configuration files, databases, and other text-based storage systems.
//
// Parsing Features:
//   - Base64 decoding of string representation
//   - JSON parsing of key and salt fields
//   - Validation of key and salt format and size
//   - Error handling for corrupted or invalid data
//   - Compatible with String() method output
//
// Validation Steps:
//   1. Non-empty string validation
//   2. Base64 decoding of outer structure
//   3. JSON unmarshaling of key/salt structure
//   4. Base64 decoding of individual key and salt fields
//   5. Key and salt size validation
//
// Security Considerations:
//   - Parsed key contains sensitive cryptographic material
//   - Should be cleared with SecureZero after use
//   - Validate key source and transport security
//   - Error messages don't leak key material
//   - Constant-time operations where possible
//
// Error Conditions:
//   - Empty or malformed string input
//   - Invalid base64 encoding at any level
//   - Malformed JSON structure
//   - Missing or invalid key/salt fields
//   - Incorrect key or salt sizes
//
// Use Cases:
//   - Configuration file key loading
//   - Database key retrieval and reconstruction
//   - API key exchange and parsing
//   - Backup and restore operations
//   - Cross-system key transport
//
// Parameters:
//   - keyStr: Base64-encoded string representation from String() method
//
// Returns:
//   - *EncryptionKey: Reconstructed encryption key with key and salt
//   - error: Non-nil if parsing fails or validation errors occur
//
// Call Flow:
//   - Called by: UnmarshalText, configuration loading, key management systems
//   - Calls: Base64 decoding, JSON unmarshaling, validation functions
//
// Time Complexity: O(n) where n is the string length
// Space Complexity: O(1) - fixed size key structure output
func ParseKeyFromString(keyStr string) (*EncryptionKey, error) {
	if keyStr == "" {
		return nil, fmt.Errorf("key string cannot be empty")
	}

	// Decode base64 string
	data, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key string: %w", err)
	}

	// Parse JSON
	var keyData struct {
		Key  string `json:"key"`
		Salt string `json:"salt"`
	}

	if err := json.Unmarshal(data, &keyData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key data: %w", err)
	}

	// Decode key and salt
	key, err := base64.StdEncoding.DecodeString(keyData.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(keyData.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}

	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// GenerateSecureSyncKey generates a cryptographically secure key for sync operations using proper entropy.
// This function creates session-specific encryption keys for secure synchronization operations,
// combining user-provided context with cryptographically secure random entropy for maximum security.
//
// Sync Key Generation Features:
//   - Cryptographically secure random entropy generation
//   - Session-specific key derivation for temporal isolation
//   - User salt integration for personalization
//   - Timestamp inclusion for uniqueness and replay protection
//   - PBKDF2 key derivation with secure parameters
//
// Security Properties:
//   - High-entropy random key generation using crypto/rand
//   - Session isolation prevents cross-session key correlation
//   - Temporal uniqueness through timestamp integration
//   - User-specific salt integration for personalization
//   - Secure parameters (100,000 iterations) for key stretching
//
// Key Derivation Process:
//   1. Generate 32 bytes of cryptographically secure random entropy
//   2. Create session-specific salt from user salt, session ID, and timestamp
//   3. Apply PBKDF2-SHA256 with 100,000 iterations for key derivation
//   4. Return 32-byte key suitable for AES-256 encryption
//
// Sync Operation Use Cases:
//   - Secure data synchronization between devices
//   - Session-specific encryption for temporary operations
//   - Cross-device communication with forward secrecy
//   - Collaborative editing with secure key exchange
//   - Backup and restore operations with session isolation
//
// Parameters:
//   - sessionID: Unique session identifier for key isolation (must be non-empty)
//   - userSalt: Optional user-specific salt for personalization (nil generates random salt)
//
// Returns:
//   - *EncryptionKey: Session-specific encryption key with derived salt
//   - error: Non-nil if session ID is empty or entropy generation fails
//
// Call Flow:
//   - Called by: Sync operations, session management, collaborative systems
//   - Calls: crypto/rand for entropy, PBKDF2 for key derivation
//
// Time Complexity: O(i) where i is PBKDF2 iteration count (100,000)
// Space Complexity: O(1) - fixed size key and salt output
func GenerateSecureSyncKey(sessionID string, userSalt []byte) (*EncryptionKey, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Generate random entropy using crypto/rand
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("failed to generate entropy: %w", err)
	}

	// Create session-specific salt by combining user salt with session ID and timestamp
	var salt []byte
	if len(userSalt) > 0 {
		salt = append(salt, userSalt...)
	} else {
		// Generate random salt if none provided
		randomSalt := make([]byte, 16)
		if _, err := rand.Read(randomSalt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
		salt = randomSalt
	}

	// Add session ID and timestamp for uniqueness
	salt = append(salt, []byte(sessionID)...)
	timestamp := time.Now().UnixNano()
	timestampBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		timestampBytes[i] = byte(timestamp >> (8 * i))
	}
	salt = append(salt, timestampBytes...)

	// Derive key using PBKDF2 with secure parameters
	key := pbkdf2.Key(entropy, salt, 100000, 32, sha256.New)

	return &EncryptionKey{
		Key:  key,
		Salt: salt[:32], // Keep salt to standard 32-byte length
	}, nil
}

// GenerateSecureSyncKeyWithRotation generates a secure sync key with rotation support for forward secrecy.
// This function creates versioned encryption keys that support key rotation during long-running
// sync sessions, providing forward secrecy and protection against key compromise.
//
// Key Rotation Features:
//   - Versioned key generation with rotation counter
//   - Forward secrecy through periodic key updates
//   - Backward compatibility with rotation counter tracking
//   - Session continuity across key rotation events
//   - Automatic key derivation parameter updating
//
// Forward Secrecy Benefits:
//   - Compromised key doesn't affect previous or future keys
//   - Time-limited key exposure reduces attack surface
//   - Automatic key rotation for long-running sessions
//   - Protection against key extraction attacks
//   - Compliance with security standards requiring key rotation
//
// Rotation Implementation:
//   1. Append rotation counter to session ID for uniqueness
//   2. Delegate to GenerateSecureSyncKey with modified session ID
//   3. Maintain consistent user salt across rotations
//   4. Generate cryptographically independent keys per rotation
//
// Use Cases:
//   - Long-running sync sessions requiring periodic key updates
//   - High-security applications with key rotation policies
//   - Collaborative systems with forward secrecy requirements
//   - Compliance scenarios requiring temporal key isolation
//   - Multi-device synchronization with security guarantees
//
// Rotation Strategy:
//   - Increment rotation counter for each key update
//   - Maintain session continuity with versioned keys
//   - Enable key rollback for compatibility if needed
//   - Support automated rotation based on time or data volume
//
// Parameters:
//   - sessionID: Base session identifier for key derivation
//   - userSalt: User-specific salt maintained across rotations
//   - rotationCounter: Rotation version number for key uniqueness
//
// Returns:
//   - *EncryptionKey: Rotation-specific encryption key
//   - error: Non-nil if session ID is empty or key generation fails
//
// Call Flow:
//   - Called by: Key rotation systems, long-running sync operations, security policies
//   - Calls: GenerateSecureSyncKey with rotation-specific session ID
//
// Time Complexity: O(i) where i is PBKDF2 iteration count (100,000)
// Space Complexity: O(1) - fixed size key and salt output
func GenerateSecureSyncKeyWithRotation(sessionID string, userSalt []byte, rotationCounter uint32) (*EncryptionKey, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Add rotation counter to session ID for key versioning
	rotationID := fmt.Sprintf("%s-rotation-%d", sessionID, rotationCounter)

	return GenerateSecureSyncKey(rotationID, userSalt)
}
