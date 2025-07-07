package descriptors

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
)

// Store handles descriptor storage and retrieval
type Store struct {
	ipfsClient *ipfs.Client
}

// NewStore creates a new descriptor store
func NewStore(ipfsClient *ipfs.Client) (*Store, error) {
	if ipfsClient == nil {
		return nil, errors.New("IPFS client is required")
	}
	
	return &Store{
		ipfsClient: ipfsClient,
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
	
	// Store in IPFS
	reader := bytes.NewReader(data)
	cid, err := s.ipfsClient.Add(reader)
	if err != nil {
		return "", fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	return cid, nil
}

// Load retrieves a descriptor from IPFS by its CID
func (s *Store) Load(cid string) (*Descriptor, error) {
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
	
	// Deserialize descriptor
	descriptor, err := FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize descriptor: %w", err)
	}
	
	return descriptor, nil
}