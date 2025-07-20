package compliance

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

// FieldEncryption represents field-level encryption configuration
type FieldEncryption struct {
	masterKey       []byte
	encryptedFields map[string]bool
	keyDerivation   KeyDerivationStrategy
	gcm             cipher.AEAD
}

// KeyDerivationStrategy defines how encryption keys are derived
type KeyDerivationStrategy int

const (
	PerFieldStrategy KeyDerivationStrategy = iota
	PerRecordStrategy
	HybridStrategy
)

// EncryptedField represents an encrypted field with metadata
type EncryptedField struct {
	Ciphertext string `json:"ciphertext"`
	KeyID      string `json:"key_id"`
	Algorithm  string `json:"algorithm"`
	Version    int    `json:"version"`
}

// EncryptionConfig defines encryption parameters
type EncryptionConfig struct {
	Algorithm         string
	KeySize          int
	RotationInterval time.Duration
	MasterKeyID      string
}

// EncryptionMetrics tracks encryption performance
type EncryptionMetrics struct {
	EncryptionTime   time.Duration
	DecryptionTime   time.Duration
	SizeOverhead     int
	OperationCount   int64
}

// TestFieldEncryption_BasicOperations tests core encryption/decryption functionality
func TestFieldEncryption_BasicOperations(t *testing.T) {
	testCases := []struct {
		name      string
		plaintext string
		fieldType string
		expectError bool
	}{
		{
			name:      "Email Encryption",
			plaintext: "user@example.com",
			fieldType: "email",
			expectError: false,
		},
		{
			name:      "Legal Document Encryption",
			plaintext: "This is a DMCA takedown notice for copyrighted material...",
			fieldType: "legal_document",
			expectError: false,
		},
		{
			name:      "Address Encryption",
			plaintext: "123 Main St, Anytown, CA 90210",
			fieldType: "address",
			expectError: false,
		},
		{
			name:      "Empty Field",
			plaintext: "",
			fieldType: "email",
			expectError: false,
		},
		{
			name:      "Large Legal Document",
			plaintext: strings.Repeat("Legal content ", 1000),
			fieldType: "legal_document",
			expectError: false,
		},
		{
			name:      "Unicode Content",
			plaintext: "José González <josé@example.com>",
			fieldType: "email",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// TDD: This will fail until FieldEncryption is implemented
			encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
			if err != nil {
				t.Fatalf("Failed to create field encryption: %v", err)
			}

			// Test encryption
			encrypted, err := encryption.EncryptField(tc.plaintext, tc.fieldType)
			if tc.expectError && err == nil {
				t.Errorf("Expected encryption error for %s, got success", tc.name)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected encryption error for %s: %v", tc.name, err)
			}

			if !tc.expectError {
				// Verify encrypted data is different from plaintext
				if encrypted.Ciphertext == tc.plaintext && tc.plaintext != "" {
					t.Errorf("Encrypted data same as plaintext for %s", tc.name)
				}

				// Test decryption
				decrypted, err := encryption.DecryptField(encrypted, tc.fieldType)
				if err != nil {
					t.Errorf("Decryption failed for %s: %v", tc.name, err)
				}

				if decrypted != tc.plaintext {
					t.Errorf("Decrypted data doesn't match original for %s: got %q, want %q", 
						tc.name, decrypted, tc.plaintext)
				}

				// Verify encryption metadata
				if encrypted.Algorithm == "" {
					t.Errorf("Missing algorithm in encrypted field for %s", tc.name)
				}
				if encrypted.KeyID == "" {
					t.Errorf("Missing key ID in encrypted field for %s", tc.name)
				}
				if encrypted.Version < 1 {
					t.Errorf("Invalid version in encrypted field for %s", tc.name)
				}
			}
		})
	}
}

