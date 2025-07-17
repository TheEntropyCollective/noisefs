package crypto

import (
	"encoding/json"
	"testing"
)

// TestEncryptionKeyString tests the String method of EncryptionKey
func TestEncryptionKeyString(t *testing.T) {
	key, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	keyStr := key.String()
	if keyStr == "" {
		t.Error("Key string should not be empty")
	}
	
	// Test that multiple calls return the same string
	keyStr2 := key.String()
	if keyStr != keyStr2 {
		t.Error("Multiple calls to String() should return the same value")
	}
}

// TestParseKeyFromString tests the ParseKeyFromString function
func TestParseKeyFromString(t *testing.T) {
	// Generate original key
	original, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	// Serialize to string
	keyStr := original.String()
	
	// Parse back
	parsed, err := ParseKeyFromString(keyStr)
	if err != nil {
		t.Fatalf("Failed to parse key string: %v", err)
	}
	
	// Verify parsed key matches original
	if len(parsed.Key) != len(original.Key) {
		t.Errorf("Expected key length %d, got %d", len(original.Key), len(parsed.Key))
	}
	
	for i, b := range original.Key {
		if parsed.Key[i] != b {
			t.Errorf("Key byte %d: expected %d, got %d", i, b, parsed.Key[i])
		}
	}
	
	if len(parsed.Salt) != len(original.Salt) {
		t.Errorf("Expected salt length %d, got %d", len(original.Salt), len(parsed.Salt))
	}
	
	for i, b := range original.Salt {
		if parsed.Salt[i] != b {
			t.Errorf("Salt byte %d: expected %d, got %d", i, b, parsed.Salt[i])
		}
	}
}

// TestParseKeyFromStringErrors tests error conditions for ParseKeyFromString
func TestParseKeyFromStringErrors(t *testing.T) {
	// Test empty string
	_, err := ParseKeyFromString("")
	if err == nil {
		t.Error("Expected error for empty string")
	}
	
	// Test invalid base64
	_, err = ParseKeyFromString("invalid-base64-string!!!")
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
	
	// Test invalid JSON
	_, err = ParseKeyFromString("dGVzdA==") // "test" in base64
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestEncryptionKeyMarshalText tests the MarshalText method
func TestEncryptionKeyMarshalText(t *testing.T) {
	key, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	text, err := key.MarshalText()
	if err != nil {
		t.Fatalf("Failed to marshal text: %v", err)
	}
	
	if len(text) == 0 {
		t.Error("Marshaled text should not be empty")
	}
	
	// Should be the same as String()
	if string(text) != key.String() {
		t.Error("MarshalText should return the same as String()")
	}
}

// TestEncryptionKeyUnmarshalText tests the UnmarshalText method
func TestEncryptionKeyUnmarshalText(t *testing.T) {
	// Generate original key
	original, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	// Marshal to text
	text, err := original.MarshalText()
	if err != nil {
		t.Fatalf("Failed to marshal text: %v", err)
	}
	
	// Create new key and unmarshal
	var restored EncryptionKey
	err = restored.UnmarshalText(text)
	if err != nil {
		t.Fatalf("Failed to unmarshal text: %v", err)
	}
	
	// Verify restored key matches original
	if len(restored.Key) != len(original.Key) {
		t.Errorf("Expected key length %d, got %d", len(original.Key), len(restored.Key))
	}
	
	for i, b := range original.Key {
		if restored.Key[i] != b {
			t.Errorf("Key byte %d: expected %d, got %d", i, b, restored.Key[i])
		}
	}
	
	if len(restored.Salt) != len(original.Salt) {
		t.Errorf("Expected salt length %d, got %d", len(original.Salt), len(restored.Salt))
	}
	
	for i, b := range original.Salt {
		if restored.Salt[i] != b {
			t.Errorf("Salt byte %d: expected %d, got %d", i, b, restored.Salt[i])
		}
	}
}

// TestEncryptionKeyJSONSerialization tests JSON serialization/deserialization
func TestEncryptionKeyJSONSerialization(t *testing.T) {
	// Generate original key
	original, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	// Test JSON marshaling
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	
	// Test JSON unmarshaling
	var restored EncryptionKey
	err = json.Unmarshal(jsonData, &restored)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	
	// Verify restored key matches original
	if len(restored.Key) != len(original.Key) {
		t.Errorf("Expected key length %d, got %d", len(original.Key), len(restored.Key))
	}
	
	for i, b := range original.Key {
		if restored.Key[i] != b {
			t.Errorf("Key byte %d: expected %d, got %d", i, b, restored.Key[i])
		}
	}
	
	if len(restored.Salt) != len(original.Salt) {
		t.Errorf("Expected salt length %d, got %d", len(original.Salt), len(restored.Salt))
	}
	
	for i, b := range original.Salt {
		if restored.Salt[i] != b {
			t.Errorf("Salt byte %d: expected %d, got %d", i, b, restored.Salt[i])
		}
	}
}

// TestKeySerializationUniqueness tests that different keys produce different serializations
func TestKeySerializationUniqueness(t *testing.T) {
	// Generate multiple keys
	keys := make([]*EncryptionKey, 5)
	serializations := make(map[string]bool)
	
	for i := 0; i < 5; i++ {
		key, err := GenerateKey("test-password")
		if err != nil {
			t.Fatalf("Failed to generate key %d: %v", i, err)
		}
		keys[i] = key
		
		keyStr := key.String()
		if serializations[keyStr] {
			t.Errorf("Duplicate serialization for key %d", i)
		}
		serializations[keyStr] = true
	}
	
	// Verify all keys are different
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			if keys[i].String() == keys[j].String() {
				t.Errorf("Keys %d and %d have the same serialization", i, j)
			}
		}
	}
}

