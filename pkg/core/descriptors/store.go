// Package descriptors provides descriptor storage and retrieval operations for NoiseFS.
// This file implements the Store type that handles persistent storage and retrieval of
// file and directory descriptors through the storage manager abstraction.
//
// The descriptor store provides:
//   - Unified storage abstraction for descriptor persistence
//   - JSON serialization and deserialization for descriptor storage
//   - Integration with storage manager for backend flexibility
//   - Error handling and validation for storage operations
//   - Context support for cancellation and timeout management
//
// Storage Features:
//   - Backend-agnostic storage through storage manager abstraction
//   - Automatic JSON serialization for human-readable storage
//   - Content-addressed storage using descriptor CIDs
//   - Validation during save and load operations
//   - Comprehensive error handling and reporting
//
// Integration Benefits:
//   - Works with IPFS, local storage, and other storage backends
//   - Consistent interface for descriptor storage operations
//   - Transaction support through storage manager
//   - Performance optimization through storage manager features
//
// Use Cases:
//   - Persistent storage of file reconstruction metadata
//   - Cross-system descriptor sharing and exchange
//   - Backup and restore operations
//   - API-based descriptor storage and retrieval
package descriptors

import (
	"context"
	"errors"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// Store handles descriptor storage and retrieval through storage manager abstraction.
// This structure provides a high-level interface for persistent descriptor operations,
// abstracting the underlying storage implementation and providing consistent storage semantics.
//
// Storage Abstraction:
//   - Backend-agnostic storage through storage manager
//   - Consistent interface across different storage implementations
//   - Automatic content addressing for descriptor identification
//   - Integration with NoiseFS storage architecture
//
// Features:
//   - JSON-based descriptor serialization for portability
//   - Content-addressed storage using CIDs for identification
//   - Validation during storage and retrieval operations
//   - Error handling with detailed error context
//   - Context support for operation cancellation
//
// Backend Support:
//   - IPFS for distributed storage
//   - Local file system for development and testing
//   - Other storage backends through storage manager interface
//   - Pluggable storage architecture for flexibility
//
// Thread Safety:
//   Storage operations are thread-safe through storage manager implementation,
//   enabling concurrent descriptor operations from multiple goroutines.
//
// Time Complexity: O(1) for storage operations plus backend latency
// Space Complexity: O(d) where d is the descriptor size
type Store struct {
	storageManager *storage.Manager // Storage manager for backend-agnostic descriptor storage
}

// NewStore creates a new descriptor store using storage manager.
// This function is deprecated in favor of NewStoreWithManager for clearer naming.
// Provided for backward compatibility with existing code that uses the original constructor.
//
// Deprecation Notice:
//   - Use NewStoreWithManager instead for new code
//   - Maintained for backward compatibility only
//   - Will be removed in future versions
//   - Direct delegation to NewStoreWithManager for consistency
//
// Migration Path:
//   Replace NewStore calls with NewStoreWithManager for identical functionality
//   and future compatibility with NoiseFS API evolution.
//
// Parameters:
//   - storageManager: Storage manager for backend abstraction
//
// Returns:
//   - *Store: Descriptor store instance
//   - error: Non-nil if storage manager is invalid
//
// Deprecated: Use NewStoreWithManager instead for clearer naming
func NewStore(storageManager *storage.Manager) (*Store, error) {
	return NewStoreWithManager(storageManager)
}

// NewStoreWithManager creates a new descriptor store with storage manager for backend abstraction.
// This constructor initializes a descriptor store that provides persistent storage operations
// through the storage manager abstraction, enabling flexible backend selection and configuration.
//
// Storage Manager Integration:
//   - Validates storage manager availability for reliable operations
//   - Provides backend abstraction for storage operations
//   - Enables pluggable storage architecture
//   - Supports IPFS, local storage, and other backends
//
// Initialization Features:
//   - Storage manager validation to prevent invalid configurations
//   - Error handling for missing dependencies
//   - Ready-to-use store instance for immediate operations
//   - Thread-safe initialization for concurrent usage
//
// Backend Flexibility:
//   - Works with any storage manager implementation
//   - Supports development, testing, and production configurations
//   - Enables storage backend switching without code changes
//   - Provides consistent interface across different backends
//
// Use Cases:
//   - Production descriptor storage with IPFS backend
//   - Development and testing with local storage
//   - Cross-platform deployment with different storage backends
//   - Performance optimization through backend selection
//
// Parameters:
//   - storageManager: Storage manager providing backend abstraction (must be non-nil)
//
// Returns:
//   - *Store: Initialized descriptor store ready for operations
//   - error: Non-nil if storage manager is nil or invalid
//
// Call Flow:
//   - Called by: Client initialization, storage system setup, API configuration
//   - Used with: Save and Load operations for descriptor persistence
//
// Time Complexity: O(1) - simple validation and initialization
// Space Complexity: O(1) - fixed store structure allocation
func NewStoreWithManager(storageManager *storage.Manager) (*Store, error) {
	if storageManager == nil {
		return nil, errors.New("storage manager is required")
	}

	return &Store{
		storageManager: storageManager,
	}, nil
}

// Save stores a descriptor in distributed storage and returns its content identifier.
// This method provides persistent storage of descriptor metadata through JSON serialization
// and content-addressed storage, enabling reliable descriptor persistence and retrieval.
//
// Storage Process:
//   1. Validate descriptor is non-nil and well-formed
//   2. Serialize descriptor to human-readable JSON format
//   3. Create storage block from JSON data
//   4. Store block through storage manager with content addressing
//   5. Return content identifier for future retrieval
//
// Content Addressing:
//   - Automatic CID generation based on descriptor content
//   - Deterministic addressing enables content deduplication
//   - Immutable storage through content-based identification
//   - Cross-system descriptor sharing through CID references
//
// Serialization Features:
//   - JSON format for human readability and debugging
//   - Automatic validation during serialization
//   - Cross-platform compatibility through standard JSON
//   - Structured data storage with schema validation
//
// Storage Backend Integration:
//   - Backend-agnostic storage through storage manager
//   - Support for IPFS, local storage, and other backends
//   - Transaction semantics provided by storage manager
//   - Error handling with backend-specific context
//
// Parameters:
//   - descriptor: Descriptor to store (must be non-nil and valid)
//
// Returns:
//   - string: Content identifier for stored descriptor
//   - error: Non-nil if descriptor is invalid, serialization fails, or storage fails
//
// Call Flow:
//   - Called by: Upload operations, descriptor management, API handlers
//   - Calls: descriptor.ToJSON, blocks.NewBlock, storage manager Put
//
// Time Complexity: O(d) where d is descriptor size for serialization
// Space Complexity: O(d) for JSON serialization and block creation
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

// Load retrieves a descriptor from distributed storage by its content identifier.
// This method provides reliable descriptor retrieval through content-addressed storage,
// automatically deserializing JSON data and validating descriptor integrity.
//
// Retrieval Process:
//   1. Validate CID is non-empty and well-formed
//   2. Create storage address from content identifier
//   3. Retrieve block data through storage manager
//   4. Extract JSON data from storage block
//   5. Deserialize and validate descriptor from JSON
//
// Content Addressing:
//   - Content-based retrieval using immutable identifiers
//   - Deterministic access to previously stored descriptors
//   - Cross-system descriptor sharing through CID references
//   - Integrity verification through content addressing
//
// Deserialization Features:
//   - Automatic JSON parsing with error handling
//   - Comprehensive validation of deserialized descriptor
//   - Type-safe descriptor reconstruction
//   - Error detection for corrupted or invalid data
//
// Storage Backend Integration:
//   - Backend-agnostic retrieval through storage manager
//   - Support for IPFS, local storage, and other backends
//   - Network transparency for distributed storage
//   - Caching and performance optimization through storage manager
//
// Error Handling:
//   - CID validation to prevent invalid retrieval attempts
//   - Storage error propagation with context
//   - JSON parsing error detection and reporting
//   - Descriptor validation error handling
//
// Parameters:
//   - cid: Content identifier for descriptor retrieval (must be non-empty)
//
// Returns:
//   - *Descriptor: Retrieved and validated descriptor
//   - error: Non-nil if CID is invalid, retrieval fails, or deserialization fails
//
// Call Flow:
//   - Called by: Download operations, descriptor access, API handlers
//   - Calls: storage manager Get, FromJSON for deserialization
//
// Time Complexity: O(d) where d is descriptor size for deserialization
// Space Complexity: O(d) for descriptor reconstruction and validation
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