// TestKeyDerivation_Strategies tests different key derivation approaches
func TestKeyDerivation_Strategies(t *testing.T) {
	masterKey := generateTestMasterKey()
	plaintext := "sensitive@example.com"

	strategies := []struct {
		name     string
		strategy KeyDerivationStrategy
	}{
		{"PerField", PerFieldStrategy},
		{"PerRecord", PerRecordStrategy},
		{"Hybrid", HybridStrategy},
	}

	for _, strat := range strategies {
		t.Run(strat.name, func(t *testing.T) {
			// TDD: This will fail until key derivation strategies are implemented
			encryption, err := NewFieldEncryption(masterKey, strat.strategy)
			if err != nil {
				t.Fatalf("Failed to create encryption with strategy %s: %v", strat.name, err)
			}

			// Test that same plaintext with same field type produces consistent results
			encrypted1, err := encryption.EncryptField(plaintext, "email")
			if err != nil {
				t.Fatalf("First encryption failed: %v", err)
			}

			encrypted2, err := encryption.EncryptField(plaintext, "email")
			if err != nil {
				t.Fatalf("Second encryption failed: %v", err)
			}

			// For per-field strategy, encryptions should use same key (but different IVs)
			if strat.strategy == PerFieldStrategy {
				if encrypted1.KeyID != encrypted2.KeyID {
					t.Errorf("Per-field strategy should use same key ID: %s vs %s", 
						encrypted1.KeyID, encrypted2.KeyID)
				}
			}

			// Verify both can be decrypted correctly
			decrypted1, err := encryption.DecryptField(encrypted1, "email")
			if err != nil || decrypted1 != plaintext {
				t.Errorf("Failed to decrypt first encryption: %v", err)
			}

			decrypted2, err := encryption.DecryptField(encrypted2, "email")
			if err != nil || decrypted2 != plaintext {
				t.Errorf("Failed to decrypt second encryption: %v", err)
			}
		})
	}
}

