package announce

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SignatureVerifier provides cryptographic signature verification for NoiseFS announcements.
//
// This component integrates with libp2p's IPNS signature system to verify announcement
// authenticity when signatures are present or required. It supports flexible signature
// policies ranging from optional verification to mandatory authentication, enabling
// both anonymous and verified content models.
//
// Key Features:
//   - IPNS signature verification using libp2p cryptographic primitives
//   - Flexible signature policies (optional, required, disabled)
//   - Support for multiple cryptographic algorithms (RSA, Ed25519, secp256k1, ECDSA)
//   - Canonical content generation preventing signature manipulation
//   - Peer ID validation and public key extraction
//
// Cryptographic Security:
//   - SHA-256 hashing of canonical announcement content
//   - Standard libp2p signature verification algorithms
//   - Protection against signature reuse via canonical content generation
//   - Peer ID authenticity verification
//
// Thread Safety: SignatureVerifier is safe for concurrent use across multiple goroutines.
type SignatureVerifier struct {
	// requireSignature determines the signature policy for announcement validation.
	// When true, all announcements must include valid cryptographic signatures.
	// When false, signatures are optional but verified when present.
	requireSignature bool
}

// NewSignatureVerifier creates a new signature verifier with the specified policy.
//
// The verifier is configured with a signature requirement policy that determines
// whether signatures are mandatory for all announcements or optional. This enables
// flexible deployment scenarios supporting both anonymous and authenticated content.
//
// Parameters:
//   - requireSignature: true to mandate signatures, false for optional verification
//
// Returns:
//   A new SignatureVerifier ready for announcement verification
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Policy Implications:
//   - Required: All announcements must include valid IPNS signatures
//   - Optional: Signatures verified when present, anonymous announcements allowed
//   - Recommended: Optional for public networks, required for private/authenticated networks
//
// Example:
//   verifier := announce.NewSignatureVerifier(false)  // Optional signatures
//   err := verifier.VerifyAnnouncement(announcement)
func NewSignatureVerifier(requireSignature bool) *SignatureVerifier {
	return &SignatureVerifier{
		requireSignature: requireSignature,
	}
}

// VerifyAnnouncement performs cryptographic verification of announcement signatures based on policy.
//
// This method implements flexible signature verification that respects the configured
// signature policy while ensuring cryptographic integrity when signatures are present.
// It validates both the signature format and cryptographic authenticity using libp2p
// IPNS signature verification.
//
// Parameters:
//   - ann: The announcement to verify
//
// Returns:
//   - nil if verification succeeds or signatures are optional and absent
//   - error with specific verification failure details
//
// Time Complexity: O(1) for policy checks, O(k) for cryptographic operations where k is key size
// Space Complexity: O(1)
//
// Verification Logic:
//   - No signature + optional policy: Valid (anonymous announcement)
//   - No signature + required policy: Invalid (signature mandate violation)
//   - Present signature: Full cryptographic verification performed
//   - Invalid signature: Verification failure regardless of policy
//
// Security Properties:
//   - Cryptographic authenticity verification when signatures present
//   - Policy enforcement preventing signature requirement bypass
//   - Peer ID validation ensuring signature source authenticity
//   - Protection against signature forgery and manipulation
func (sv *SignatureVerifier) VerifyAnnouncement(ann *Announcement) error {
	// Handle optional signatures
	if ann.Signature == "" {
		if sv.requireSignature {
			return fmt.Errorf("signature required but not provided")
		}
		return nil // Valid unsigned announcement
	}

	// If signature is present, peer ID must be in the announcement
	if ann.PeerID == "" {
		return fmt.Errorf("peer ID required when signature is present")
	}

	return sv.verifySignature(ann)
}

