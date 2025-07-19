package announce

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestValidator_WithSignatureVerification_Integration(t *testing.T) {
	// Test with signatures optional (default)
	config := DefaultValidationConfig()
	validator := NewValidator(config)

	// Create a valid test announcement without signature
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
	}

	// Should pass without signature when signatures are optional
	err := validator.ValidateAnnouncement(ann)
	if err != nil {
		t.Errorf("ValidateAnnouncement() should pass without signature when optional: %v", err)
	}

	// Test with signatures required
	config.RequireSignatures = true
	validator = NewValidator(config)

	// Should fail without signature when signatures are required
	err = validator.ValidateAnnouncement(ann)
	if err == nil {
		t.Error("ValidateAnnouncement() should fail without signature when required")
	}
}

func TestValidator_WithValidSignature_Integration(t *testing.T) {
	// Generate Ed25519 key pair for testing
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatalf("Failed to create peer ID: %v", err)
	}

	// Create test announcement
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
		PeerID:     peerID.String(),
	}

	// Sign the announcement
	signatureVerifier := NewSignatureVerifier(false)
	content, err := signatureVerifier.canonicalizeAnnouncement(ann)
	if err != nil {
		t.Fatalf("Failed to canonicalize announcement: %v", err)
	}

	hash := sha256.Sum256(content)
	signature, err := privKey.Sign(hash[:])
	if err != nil {
		t.Fatalf("Failed to sign content: %v", err)
	}

	ann.Signature = base64.StdEncoding.EncodeToString(signature)

	// Test with signatures optional
	config := DefaultValidationConfig()
	validator := NewValidator(config)

	err = validator.ValidateAnnouncement(ann)
	if err != nil {
		t.Errorf("ValidateAnnouncement() should pass with valid signature: %v", err)
	}

	// Test with signatures required
	config.RequireSignatures = true
	validator = NewValidator(config)

	err = validator.ValidateAnnouncement(ann)
	if err != nil {
		t.Errorf("ValidateAnnouncement() should pass with valid signature when required: %v", err)
	}
}

func TestValidator_WithInvalidSignature_Integration(t *testing.T) {
	// Generate Ed25519 key pair for testing
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create peer ID
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatalf("Failed to create peer ID: %v", err)
	}

	// Create test announcement with invalid signature
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
		PeerID:     peerID.String(),
		Signature:  "aW52YWxpZC1zaWduYXR1cmU=", // base64 encoded "invalid-signature"
	}

	// Test with signatures optional - should still fail because signature is malformed
	config := DefaultValidationConfig()
	validator := NewValidator(config)

	err = validator.ValidateAnnouncement(ann)
	if err == nil {
		t.Error("ValidateAnnouncement() should fail with invalid signature even when optional")
	}

	// Test with signatures required - should also fail
	config.RequireSignatures = true
	validator = NewValidator(config)

	err = validator.ValidateAnnouncement(ann)
	if err == nil {
		t.Error("ValidateAnnouncement() should fail with invalid signature when required")
	}
}

func TestValidator_InvalidPeerID_Integration(t *testing.T) {
	// Create test announcement with invalid peer ID
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
		PeerID:     "invalid-peer-id-format",
		Signature:  "dGVzdC1zaWduYXR1cmU=", // base64 encoded "test-signature"
	}

	config := DefaultValidationConfig()
	validator := NewValidator(config)

	err := validator.ValidateAnnouncement(ann)
	if err == nil {
		t.Error("ValidateAnnouncement() should fail with invalid peer ID format")
	}

	// Error should mention peer ID validation
	if !strings.Contains(err.Error(), "peer ID") {
		t.Errorf("Error should mention peer ID validation, got: %v", err)
	}
}

func TestValidator_BackwardCompatibility_Integration(t *testing.T) {
	// Test that existing announcements without PeerID field still work
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
		// No PeerID, no Signature - should work with optional signatures
	}

	config := DefaultValidationConfig()
	config.RequireSignatures = false // Explicitly set to false
	validator := NewValidator(config)

	err := validator.ValidateAnnouncement(ann)
	if err != nil {
		t.Errorf("ValidateAnnouncement() should maintain backward compatibility: %v", err)
	}
}

func TestValidator_SignatureRequiredButMissing_Integration(t *testing.T) {
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
		// No signature
	}

	config := DefaultValidationConfig()
	config.RequireSignatures = true
	validator := NewValidator(config)

	err := validator.ValidateAnnouncement(ann)
	if err == nil {
		t.Error("ValidateAnnouncement() should fail when signature is required but missing")
	}

	// Error should mention signature requirement
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("Error should mention signature requirement, got: %v", err)
	}
}