// TestTakedownRecord_EncryptedSerialization tests encryption of TakedownRecord sensitive fields
func TestTakedownRecord_EncryptedSerialization(t *testing.T) {
	originalRecord := &TakedownRecord{
		DescriptorCID:    "QmTest123",
		TakedownID:       "TD-12345",
		FilePath:         "/test/file.txt",
		RequestorName:    "ACME Corp",
		RequestorEmail:   "legal@acme.com", // SENSITIVE
		CopyrightWork:    "ACME Software v1.0",
		TakedownDate:     time.Now(),
		Status:           "active",
		DMCANoticeHash:   "abc123",
		UploaderID:       "user-456",
		OriginalNotice:   "This is the original DMCA notice content...", // SENSITIVE
		LegalBasis:       "DMCA 512(c)",
		ProcessingNotes:  "Processed automatically",
	}

	t.Run("Encrypt_Sensitive_Fields", func(t *testing.T) {
		// TDD: This will fail until EncryptedTakedownRecord is implemented
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		encryptedRecord, err := EncryptTakedownRecord(originalRecord, encryption)
		if err != nil {
			t.Fatalf("Failed to encrypt takedown record: %v", err)
		}

		// Verify sensitive fields are encrypted
		if encryptedRecord.RequestorEmail == originalRecord.RequestorEmail {
			t.Error("RequestorEmail was not encrypted")
		}
		if encryptedRecord.OriginalNotice == originalRecord.OriginalNotice {
			t.Error("OriginalNotice was not encrypted")
		}

		// Verify non-sensitive fields remain unchanged
		if encryptedRecord.DescriptorCID != originalRecord.DescriptorCID {
			t.Error("Non-sensitive field DescriptorCID was modified")
		}
		if encryptedRecord.RequestorName != originalRecord.RequestorName {
			t.Error("Non-sensitive field RequestorName was modified")
		}

		// Test decryption
		decryptedRecord, err := DecryptTakedownRecord(encryptedRecord, encryption)
		if err != nil {
			t.Fatalf("Failed to decrypt takedown record: %v", err)
		}

		// Verify decrypted sensitive fields match original
		if decryptedRecord.RequestorEmail != originalRecord.RequestorEmail {
			t.Errorf("Decrypted RequestorEmail doesn't match: got %q, want %q",
				decryptedRecord.RequestorEmail, originalRecord.RequestorEmail)
		}
		if decryptedRecord.OriginalNotice != originalRecord.OriginalNotice {
			t.Errorf("Decrypted OriginalNotice doesn't match: got %q, want %q",
				decryptedRecord.OriginalNotice, originalRecord.OriginalNotice)
		}
	})

	t.Run("JSON_Serialization_With_Encryption", func(t *testing.T) {
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		encryptedRecord, err := EncryptTakedownRecord(originalRecord, encryption)
		if err != nil {
			t.Fatalf("Failed to encrypt record: %v", err)
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(encryptedRecord)
		if err != nil {
			t.Fatalf("Failed to marshal encrypted record: %v", err)
		}

		// Verify sensitive data is not in JSON
		jsonString := string(jsonData)
		if strings.Contains(jsonString, originalRecord.RequestorEmail) {
			t.Error("Original email found in JSON output")
		}
		if strings.Contains(jsonString, originalRecord.OriginalNotice) {
			t.Error("Original notice found in JSON output")
		}

		// Test JSON unmarshaling
		var unmarshaledRecord TakedownRecord
		if err := json.Unmarshal(jsonData, &unmarshaledRecord); err != nil {
			t.Fatalf("Failed to unmarshal encrypted record: %v", err)
		}

		// Decrypt and verify
		decryptedRecord, err := DecryptTakedownRecord(&unmarshaledRecord, encryption)
		if err != nil {
			t.Fatalf("Failed to decrypt unmarshaled record: %v", err)
		}

		if !reflect.DeepEqual(decryptedRecord, originalRecord) {
			t.Error("Round-trip serialization produced different result")
		}
	})
}

// TestCounterNotice_EncryptedSerialization tests encryption of CounterNotice sensitive fields
func TestCounterNotice_EncryptedSerialization(t *testing.T) {
	originalNotice := &CounterNotice{
		CounterNoticeID:       "CN-12345",
		UserID:               "user-789",
		UserName:             "John Doe",
		UserEmail:            "john@example.com", // SENSITIVE
		UserAddress:          "456 Oak St, Somewhere, NY 12345", // SENSITIVE
		SwornStatement:       "I swear under penalty of perjury...", // SENSITIVE
		GoodFaithBelief:      "I have a good faith belief...",
		ConsentToJurisdiction: true,
		Signature:            "John Doe",
		SubmissionDate:       time.Now(),
		Status:               "pending",
		ProcessingNotes:      "Under review",
	}

	t.Run("Encrypt_All_Sensitive_Fields", func(t *testing.T) {
		// TDD: This will fail until EncryptedCounterNotice is implemented
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		encryptedNotice, err := EncryptCounterNotice(originalNotice, encryption)
		if err != nil {
			t.Fatalf("Failed to encrypt counter notice: %v", err)
		}

		// Verify all sensitive fields are encrypted
		sensitiveFields := []struct {
			encrypted, original string
			fieldName          string
		}{
			{encryptedNotice.UserEmail, originalNotice.UserEmail, "UserEmail"},
			{encryptedNotice.UserAddress, originalNotice.UserAddress, "UserAddress"},
			{encryptedNotice.SwornStatement, originalNotice.SwornStatement, "SwornStatement"},
		}

		for _, field := range sensitiveFields {
			if field.encrypted == field.original {
				t.Errorf("Sensitive field %s was not encrypted", field.fieldName)
			}
		}

		// Test decryption
		decryptedNotice, err := DecryptCounterNotice(encryptedNotice, encryption)
		if err != nil {
			t.Fatalf("Failed to decrypt counter notice: %v", err)
		}

		// Verify all sensitive fields decrypt correctly
		if decryptedNotice.UserEmail != originalNotice.UserEmail {
			t.Errorf("UserEmail decryption failed: got %q, want %q", 
				decryptedNotice.UserEmail, originalNotice.UserEmail)
		}
		if decryptedNotice.UserAddress != originalNotice.UserAddress {
			t.Errorf("UserAddress decryption failed: got %q, want %q", 
				decryptedNotice.UserAddress, originalNotice.UserAddress)
		}
		if decryptedNotice.SwornStatement != originalNotice.SwornStatement {
			t.Errorf("SwornStatement decryption failed: got %q, want %q", 
				decryptedNotice.SwornStatement, originalNotice.SwornStatement)
		}
	})
}

// TestAuditEntry_DynamicFieldEncryption tests encryption of audit entries with dynamic content
func TestAuditEntry_DynamicFieldEncryption(t *testing.T) {
	originalEntry := &DetailedAuditEntry{
		EntryID:       "AE-12345",
		Timestamp:     time.Now(),
		EventType:     "dmca_takedown",
		UserID:        "user-123",
		TargetID:      "descriptor-456",
		Action:        "blacklist_descriptor",
		ActionDetails: map[string]interface{}{
			"requestor_email": "sensitive@example.com", // SENSITIVE
			"user_ip":        "192.168.1.100",         // SENSITIVE
			"user_agent":     "Mozilla/5.0...",        // SENSITIVE
			"takedown_id":    "TD-789",                // Non-sensitive
			"timestamp":      time.Now().Unix(),       // Non-sensitive
		},
		Result:          "success",
		IPAddress:       "192.168.1.100", // SENSITIVE
		UserAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", // SENSITIVE
		ComplianceNotes: "Automated takedown processing",
	}

	t.Run("Encrypt_Audit_PII_Fields", func(t *testing.T) {
		// TDD: This will fail until EncryptedDetailedAuditEntry is implemented
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		encryptedEntry, err := EncryptDetailedAuditEntry(originalEntry, encryption)
		if err != nil {
			t.Fatalf("Failed to encrypt audit entry: %v", err)
		}

		// Verify PII fields are encrypted
		if encryptedEntry.IPAddress == originalEntry.IPAddress {
			t.Error("IPAddress was not encrypted")
		}
		if encryptedEntry.UserAgent == originalEntry.UserAgent {
			t.Error("UserAgent was not encrypted")
		}

		// Verify ActionDetails sensitive fields are encrypted
		if details, ok := encryptedEntry.ActionDetails["requestor_email"].(string); ok {
			if details == "sensitive@example.com" {
				t.Error("Sensitive email in ActionDetails was not encrypted")
			}
		}

		// Test decryption
		decryptedEntry, err := DecryptDetailedAuditEntry(encryptedEntry, encryption)
		if err != nil {
			t.Fatalf("Failed to decrypt audit entry: %v", err)
		}

		// Verify decryption accuracy
		if decryptedEntry.IPAddress != originalEntry.IPAddress {
			t.Errorf("IPAddress decryption failed: got %q, want %q", 
				decryptedEntry.IPAddress, originalEntry.IPAddress)
		}
		if decryptedEntry.UserAgent != originalEntry.UserAgent {
			t.Errorf("UserAgent decryption failed: got %q, want %q", 
				decryptedEntry.UserAgent, originalEntry.UserAgent)
		}
	})

	t.Run("Dynamic_ActionDetails_Encryption", func(t *testing.T) {
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		// Test with various ActionDetails structures
		testCases := []map[string]interface{}{
			{
				"user_email": "test@example.com",
				"public_id":  "public-123",
			},
			{
				"nested": map[string]interface{}{
					"user_ip":   "10.0.0.1",
					"public_id": "nested-456",
				},
			},
			{
				"array_data": []interface{}{
					"public-item",
					map[string]interface{}{
						"user_phone": "+1-555-0123",
					},
				},
			},
		}

		for i, testCase := range testCases {
			t.Run(fmt.Sprintf("ActionDetails_Case_%d", i), func(t *testing.T) {
				entry := &DetailedAuditEntry{
					EntryID:       fmt.Sprintf("AE-%d", i),
					ActionDetails: testCase,
				}

				encrypted, err := EncryptDetailedAuditEntry(entry, encryption)
				if err != nil {
					t.Fatalf("Failed to encrypt entry with ActionDetails case %d: %v", i, err)
				}

				decrypted, err := DecryptDetailedAuditEntry(encrypted, encryption)
				if err != nil {
					t.Fatalf("Failed to decrypt entry with ActionDetails case %d: %v", i, err)
				}

				if !reflect.DeepEqual(decrypted.ActionDetails, entry.ActionDetails) {
					t.Errorf("ActionDetails case %d: decrypted doesn't match original", i)
				}
			})
		}
	})
}

// TestKeyRotation_ExistingData tests key rotation with existing encrypted data
func TestKeyRotation_ExistingData(t *testing.T) {
	// Create data with old key
	oldKey := generateTestMasterKey()
	oldEncryption, err := NewFieldEncryption(oldKey, PerFieldStrategy)
	if err != nil {
		t.Fatalf("Failed to create old encryption: %v", err)
	}

	originalEmail := "sensitive@example.com"
	encryptedWithOldKey, err := oldEncryption.EncryptField(originalEmail, "email")
	if err != nil {
		t.Fatalf("Failed to encrypt with old key: %v", err)
	}

	// Rotate to new key
	newKey := generateTestMasterKey()
	newEncryption, err := NewFieldEncryption(newKey, PerFieldStrategy)
	if err != nil {
		t.Fatalf("Failed to create new encryption: %v", err)
	}

	t.Run("Old_Key_Data_Inaccessible_With_New_Key", func(t *testing.T) {
		// TDD: This will fail until key rotation handling is implemented
		_, err := newEncryption.DecryptField(encryptedWithOldKey, "email")
		if err == nil {
			t.Error("Expected decryption to fail with wrong key, but it succeeded")
		}
	})

	t.Run("Key_Migration_Process", func(t *testing.T) {
		// TDD: This will fail until key migration is implemented
		keyManager := NewKeyManager()
		
		// Register both keys
		_ = keyManager.RegisterKey(oldKey, 1) // oldKeyID - not used in this test
		newKeyID := keyManager.RegisterKey(newKey, 2)
		
		// Create migration-aware encryption
		migrationEncryption, err := NewMigrationAwareEncryption(keyManager, newKeyID)
		if err != nil {
			t.Fatalf("Failed to create migration encryption: %v", err)
		}

		// Should be able to decrypt old data
		decrypted, err := migrationEncryption.DecryptField(encryptedWithOldKey, "email")
		if err != nil {
			t.Fatalf("Failed to decrypt with migration-aware encryption: %v", err)
		}

		if decrypted != originalEmail {
			t.Errorf("Migration decryption failed: got %q, want %q", decrypted, originalEmail)
		}

		// Re-encrypt with new key
		reencrypted, err := migrationEncryption.EncryptField(originalEmail, "email")
		if err != nil {
			t.Fatalf("Failed to re-encrypt with new key: %v", err)
		}

		// Verify new encryption uses new key
		if reencrypted.KeyID == encryptedWithOldKey.KeyID {
			t.Error("Re-encryption still using old key ID")
		}
		if reencrypted.KeyID != newKeyID {
			t.Errorf("Re-encryption not using new key: got %s, want %s", reencrypted.KeyID, newKeyID)
		}
	})
}

// TestMigration_PlaintextToEncrypted tests migrating existing plaintext data to encrypted
func TestMigration_PlaintextToEncrypted(t *testing.T) {
	// Simulate existing database with plaintext data
	existingRecords := []*TakedownRecord{
		{
			DescriptorCID:  "QmTest1",
			RequestorEmail: "legal1@company.com",
			OriginalNotice: "DMCA notice content 1...",
		},
		{
			DescriptorCID:  "QmTest2", 
			RequestorEmail: "legal2@firm.com",
			OriginalNotice: "DMCA notice content 2...",
		},
	}

	t.Run("Batch_Migration_Process", func(t *testing.T) {
		// TDD: This will fail until migration utilities are implemented
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		migrator := NewEncryptionMigrator(encryption)
		
		// Test batch migration
		migratedRecords, err := migrator.MigrateTakedownRecords(existingRecords)
		if err != nil {
			t.Fatalf("Migration failed: %v", err)
		}

		if len(migratedRecords) != len(existingRecords) {
			t.Errorf("Migration count mismatch: got %d, want %d", 
				len(migratedRecords), len(existingRecords))
		}

		// Verify sensitive fields are encrypted
		for i, migrated := range migratedRecords {
			original := existingRecords[i]
			
			if migrated.RequestorEmail == original.RequestorEmail {
				t.Errorf("Record %d: RequestorEmail not encrypted", i)
			}
			if migrated.OriginalNotice == original.OriginalNotice {
				t.Errorf("Record %d: OriginalNotice not encrypted", i)
			}

			// Verify decryption works
			decrypted, err := DecryptTakedownRecord(migrated, encryption)
			if err != nil {
				t.Errorf("Record %d: Failed to decrypt migrated record: %v", i, err)
				continue
			}

			if decrypted.RequestorEmail != original.RequestorEmail {
				t.Errorf("Record %d: Email migration failed: got %q, want %q",
					i, decrypted.RequestorEmail, original.RequestorEmail)
			}
		}
	})

	t.Run("Incremental_Migration_Progress", func(t *testing.T) {
		encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
		if err != nil {
			t.Fatalf("Failed to create encryption: %v", err)
		}

		migrator := NewEncryptionMigrator(encryption)
		
		// Test incremental migration with progress tracking
		progress := make(chan MigrationProgress, 10)
		ctx := NewMigrationContext(progress)

		go func() {
			defer close(progress)
			err := migrator.MigrateTakedownRecordsIncremental(existingRecords, ctx)
			if err != nil {
				t.Errorf("Incremental migration failed: %v", err)
			}
		}()

		// Track migration progress
		var progressUpdates []MigrationProgress
		for update := range progress {
			progressUpdates = append(progressUpdates, update)
		}

		// Verify progress tracking
		if len(progressUpdates) == 0 {
			t.Error("No progress updates received")
		}

		finalProgress := progressUpdates[len(progressUpdates)-1]
		if finalProgress.Completed != len(existingRecords) {
			t.Errorf("Final progress mismatch: got %d completed, want %d", 
				finalProgress.Completed, len(existingRecords))
		}
	})
}

// TestEncryptionPerformance_Overhead tests encryption performance impact
func TestEncryptionPerformance_Overhead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
	if err != nil {
		t.Fatalf("Failed to create encryption: %v", err)
	}

	testData := []struct {
		name string
		data string
		size int
	}{
		{"Small Email", "user@example.com", 16},
		{"Medium Notice", strings.Repeat("Legal text ", 50), 550},
		{"Large Document", strings.Repeat("Long legal document content ", 500), 14000},
	}

	for _, td := range testData {
		t.Run(fmt.Sprintf("Performance_%s", td.name), func(t *testing.T) {
			metrics := &EncryptionMetrics{}

			// Benchmark encryption
			iterations := 100
			start := time.Now()
			
			var encrypted *EncryptedField
			for i := 0; i < iterations; i++ {
				encrypted, err = encryption.EncryptField(td.data, "test")
				if err != nil {
					t.Fatalf("Encryption failed: %v", err)
				}
			}
			metrics.EncryptionTime = time.Since(start) / time.Duration(iterations)

			// Benchmark decryption
			start = time.Now()
			for i := 0; i < iterations; i++ {
				_, err = encryption.DecryptField(encrypted, "test")
				if err != nil {
					t.Fatalf("Decryption failed: %v", err)
				}
			}
			metrics.DecryptionTime = time.Since(start) / time.Duration(iterations)

			// Calculate size overhead
			encryptedData, _ := json.Marshal(encrypted)
			metrics.SizeOverhead = len(encryptedData) - len(td.data)
			overheadPercent := float64(metrics.SizeOverhead) / float64(len(td.data)) * 100

			t.Logf("%s Performance Metrics:", td.name)
			t.Logf("  Encryption Time: %v", metrics.EncryptionTime)
			t.Logf("  Decryption Time: %v", metrics.DecryptionTime)
			t.Logf("  Size Overhead: %d bytes (%.1f%%)", metrics.SizeOverhead, overheadPercent)

			// Performance assertions
			if metrics.EncryptionTime > 10*time.Millisecond {
				t.Errorf("Encryption too slow: %v > 10ms", metrics.EncryptionTime)
			}
			if metrics.DecryptionTime > 10*time.Millisecond {
				t.Errorf("Decryption too slow: %v > 10ms", metrics.DecryptionTime)
			}
			if overheadPercent > 300 { // Allow up to 300% overhead for small data
				t.Errorf("Size overhead too high: %.1f%% > 300%%", overheadPercent)
			}
		})
	}
}