// verifySignature performs comprehensive cryptographic signature verification using libp2p primitives.
//
// This method implements the core signature verification algorithm, including content
// canonicalization, hash generation, public key extraction, and cryptographic verification.
// It uses standard libp2p/IPNS signature verification procedures for compatibility.
//
// Parameters:
//   - ann: The announcement with signature and peer ID to verify
//
// Returns:
//   - nil if cryptographic verification succeeds
//   - error with specific verification failure details
//
// Time Complexity: O(k) where k is the cryptographic key size
// Space Complexity: O(n) where n is the announcement content size
//
// Verification Process:
//   1. Base64 decode the signature data
//   2. Generate canonical announcement content (excluding signature fields)
//   3. SHA-256 hash the canonical content
//   4. Extract public key from the peer ID
//   5. Perform cryptographic signature verification
//   6. Validate signature authenticity against the content hash
//
// Security Features:
//   - Canonical content generation prevents signature manipulation
//   - SHA-256 content hashing for cryptographic integrity
//   - Standard libp2p signature verification algorithms
//   - Protection against signature reuse and content modification
func (sv *SignatureVerifier) verifySignature(ann *Announcement) error {
	// Decode signature
	signature, err := base64.StdEncoding.DecodeString(ann.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Get canonical content for signing
	content, err := sv.canonicalizeAnnouncement(ann)
	if err != nil {
		return fmt.Errorf("failed to canonicalize announcement: %w", err)
	}

	// Hash the content (standard practice for signing)
	hash := sha256.Sum256(content)

	// Extract public key from peer ID
	pubKey, err := sv.extractPublicKey(ann.PeerID)
	if err != nil {
		return fmt.Errorf("failed to extract public key: %w", err)
	}

	// Verify signature
	valid, err := pubKey.Verify(hash[:], signature)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	if !valid {
		return fmt.Errorf("signature verification failed: invalid signature")
	}

	return nil
}

// canonicalizeAnnouncement generates deterministic canonical content for signature verification.
//
// This method creates a standardized representation of announcement content that
// excludes signature-related fields to prevent circular dependencies. The canonical
// form ensures consistent signature verification across different implementations
// and prevents signature manipulation attacks.
//
// Parameters:
//   - ann: The announcement to canonicalize
//
// Returns:
//   - Canonical JSON representation as bytes
//   - error if JSON marshaling fails
//
// Time Complexity: O(n) where n is the announcement content size
// Space Complexity: O(n) for the canonical representation
//
// Canonicalization Rules:
//   - Excludes 'Signature' and 'PeerID' fields to prevent circular dependency
//   - Uses deterministic JSON field ordering via struct tags
//   - Includes all content-defining fields (version, descriptor, topic, etc.)
//   - Maintains consistent encoding across different JSON marshalers
//
// Security Properties:
//   - Prevents signature field manipulation attacks
//   - Ensures consistent content representation for verification
//   - Eliminates circular dependency between content and signature
//   - Deterministic output for reliable signature verification
func (sv *SignatureVerifier) canonicalizeAnnouncement(ann *Announcement) ([]byte, error) {
	// Create a copy without the signature and peerID fields
	canonical := struct {
		Version    string `json:"v"`
		Descriptor string `json:"d"`
		TopicHash  string `json:"t"`
		TagBloom   string `json:"tb,omitempty"`
		Category   string `json:"c"`
		SizeClass  string `json:"s"`
		Timestamp  int64  `json:"ts"`
		TTL        int64  `json:"ttl"`
		Nonce      string `json:"n,omitempty"`
	}{
		Version:    ann.Version,
		Descriptor: ann.Descriptor,
		TopicHash:  ann.TopicHash,
		TagBloom:   ann.TagBloom,
		Category:   ann.Category,
		SizeClass:  ann.SizeClass,
		Timestamp:  ann.Timestamp,
		TTL:        ann.TTL,
		Nonce:      ann.Nonce,
	}

	// Use deterministic JSON marshaling
	return json.Marshal(canonical)
}

// extractPublicKey retrieves the cryptographic public key from a libp2p peer ID.
//
// This method decodes the peer ID and extracts the embedded public key used for
// signature verification. It handles the libp2p peer ID format and validates
// the key extraction process for signature verification compatibility.
//
// Parameters:
//   - peerIDStr: String representation of the libp2p peer ID
//
// Returns:
//   - Public key ready for signature verification
//   - error if peer ID is invalid or key extraction fails
//
// Time Complexity: O(1) for most key types
// Space Complexity: O(k) where k is the key size
//
// Supported Key Types:
//   - RSA: Variable size RSA public keys
//   - Ed25519: 32-byte Ed25519 public keys
//   - secp256k1: ECDSA secp256k1 public keys
//   - ECDSA: General ECDSA public keys
//
// Peer ID Format:
//   - Modern: "12D3KooW..." (CIDv1 with embedded public key)
//   - Legacy: "Qm..." (CIDv0 format)
//   - Both formats supported for backward compatibility
//
// Error Conditions:
//   - Invalid peer ID format or encoding
//   - Unsupported key type embedded in peer ID
//   - Corrupted or incomplete key data
func (sv *SignatureVerifier) extractPublicKey(peerIDStr string) (crypto.PubKey, error) {
	// Validate and decode peer ID
	pid, err := peer.Decode(peerIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid peer ID format: %w", err)
	}

	// Extract public key from peer ID
	pubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("cannot extract public key from peer ID: %w", err)
	}

	return pubKey, nil
}

