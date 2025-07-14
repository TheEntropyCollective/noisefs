package descriptors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// EncryptedDescriptor represents an encrypted descriptor stored in IPFS
type EncryptedDescriptor struct {
	Version     string `json:"version"`
	Salt        []byte `json:"salt"`
	Ciphertext  []byte `json:"ciphertext"`
	IsEncrypted bool   `json:"is_encrypted"`
}

// PasswordProvider is a function that provides the password when needed
// The returned password should be cleared from memory after use
// 
// Example of secure usage:
//   provider := func() (string, error) {
//       // Prompt user for password (e.g., from terminal, secure dialog, etc.)
//       password := getPasswordFromUser()
//       return password, nil
//   }
//   store, err := NewEncryptedStore(ipfsClient, provider)
type PasswordProvider func() (string, error)

// EncryptedStore handles encrypted descriptor storage and retrieval
type EncryptedStore struct {
	storageManager   *storage.Manager
	passwordProvider PasswordProvider
}

// NewEncryptedStore creates a new encrypted descriptor store
func NewEncryptedStore(storageManager *storage.Manager, passwordProvider PasswordProvider) (*EncryptedStore, error) {
	if storageManager == nil {
		return nil, errors.New("storage manager is required")
	}
	
	return &EncryptedStore{
		storageManager:   storageManager,
		passwordProvider: passwordProvider,
	}, nil
}

// NewEncryptedStoreWithPassword creates a new encrypted descriptor store with a static password
// WARNING: This is a convenience function. For better security, use NewEncryptedStore with a custom PasswordProvider
func NewEncryptedStoreWithPassword(storageManager *storage.Manager, password string) (*EncryptedStore, error) {
	// Create a copy of the password to avoid external modifications
	passwordCopy := password
	
	// Create a password provider that returns the password
	provider := func() (string, error) {
		if passwordCopy == "" {
			return "", nil
		}
		return passwordCopy, nil
	}
	
	return NewEncryptedStore(storageManager, provider)
}

// Save stores a descriptor in IPFS with encryption
func (s *EncryptedStore) Save(descriptor *Descriptor) (string, error) {
	if descriptor == nil {
		return "", errors.New("descriptor cannot be nil")
	}

	var data []byte
	var err error

	// Get password from provider
	password, err := s.passwordProvider()
	if err != nil {
		return "", fmt.Errorf("failed to get password: %w", err)
	}
	
	// Convert password to byte slice for SecureZero
	passwordBytes := []byte(password)
	defer crypto.SecureZero(passwordBytes)

	if password != "" {
		// Encrypt the descriptor
		data, err = s.encryptDescriptor(descriptor, password)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt descriptor: %w", err)
		}
	} else {
		// Store unencrypted (for public content)
		plainDescriptor := &EncryptedDescriptor{
			Version:     "3.0", // New version for encrypted descriptor format
			Salt:        nil,
			Ciphertext:  nil,
			IsEncrypted: false,
		}

		// Serialize original descriptor and embed it
		origData, err := descriptor.ToJSON()
		if err != nil {
			return "", fmt.Errorf("failed to serialize descriptor: %w", err)
		}
		plainDescriptor.Ciphertext = origData

		data, err = json.MarshalIndent(plainDescriptor, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to serialize plain descriptor: %w", err)
		}
	}

	// Store in storage manager
	block, err := blocks.NewBlock(data)
	if err != nil {
		return "", fmt.Errorf("failed to create block: %w", err)
	}
	
	address, err := s.storageManager.Put(context.Background(), block)
	if err != nil {
		return "", fmt.Errorf("failed to store encrypted descriptor: %w", err)
	}
	
	cid := address.ID

	return cid, nil
}