// TestEncryptionErrors_GracefulHandling tests error scenarios and graceful degradation
func TestEncryptionErrors_GracefulHandling(t *testing.T) {
	testCases := []struct {
		name          string
		setupError    func() (*FieldEncryption, error)
		operation     func(*FieldEncryption) error
		expectedError string
	}{
		{
			name: "Invalid_Master_Key",
			setupError: func() (*FieldEncryption, error) {
				return NewFieldEncryption([]byte("too-short"), PerFieldStrategy)
			},
			operation: func(fe *FieldEncryption) error {
				_, err := fe.EncryptField("test", "email")
				return err
			},
			expectedError: "invalid key size",
		},
		{
			name: "Corrupted_Ciphertext",
			setupError: func() (*FieldEncryption, error) {
				return NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
			},
			operation: func(fe *FieldEncryption) error {
				corrupted := &EncryptedField{
					Ciphertext: "corrupted-data-not-base64!@#",
					KeyID:      "test-key",
					Algorithm:  "AES-GCM",
					Version:    1,
				}
				_, err := fe.DecryptField(corrupted, "email")
				return err
			},
			expectedError: "failed to decode ciphertext",
		},
		{
			name: "Missing_Key_ID",
			setupError: func() (*FieldEncryption, error) {
				return NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
			},
			operation: func(fe *FieldEncryption) error {
				invalidField := &EncryptedField{
					Ciphertext: "dGVzdA==",
					KeyID:      "",
					Algorithm:  "AES-GCM",
					Version:    1,
				}
				_, err := fe.DecryptField(invalidField, "email")
				return err
			},
			expectedError: "missing key ID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// TDD: This will fail until error handling is implemented
			fe, setupErr := tc.setupError()
			
			var err error
			if setupErr != nil {
				err = setupErr
			} else {
				err = tc.operation(fe)
			}

			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tc.expectedError)
			} else if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing %q, got %q", tc.expectedError, err.Error())
			}
		})
	}
}