// TestKeySerializationRoundTrip tests multiple serialization/deserialization cycles
func TestKeySerializationRoundTrip(t *testing.T) {
	// Generate original key
	original, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	current := original
	
	// Perform multiple round trips
	for i := 0; i < 10; i++ {
		// Serialize
		keyStr := current.String()
		
		// Deserialize
		parsed, err := ParseKeyFromString(keyStr)
		if err != nil {
			t.Fatalf("Failed to parse key on round trip %d: %v", i, err)
		}
		
		// Verify it matches
		if len(parsed.Key) != len(original.Key) {
			t.Errorf("Round trip %d: expected key length %d, got %d", i, len(original.Key), len(parsed.Key))
		}
		
		for j, b := range original.Key {
			if parsed.Key[j] != b {
				t.Errorf("Round trip %d, key byte %d: expected %d, got %d", i, j, b, parsed.Key[j])
			}
		}
		
		current = parsed
	}
}

// TestEncryptDecryptWithSerializedKey tests that serialized keys work for encryption/decryption
func TestEncryptDecryptWithSerializedKey(t *testing.T) {
	// Generate original key
	original, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	
	// Test data
	testData := []byte("This is test data for encryption/decryption")
	
	// Encrypt with original key
	encrypted, err := Encrypt(testData, original)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}
	
	// Serialize and deserialize key
	keyStr := original.String()
	restoredKey, err := ParseKeyFromString(keyStr)
	if err != nil {
		t.Fatalf("Failed to restore key: %v", err)
	}
	
	// Decrypt with restored key
	decrypted, err := Decrypt(encrypted, restoredKey)
	if err != nil {
		t.Fatalf("Failed to decrypt with restored key: %v", err)
	}
	
	// Verify decrypted data matches original
	if len(decrypted) != len(testData) {
		t.Errorf("Expected decrypted length %d, got %d", len(testData), len(decrypted))
	}
	
	for i, b := range testData {
		if decrypted[i] != b {
			t.Errorf("Decrypted byte %d: expected %d, got %d", i, b, decrypted[i])
		}
	}
}