// Load retrieves and decrypts a descriptor from IPFS by its CID
func (s *EncryptedStore) Load(cid string) (*Descriptor, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}

	// Retrieve from storage manager
	address := &storage.BlockAddress{ID: cid}
	block, err := s.storageManager.Get(context.Background(), address)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}

	data := block.Data

	// Try to parse as encrypted descriptor first
	var encDesc EncryptedDescriptor
	if err := json.Unmarshal(data, &encDesc); err == nil {
		// This is a new format encrypted descriptor
		if encDesc.Version == "3.0" {
			if encDesc.IsEncrypted {
				// Decrypt the descriptor
				return s.decryptDescriptor(&encDesc)
			} else {
				// Unencrypted descriptor in new format
				return FromJSON(encDesc.Ciphertext)
			}
		}
	}

	// Fallback: try to parse as legacy unencrypted descriptor
	descriptor, err := FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse descriptor (tried both encrypted and legacy formats): %w", err)
	}

	return descriptor, nil
}

// SaveUnencrypted stores a descriptor without encryption (for public content)
func (s *EncryptedStore) SaveUnencrypted(descriptor *Descriptor) (string, error) {
	// Temporarily use a password provider that returns empty password
	oldProvider := s.passwordProvider
	s.passwordProvider = func() (string, error) { return "", nil }
	defer func() { s.passwordProvider = oldProvider }()

	return s.Save(descriptor)
}

// encryptDescriptor encrypts a descriptor
func (s *EncryptedStore) encryptDescriptor(descriptor *Descriptor, password string) ([]byte, error) {
	// Generate encryption key from password
	encKey, err := crypto.GenerateKey(password)
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Serialize descriptor to JSON
	plaintext, err := descriptor.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize descriptor: %w", err)
	}

	// Encrypt the descriptor
	ciphertext, err := crypto.Encrypt(plaintext, encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt descriptor: %w", err)
	}

	// Create encrypted descriptor wrapper
	encDesc := &EncryptedDescriptor{
		Version:     "3.0",
		Salt:        encKey.Salt,
		Ciphertext:  ciphertext,
		IsEncrypted: true,
	}

	// Serialize encrypted descriptor
	data, err := json.MarshalIndent(encDesc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize encrypted descriptor: %w", err)
	}

	// Clear sensitive data
	crypto.SecureZero(encKey.Key)
	crypto.SecureZero(plaintext)

	return data, nil
}

// decryptDescriptor decrypts an encrypted descriptor
func (s *EncryptedStore) decryptDescriptor(encDesc *EncryptedDescriptor) (*Descriptor, error) {
	// Get password from provider
	password, err := s.passwordProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to get password: %w", err)
	}
	
	// Convert password to byte slice for SecureZero
	passwordBytes := []byte(password)
	defer crypto.SecureZero(passwordBytes)
	
	if password == "" {
		return nil, errors.New("password required to decrypt descriptor")
	}

	// Derive encryption key from password and salt
	encKey, err := crypto.DeriveKey(password, encDesc.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive encryption key: %w", err)
	}

	// Decrypt the descriptor
	plaintext, err := crypto.Decrypt(encDesc.Ciphertext, encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt descriptor (wrong password?): %w", err)
	}

	// Parse decrypted descriptor
	descriptor, err := FromJSON(plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decrypted descriptor: %w", err)
	}

	// Clear sensitive data
	crypto.SecureZero(encKey.Key)
	crypto.SecureZero(plaintext)

	return descriptor, nil
}

// IsEncrypted checks if a descriptor CID points to an encrypted descriptor
func (s *EncryptedStore) IsEncrypted(cid string) (bool, error) {
	if cid == "" {
		return false, errors.New("CID cannot be empty")
	}

	// Retrieve from storage manager
	address := &storage.BlockAddress{ID: cid}
	block, err := s.storageManager.Get(context.Background(), address)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}

	data := block.Data

	// Try to parse as encrypted descriptor
	var encDesc EncryptedDescriptor
	if err := json.Unmarshal(data, &encDesc); err == nil {
		if encDesc.Version == "3.0" {
			return encDesc.IsEncrypted, nil
		}
	}

	// Legacy format is always unencrypted
	return false, nil
}