// TestEncryptionSecurity_AttackResistance tests security properties
func TestEncryptionSecurity_AttackResistance(t *testing.T) {
	encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
	if err != nil {
		t.Fatalf("Failed to create encryption: %v", err)
	}

	t.Run("Ciphertext_Uniqueness", func(t *testing.T) {
		plaintext := "sensitive@example.com"
		
		// Encrypt same plaintext multiple times
		encryptions := make([]*EncryptedField, 10)
		for i := 0; i < 10; i++ {
			encryptions[i], err = encryption.EncryptField(plaintext, "email")
			if err != nil {
				t.Fatalf("Encryption %d failed: %v", i, err)
			}
		}

		// Verify all ciphertexts are unique (due to random IVs)
		seen := make(map[string]bool)
		for i, enc := range encryptions {
			if seen[enc.Ciphertext] {
				t.Errorf("Duplicate ciphertext found at index %d", i)
			}
			seen[enc.Ciphertext] = true
		}
	})

	t.Run("Key_Separation", func(t *testing.T) {
		plaintext := "test@example.com"

		// Encrypt with different field types
		emailEnc, err := encryption.EncryptField(plaintext, "email")
		if err != nil {
			t.Fatalf("Email encryption failed: %v", err)
		}

		addressEnc, err := encryption.EncryptField(plaintext, "address")
		if err != nil {
			t.Fatalf("Address encryption failed: %v", err)
		}

		// For per-field strategy, different field types should use different keys
		if encryption.keyDerivation == PerFieldStrategy {
			if emailEnc.KeyID == addressEnc.KeyID {
				t.Error("Same plaintext in different field types should use different keys")
			}
		}

		// Both should decrypt correctly
		emailDecrypted, err := encryption.DecryptField(emailEnc, "email")
		if err != nil || emailDecrypted != plaintext {
			t.Errorf("Email decryption failed: %v", err)
		}

		addressDecrypted, err := encryption.DecryptField(addressEnc, "address")
		if err != nil || addressDecrypted != plaintext {
			t.Errorf("Address decryption failed: %v", err)
		}
	})

	t.Run("Tampered_Ciphertext_Detection", func(t *testing.T) {
		plaintext := "secure@example.com"
		encrypted, err := encryption.EncryptField(plaintext, "email")
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}

		// Tamper with ciphertext
		originalCiphertext := encrypted.Ciphertext
		tampered := &EncryptedField{
			Ciphertext: corruptBase64(originalCiphertext),
			KeyID:      encrypted.KeyID,
			Algorithm:  encrypted.Algorithm,
			Version:    encrypted.Version,
		}

		// Decryption should fail
		_, err = encryption.DecryptField(tampered, "email")
		if err == nil {
			t.Error("Decryption of tampered ciphertext should fail")
		}
	})
}

