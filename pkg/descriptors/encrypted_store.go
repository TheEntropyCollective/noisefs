package descriptors

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/TheEntropyCollective/noisefs/pkg/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
)

// EncryptedDescriptor represents an encrypted descriptor stored in IPFS
type EncryptedDescriptor struct {
	Version     string `json:"version"`
	Salt        []byte `json:"salt"`
	Ciphertext  []byte `json:"ciphertext"`
	IsEncrypted bool   `json:"is_encrypted"`
}

// EncryptedStore handles encrypted descriptor storage and retrieval
type EncryptedStore struct {
	ipfsClient *ipfs.Client
	password   string
}

// NewEncryptedStore creates a new encrypted descriptor store
func NewEncryptedStore(ipfsClient *ipfs.Client, password string) (*EncryptedStore, error) {
	if ipfsClient == nil {
		return nil, errors.New("IPFS client is required")
	}
	
	return &EncryptedStore{
		ipfsClient: ipfsClient,
		password:   password,
	}, nil
}

// Save stores a descriptor in IPFS with encryption
func (s *EncryptedStore) Save(descriptor *Descriptor) (string, error) {
	if descriptor == nil {
		return "", errors.New("descriptor cannot be nil")
	}

	var data []byte
	var err error

	if s.password != "" {
		// Encrypt the descriptor
		data, err = s.encryptDescriptor(descriptor)
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

	// Store in IPFS
	reader := bytes.NewReader(data)
	cid, err := s.ipfsClient.Add(reader)
	if err != nil {
		return "", fmt.Errorf("failed to store encrypted descriptor: %w", err)
	}

	return cid, nil
}

// Load retrieves and decrypts a descriptor from IPFS by its CID
func (s *EncryptedStore) Load(cid string) (*Descriptor, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}

	// Retrieve from IPFS
	reader, err := s.ipfsClient.Cat(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}
	defer reader.Close()

	// Read data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read descriptor data: %w", err)
	}

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
	// Temporarily disable password for this save
	oldPassword := s.password
	s.password = ""
	defer func() { s.password = oldPassword }()

	return s.Save(descriptor)
}

// encryptDescriptor encrypts a descriptor
func (s *EncryptedStore) encryptDescriptor(descriptor *Descriptor) ([]byte, error) {
	// Generate encryption key from password
	encKey, err := crypto.GenerateKey(s.password)
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
	if s.password == "" {
		return nil, errors.New("password required to decrypt descriptor")
	}

	// Derive encryption key from password and salt
	encKey, err := crypto.DeriveKey(s.password, encDesc.Salt)
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

	// Retrieve from IPFS
	reader, err := s.ipfsClient.Cat(cid)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve descriptor: %w", err)
	}
	defer reader.Close()

	// Read data
	data, err := io.ReadAll(reader)
	if err != nil {
		return false, fmt.Errorf("failed to read descriptor data: %w", err)
	}

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