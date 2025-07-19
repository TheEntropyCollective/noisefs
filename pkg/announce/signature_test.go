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

func TestSignatureVerifier_NewSignatureVerifier(t *testing.T) {
	tests := []struct {
		name             string
		requireSignature bool
	}{
		{"Optional signatures", false},
		{"Required signatures", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := NewSignatureVerifier(tt.requireSignature)
			if sv == nil {
				t.Fatal("NewSignatureVerifier returned nil")
			}
			if sv.requireSignature != tt.requireSignature {
				t.Errorf("Expected requireSignature=%v, got %v", tt.requireSignature, sv.requireSignature)
			}
		})
	}
}

func TestSignatureVerifier_ValidatePeerID(t *testing.T) {
	sv := NewSignatureVerifier(false)

	tests := []struct {
		name    string
		peerID  string
		wantErr bool
	}{
		{"Empty peer ID", "", true},
		{"Invalid format - wrong prefix", "invalid-peer-id", true},
		{"Valid Ed25519 peer ID format", "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXkT", false},
		{"Valid RSA peer ID format", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sv.ValidatePeerID(tt.peerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePeerID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSignatureVerifier_CanonicalizeAnnouncement(t *testing.T) {
	sv := NewSignatureVerifier(false)

	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce",
		Signature:  "should-be-excluded",
	}

	content, err := sv.canonicalizeAnnouncement(ann)
	if err != nil {
		t.Fatalf("canonicalizeAnnouncement() error = %v", err)
	}

	// Verify content doesn't contain signature
	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Fatal("canonicalizeAnnouncement() returned empty content")
	}

	// Should not contain signature field
	if strings.Contains(contentStr, "should-be-excluded") {
		t.Error("Canonical content should not include signature field")
	}

	// Should contain other fields
	if !strings.Contains(contentStr, "QmTest123") {
		t.Error("Canonical content should include descriptor")
	}
}

func TestSignatureVerifier_GetSignableContent(t *testing.T) {
	sv := NewSignatureVerifier(false)

	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
	}

	content1, err := sv.GetSignableContent(ann)
	if err != nil {
		t.Fatalf("GetSignableContent() error = %v", err)
	}

	content2, err := sv.GetSignableContent(ann)
	if err != nil {
		t.Fatalf("GetSignableContent() error = %v", err)
	}

	// Should be deterministic
	if string(content1) != string(content2) {
		t.Error("GetSignableContent() should return deterministic results")
	}
}

func TestSignatureVerifier_VerifyAnnouncement_UnsignedValid(t *testing.T) {
	tests := []struct {
		name             string
		requireSignature bool
		expectError      bool
	}{
		{"Optional signatures - unsigned valid", false, false},
		{"Required signatures - unsigned invalid", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := NewSignatureVerifier(tt.requireSignature)

			ann := &Announcement{
				Version:    "1.0",
				Descriptor: "QmTest123",
				TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
				Category:   "other",
				SizeClass:  "small",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				// No signature
			}

			err := sv.VerifyAnnouncement(ann)
			if (err != nil) != tt.expectError {
				t.Errorf("VerifyAnnouncement() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestSignatureVerifier_VerifyAnnouncement_SignatureWithoutPeerID(t *testing.T) {
	sv := NewSignatureVerifier(false)

	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Signature:  "fake-signature",
	}

	err := sv.VerifyAnnouncement(ann)
	if err == nil {
		t.Error("VerifyAnnouncement() should fail when signature present but peer ID missing")
	}
}

func TestSignatureVerifier_VerifyAnnouncement_InvalidSignatureEncoding(t *testing.T) {
	sv := NewSignatureVerifier(false)

	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmTest123",
		TopicHash:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Category:   "other",
		SizeClass:  "small",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Signature:  "invalid-base64-!@#$%",
	}

	ann.PeerID = "12D3KooWGzxzKZYveHXtpG6AsrUJBcWxHBFS2HsEoGTxrMLvKXkT"
	err := sv.VerifyAnnouncement(ann)
	if err == nil {
		t.Error("VerifyAnnouncement() should fail with invalid signature encoding")
	}
}

func TestSignatureVerifier_SignatureAlgorithms(t *testing.T) {
	sv := NewSignatureVerifier(false)

	// Test different key types
	keyTypes := []struct {
		name    string
		keyType int
	}{
		{"Ed25519", crypto.Ed25519},
		{"RSA", crypto.RSA},
		{"secp256k1", crypto.Secp256k1},
	}

	for _, kt := range keyTypes {
		t.Run(kt.name, func(t *testing.T) {
			// Generate key pair
			var privKey crypto.PrivKey
			var err error

			switch kt.keyType {
			case crypto.Ed25519:
				privKey, _, err = crypto.GenerateEd25519Key(rand.Reader)
			case crypto.RSA:
				// Skip RSA test as public keys are not embedded in peer IDs
				t.Skip("RSA keys don't embed public keys in peer IDs - skipping")
				return
			case crypto.Secp256k1:
				privKey, _, err = crypto.GenerateSecp256k1Key(rand.Reader)
			}

			if err != nil {
				t.Fatalf("Failed to generate %s key: %v", kt.name, err)
			}

			// Create peer ID
			peerID, err := peer.IDFromPrivateKey(privKey)
			if err != nil {
				t.Fatalf("Failed to create peer ID: %v", err)
			}

			// Test algorithm detection
			alg, err := sv.SignatureAlgorithm(peerID.String())
			if err != nil {
				t.Fatalf("SignatureAlgorithm() error = %v", err)
			}

			if alg != kt.name {
				t.Errorf("Expected algorithm %s, got %s", kt.name, alg)
			}
		})
	}
}

func TestSignatureVerifier_EndToEndVerification(t *testing.T) {
	sv := NewSignatureVerifier(false)

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
		Nonce:      "test-nonce",
	}

	// Get signable content
	content, err := sv.canonicalizeAnnouncement(ann)
	if err != nil {
		t.Fatalf("Failed to canonicalize announcement: %v", err)
	}

	// Hash and sign content
	hash := sha256.Sum256(content)
	signature, err := privKey.Sign(hash[:])
	if err != nil {
		t.Fatalf("Failed to sign content: %v", err)
	}

	// Add signature and peer ID to announcement
	ann.Signature = base64.StdEncoding.EncodeToString(signature)
	ann.PeerID = peerID.String()

	// Verify signature
	err = sv.VerifyAnnouncement(ann)
	if err != nil {
		t.Errorf("VerifyAnnouncement() failed: %v", err)
	}

	// Test with wrong peer ID (should fail)
	wrongPrivKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate wrong key: %v", err)
	}

	wrongPeerID, err := peer.IDFromPrivateKey(wrongPrivKey)
	if err != nil {
		t.Fatalf("Failed to create wrong peer ID: %v", err)
	}

	ann.PeerID = wrongPeerID.String()
	err = sv.VerifyAnnouncement(ann)
	if err == nil {
		t.Error("VerifyAnnouncement() should fail with wrong peer ID")
	}

	// Test with tampered content (should fail)
	ann.PeerID = peerID.String() // restore correct peer ID
	originalDescriptor := ann.Descriptor
	ann.Descriptor = "QmTampered"
	
	err = sv.VerifyAnnouncement(ann)
	if err == nil {
		t.Error("VerifyAnnouncement() should fail with tampered content")
	}

	// Restore original for final verification
	ann.Descriptor = originalDescriptor
	err = sv.VerifyAnnouncement(ann)
	if err != nil {
		t.Errorf("VerifyAnnouncement() should succeed with restored content: %v", err)
	}
}

func TestSignatureVerifier_ExtractPublicKey_Errors(t *testing.T) {
	sv := NewSignatureVerifier(false)

	tests := []struct {
		name   string
		peerID string
	}{
		{"Invalid peer ID format", "not-a-peer-id"},
		{"Empty peer ID", ""},
		{"Invalid base58", "12D3KooW!!!invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sv.extractPublicKey(tt.peerID)
			if err == nil {
				t.Error("extractPublicKey() should fail with invalid peer ID")
			}
		})
	}
}