// Test stub implementations - these will all fail initially (TDD approach)

func NewFieldEncryption(masterKey []byte, strategy KeyDerivationStrategy) (*FieldEncryption, error) {
	return nil, fmt.Errorf("field encryption not implemented")
}

func (fe *FieldEncryption) EncryptField(plaintext, fieldType string) (*EncryptedField, error) {
	return nil, fmt.Errorf("field encryption not implemented")
}

func (fe *FieldEncryption) DecryptField(encrypted *EncryptedField, fieldType string) (string, error) {
	return "", fmt.Errorf("field decryption not implemented")
}

func EncryptTakedownRecord(record *TakedownRecord, encryption *FieldEncryption) (*TakedownRecord, error) {
	return nil, fmt.Errorf("takedown record encryption not implemented")
}

func DecryptTakedownRecord(record *TakedownRecord, encryption *FieldEncryption) (*TakedownRecord, error) {
	return nil, fmt.Errorf("takedown record decryption not implemented")
}

func EncryptCounterNotice(notice *CounterNotice, encryption *FieldEncryption) (*CounterNotice, error) {
	return nil, fmt.Errorf("counter notice encryption not implemented")
}

func DecryptCounterNotice(notice *CounterNotice, encryption *FieldEncryption) (*CounterNotice, error) {
	return nil, fmt.Errorf("counter notice decryption not implemented")
}

