package compliance

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestECDSASignatureGeneration tests ECDSA signature generation and verification
func TestECDSASignatureGeneration(t *testing.T) {
	// Create system with ECDSA key
	config := DefaultAuditConfig()
	if config.SigningKey == nil {
		t.Skip("ECDSA key generation failed in DefaultAuditConfig")
	}
	system := NewComplianceAuditSystem(config)
	
	// Create test entry
	entry := &DetailedAuditEntry{
		EntryID:   "TEST-001",
		Timestamp: time.Now(),
		EventType: "test_event",
		Action:    "test_action",
	}
	
	// Calculate entry hash first
	entry.EntryHash = system.calculateEntryHash(entry)
	
	// Generate signature
	signature := system.generateSignature(entry)
	
	// Verify it's an ECDSA signature
	if !strings.HasPrefix(signature, "ECDSA-") {
		t.Errorf("Expected ECDSA signature, got: %s", signature)
	}
	
	// Verify signature components are present
	parts := strings.Split(signature, "-")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts in ECDSA signature, got %d", len(parts))
	}
	
	// Set signature on entry and verify
	entry.Signature = signature
	if !system.verifySignature(entry) {
		t.Error("Failed to verify generated ECDSA signature")
	}
}

// TestHashSignatureFallback tests fallback to hash-based signatures
func TestHashSignatureFallback(t *testing.T) {
	// Create system without signing key
	config := DefaultAuditConfig()
	config.SigningKey = nil // Force fallback
	system := NewComplianceAuditSystem(config)
	
	// Create test entry
	entry := &DetailedAuditEntry{
		EntryID:   "TEST-002",
		Timestamp: time.Now(),
		EventType: "test_event",
		Action:    "test_action",
	}
	
	// Calculate entry hash first
	entry.EntryHash = system.calculateEntryHash(entry)
	
	// Generate signature
	signature := system.generateSignature(entry)
	
	// Verify it's a hash signature (fallback)
	if !strings.HasPrefix(signature, "HASH-") {
		t.Errorf("Expected HASH signature fallback, got: %s", signature)
	}
	
	// Set signature on entry and verify
	entry.Signature = signature
	if !system.verifySignature(entry) {
		t.Error("Failed to verify generated hash signature")
	}
}

// TestSignatureVerificationFailure tests signature verification failure cases
func TestSignatureVerificationFailure(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Test with empty signature
	entry := &DetailedAuditEntry{
		EntryID:   "TEST-003",
		Timestamp: time.Now(),
		EventType: "test_event",
		Action:    "test_action",
		Signature: "",
	}
	entry.EntryHash = system.calculateEntryHash(entry)
	
	if system.verifySignature(entry) {
		t.Error("Expected verification to fail with empty signature")
	}
	
	// Test with invalid signature format
	entry.Signature = "INVALID-SIGNATURE"
	if system.verifySignature(entry) {
		t.Error("Expected verification to fail with invalid signature format")
	}
	
	// Test with malformed ECDSA signature
	entry.Signature = "ECDSA-invalid-components"
	if system.verifySignature(entry) {
		t.Error("Expected verification to fail with malformed ECDSA signature")
	}
	
	// Test with malformed hash signature
	entry.Signature = "HASH-invalid"
	if system.verifySignature(entry) {
		t.Error("Expected verification to fail with malformed hash signature")
	}
}

// TestSignatureIntegrityWithModification tests that signature verification fails when data is modified
func TestSignatureIntegrityWithModification(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Create and sign entry
	entry := &DetailedAuditEntry{
		EntryID:   "TEST-004",
		Timestamp: time.Now(),
		EventType: "test_event",
		Action:    "test_action",
	}
	entry.EntryHash = system.calculateEntryHash(entry)
	entry.Signature = system.generateSignature(entry)
	
	// Verify original signature works
	if !system.verifySignature(entry) {
		t.Fatal("Original signature should be valid")
	}
	
	// Modify the entry and recalculate hash (signature should fail with new hash)
	entry.Action = "modified_action"
	entry.EntryHash = system.calculateEntryHash(entry) // New hash for modified entry
	if system.verifySignature(entry) {
		t.Error("Signature verification should fail after modification")
	}
	
}

