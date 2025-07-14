package descriptors

import (
	"context"
	"errors"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// Store handles descriptor storage and retrieval
type Store struct {
	storageManager *storage.Manager
}

// NewStore creates a new descriptor store using storage manager
// This function is deprecated, use NewStoreWithManager instead
func NewStore(storageManager *storage.Manager) (*Store, error) {
	return NewStoreWithManager(storageManager)
}

// NewStoreWithManager creates a new descriptor store with storage manager
func NewStoreWithManager(storageManager *storage.Manager) (*Store, error) {
	if storageManager == nil {
		return nil, errors.New("storage manager is required")
	}
	
	return &Store{
		storageManager: storageManager,
	}, nil
}

// Save stores a descriptor in IPFS and returns its CID
func (s *Store) Save(descriptor *Descriptor) (string, error) {
	if descriptor == nil {
		return "", errors.New("descriptor cannot be nil")
	}
	
	// Serialize descriptor to JSON
	data, err := descriptor.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to serialize descriptor: %w", err)
	}
	
	// Store in storage manager
	block, err := blocks.NewBlock(data)
	if err != nil {
		return "", fmt.Errorf("failed to create block: %w", err)
	}
	
	address, err := s.storageManager.Put(context.Background(), block)
	if err != nil {
		return "", fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	return address.ID, nil
}

// Load retrieves a descriptor from IPFS by its CID
func (s *Store) Load(cid string) (*Descriptor, error) {
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
	
	// Deserialize descriptor
	descriptor, err := FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize descriptor: %w", err)
	}
	
	return descriptor, nil
}