func EncryptDetailedAuditEntry(entry *DetailedAuditEntry, encryption *FieldEncryption) (*DetailedAuditEntry, error) {
	return nil, fmt.Errorf("audit entry encryption not implemented")
}

func DecryptDetailedAuditEntry(entry *DetailedAuditEntry, encryption *FieldEncryption) (*DetailedAuditEntry, error) {
	return nil, fmt.Errorf("audit entry decryption not implemented")
}

// Key management stubs

type KeyManager struct {
	keys map[string][]byte
}

type MigrationProgress struct {
	Total     int
	Completed int
	Errors    int
	Current   string
}

type MigrationContext struct {
	Progress chan<- MigrationProgress
}

type EncryptionMigrator struct {
	encryption *FieldEncryption
}

func NewKeyManager() *KeyManager {
	return &KeyManager{keys: make(map[string][]byte)}
}

func (km *KeyManager) RegisterKey(key []byte, version int) string {
	return fmt.Sprintf("key-v%d", version)
}

func NewMigrationAwareEncryption(keyManager *KeyManager, currentKeyID string) (*FieldEncryption, error) {
	return nil, fmt.Errorf("migration-aware encryption not implemented")
}

func NewEncryptionMigrator(encryption *FieldEncryption) *EncryptionMigrator {
	return &EncryptionMigrator{encryption: encryption}
}