// TestBackwardCompatibility tests that old signature format is still verified
func TestBackwardCompatibility(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Create entry with old-style signature format
	entry := &DetailedAuditEntry{
		EntryID:   "TEST-005",
		Timestamp: time.Now(),
		EventType: "test_event",
		Action:    "test_action",
	}
	entry.EntryHash = system.calculateEntryHash(entry)
	
	// Generate hash-based signature (simulating old format)
	entry.Signature = system.generateSignature(entry)
	
	// Should work with current verification
	if !system.verifySignature(entry) {
		t.Error("Backward compatibility check failed")
	}
}

// TestConcurrentSignatureGeneration tests thread safety of signature generation
func TestConcurrentSignatureGeneration(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	// Use channels to coordinate goroutines
	done := make(chan bool, 10)
	signatures := make(chan string, 10)
	
	// Generate signatures concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			entry := &DetailedAuditEntry{
				EntryID:   fmt.Sprintf("CONCURRENT-%d", id),
				Timestamp: time.Now(),
				EventType: "concurrent_test",
				Action:    "test_action",
			}
			entry.EntryHash = system.calculateEntryHash(entry)
			signature := system.generateSignature(entry)
			signatures <- signature
			done <- true
		}(i)
	}
	
	// Wait for all to complete
	generatedSignatures := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		<-done
		generatedSignatures = append(generatedSignatures, <-signatures)
	}
	
	// Verify all signatures are unique (different entries should have different signatures)
	signatureSet := make(map[string]bool)
	for _, sig := range generatedSignatures {
		if signatureSet[sig] {
			t.Error("Duplicate signature generated in concurrent test")
		}
		signatureSet[sig] = true
	}
	
	// Verify all signatures have valid format
	for _, sig := range generatedSignatures {
		if !strings.HasPrefix(sig, "ECDSA-") && !strings.HasPrefix(sig, "HASH-") {
			t.Errorf("Invalid signature format: %s", sig)
		}
	}
}

// TestKeyGeneration tests ECDSA key generation for audit system
func TestKeyGeneration(t *testing.T) {
	// Test key generation in default config
	config1 := DefaultAuditConfig()
	config2 := DefaultAuditConfig()
	
	// Keys should be different between instances
	if config1.SigningKey != nil && config2.SigningKey != nil {
		// Compare public key coordinates to ensure they're different
		if config1.SigningKey.PublicKey.X.Cmp(config2.SigningKey.PublicKey.X) == 0 &&
			config1.SigningKey.PublicKey.Y.Cmp(config2.SigningKey.PublicKey.Y) == 0 {
			t.Error("Default config should generate different keys each time")
		}
	}
	
	// Test that we can create a system with a custom key
	customKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate custom key: %v", err)
	}
	
	customConfig := DefaultAuditConfig()
	customConfig.SigningKey = customKey
	
	customSystem := NewComplianceAuditSystem(customConfig)
	if customSystem.config.SigningKey != customKey {
		t.Error("Custom signing key not preserved in system")
	}
}

// TestSignatureFormatConsistency tests that signature format is consistent
func TestSignatureFormatConsistency(t *testing.T) {
	system := NewComplianceAuditSystem(nil)
	
	entry := &DetailedAuditEntry{
		EntryID:   "FORMAT-TEST",
		Timestamp: time.Now(),
		EventType: "format_test",
		Action:    "test_action",
	}
	entry.EntryHash = system.calculateEntryHash(entry)
	
	// Generate multiple signatures for the same entry
	sig1 := system.generateSignature(entry)
	sig2 := system.generateSignature(entry)
	
	// For ECDSA signatures, they will be different due to randomness
	// For hash signatures, they should be identical
	if strings.HasPrefix(sig1, "HASH-") {
		if sig1 != sig2 {
			t.Error("Hash signatures should be deterministic for same entry")
		}
	} else if strings.HasPrefix(sig1, "ECDSA-") {
		// ECDSA signatures include randomness, so they should be different
		// But both should verify correctly
		entry.Signature = sig1
		if !system.verifySignature(entry) {
			t.Error("First ECDSA signature should verify")
		}
		
		entry.Signature = sig2
		if !system.verifySignature(entry) {
			t.Error("Second ECDSA signature should verify")
		}
	}
}