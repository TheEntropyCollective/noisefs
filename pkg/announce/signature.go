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

// SignatureVerifier handles IPNS signature verification for announcements
type SignatureVerifier struct {
	requireSignature bool
}

// NewSignatureVerifier creates a new signature verifier
func NewSignatureVerifier(requireSignature bool) *SignatureVerifier {
	return &SignatureVerifier{
		requireSignature: requireSignature,
	}
}

// VerifyAnnouncement verifies the IPNS signature of an announcement if present
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

// verifySignature performs the actual signature verification
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

// canonicalizeAnnouncement creates a canonical representation for signing
// This excludes the signature and peerID fields to avoid circular dependency
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

// extractPublicKey extracts the public key from a peer ID
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

// ValidatePeerID validates that a peer ID string is properly formatted
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

// GetSignableContent returns the canonical content that would be signed
// This is useful for testing and debugging
func (sv *SignatureVerifier) GetSignableContent(ann *Announcement) ([]byte, error) {
	return sv.canonicalizeAnnouncement(ann)
}

// SignatureAlgorithm returns the algorithm used by a peer ID's public key
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