func (em *EncryptionMigrator) MigrateTakedownRecords(records []*TakedownRecord) ([]*TakedownRecord, error) {
	return nil, fmt.Errorf("batch migration not implemented")
}

func (em *EncryptionMigrator) MigrateTakedownRecordsIncremental(records []*TakedownRecord, ctx *MigrationContext) error {
	return fmt.Errorf("incremental migration not implemented")
}

func NewMigrationContext(progress chan<- MigrationProgress) *MigrationContext {
	return &MigrationContext{Progress: progress}
}

// Test helper functions

func generateTestMasterKey() []byte {
	key := make([]byte, 32) // 256-bit key
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		panic(fmt.Sprintf("Failed to generate test key: %v", err))
	}
	return key
}

func corruptBase64(data string) string {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "invalid-base64-data"
	}
	
	// Flip a bit in the middle
	if len(decoded) > 0 {
		decoded[len(decoded)/2] ^= 0x01
	}
	
	return base64.StdEncoding.EncodeToString(decoded)
}

// Benchmark tests for performance validation

func BenchmarkFieldEncryption_Email(b *testing.B) {
	encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
	if err != nil {
		b.Fatalf("Failed to create encryption: %v", err)
	}

	email := "user@example.com"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := encryption.EncryptField(email, "email")
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}
	}
}

func BenchmarkFieldDecryption_Email(b *testing.B) {
	encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
	if err != nil {
		b.Fatalf("Failed to create encryption: %v", err)
	}

	email := "user@example.com"
	encrypted, err := encryption.EncryptField(email, "email")
	if err != nil {
		b.Fatalf("Failed to encrypt test data: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := encryption.DecryptField(encrypted, "email")
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
	}
}

func BenchmarkTakedownRecord_EncryptionRoundtrip(b *testing.B) {
	encryption, err := NewFieldEncryption(generateTestMasterKey(), PerFieldStrategy)
	if err != nil {
		b.Fatalf("Failed to create encryption: %v", err)
	}

	record := &TakedownRecord{
		DescriptorCID:  "QmBenchmark",
		RequestorEmail: "benchmark@example.com",
		OriginalNotice: strings.Repeat("Legal notice content ", 50),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		encrypted, err := EncryptTakedownRecord(record, encryption)
		if err != nil {
			b.Fatalf("Encryption failed: %v", err)
		}

		_, err = DecryptTakedownRecord(encrypted, encryption)
		if err != nil {
			b.Fatalf("Decryption failed: %v", err)
		}
	}
}