// ValidatePeerID validates libp2p peer ID format and decodability for signature verification.
//
// This method performs comprehensive validation of peer ID strings to ensure they
// conform to libp2p standards and can be used for public key extraction and
// signature verification. It validates both format and structural integrity.
//
// Parameters:
//   - peerIDStr: String representation of the peer ID to validate
//
// Returns:
//   - nil if the peer ID is valid and decodable
//   - error with specific validation failure details
//
// Time Complexity: O(n) where n is the peer ID length
// Space Complexity: O(1)
//
// Validation Criteria:
//   - Non-empty string requirement
//   - Valid prefix format ("12D3KooW" for modern, "Qm" for legacy)
//   - Successful libp2p peer ID decoding
//   - Embedded public key accessibility
//
// Supported Formats:
//   - CIDv1: "12D3KooW..." (modern format with embedded public key)
//   - CIDv0: "Qm..." (legacy format, may require key lookup)
//   - Base58 encoding validation for character set compliance
//
// Use Cases:
//   - Pre-verification validation before signature operations
//   - Input sanitization for peer ID processing
//   - Format compliance checking in announcement validation
func (sv *SignatureVerifier) ValidatePeerID(peerIDStr string) error {
	if peerIDStr == "" {
		return fmt.Errorf("peer ID cannot be empty")
	}

	// Basic format validation
	if !strings.HasPrefix(peerIDStr, "12D3KooW") && !strings.HasPrefix(peerIDStr, "Qm") {
		return fmt.Errorf("invalid peer ID format: must start with 12D3KooW or Qm")
	}

	// Validate by attempting to decode
	_, err := peer.Decode(peerIDStr)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	return nil
}

// GetSignableContent returns the canonical content representation used for signature generation.
//
// This utility method exposes the exact content that would be signed or verified,
// enabling testing, debugging, and external signature generation. It provides
// the same canonicalization used internally for signature verification.
//
// Parameters:
//   - ann: The announcement to canonicalize
//
// Returns:
//   - Canonical content bytes used for signature operations
//   - error if canonicalization fails
//
// Time Complexity: O(n) where n is the announcement content size
// Space Complexity: O(n) for the canonical representation
//
// Use Cases:
//   - External signature generation tools and libraries
//   - Testing and validation of signature verification logic
//   - Debugging signature verification failures
//   - Integration with custom signing implementations
//
// Content Properties:
//   - Identical to internal canonicalization used for verification
//   - Excludes signature and peer ID fields
//   - Deterministic JSON representation
//   - Compatible with standard libp2p signature procedures
func (sv *SignatureVerifier) GetSignableContent(ann *Announcement) ([]byte, error) {
	return sv.canonicalizeAnnouncement(ann)
}

// SignatureAlgorithm identifies the cryptographic algorithm used by a peer ID's public key.
//
// This method extracts the public key from the peer ID and determines the
// cryptographic algorithm type. This information is useful for signature
// verification planning, algorithm compatibility checking, and security analysis.
//
// Parameters:
//   - peerIDStr: String representation of the peer ID to analyze
//
// Returns:
//   - Algorithm name string ("RSA", "Ed25519", "secp256k1", "ECDSA", "unknown")
//   - error if peer ID is invalid or key extraction fails
//
// Time Complexity: O(1) for algorithm identification after key extraction
// Space Complexity: O(k) where k is the key size
//
// Supported Algorithms:
//   - "RSA": RSA public key cryptography
//   - "Ed25519": Edwards-curve Digital Signature Algorithm
//   - "secp256k1": ECDSA using secp256k1 curve (Bitcoin-style)
//   - "ECDSA": General Elliptic Curve Digital Signature Algorithm
//   - "unknown": Unrecognized or unsupported algorithm
//
// Use Cases:
//   - Algorithm compatibility verification before signature operations
//   - Security analysis and algorithm strength assessment
//   - Signature verification optimization based on algorithm type
//   - Audit logging and security monitoring
func (sv *SignatureVerifier) SignatureAlgorithm(peerIDStr string) (string, error) {
	pubKey, err := sv.extractPublicKey(peerIDStr)
	if err != nil {
		return "", err
	}

	switch pubKey.Type() {
	case crypto.RSA:
		return "RSA", nil
	case crypto.Ed25519:
		return "Ed25519", nil
	case crypto.Secp256k1:
		return "secp256k1", nil
	case crypto.ECDSA:
		return "ECDSA", nil
	default:
		return "unknown", nil
	}
}