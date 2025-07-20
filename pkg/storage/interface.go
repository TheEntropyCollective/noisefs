package storage

import (
	"context"
	"io"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Backend defines the interface that all storage backends must implement
type Backend interface {
	// Core operations
	Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error)
	Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error)
	Has(ctx context.Context, address *BlockAddress) (bool, error)
	Delete(ctx context.Context, address *BlockAddress) error
	
	// Batch operations for efficiency
	PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error)
	GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error)
	
	// Pinning operations (for backends that support it)
	Pin(ctx context.Context, address *BlockAddress) error
	Unpin(ctx context.Context, address *BlockAddress) error
	
	// Metadata and health
	GetBackendInfo() *BackendInfo
	HealthCheck(ctx context.Context) *HealthStatus
	
	// Connection management
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
}

// StreamingBackend extends Backend with streaming capabilities
type StreamingBackend interface {
	Backend
	
	// Stream operations for large data
	PutStream(ctx context.Context, reader io.Reader, size int64) (*BlockAddress, error)
	GetStream(ctx context.Context, address *BlockAddress) (io.ReadCloser, error)
}

// PeerAwareBackend extends Backend with peer-aware operations
type PeerAwareBackend interface {
	Backend
	
	// Peer operations
	GetWithPeerHint(ctx context.Context, address *BlockAddress, peers []string) (*blocks.Block, error)
	BroadcastToNetwork(ctx context.Context, address *BlockAddress, block *blocks.Block) error
	GetConnectedPeers() []string
	
	// Peer manager integration
	SetPeerManager(manager interface{}) error
}

// BlockAddress represents a provider-agnostic block address
type BlockAddress struct {
	// Unique identifier for the block (CID, hash, etc.)
	ID string `json:"id"`
	
	// Backend-specific addressing information
	BackendType string                 `json:"backend_type"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	
	// Size and verification info
	Size     int64  `json:"size,omitempty"`
	Checksum string `json:"checksum,omitempty"`
	
	// Storage location hints
	Providers []string `json:"providers,omitempty"`
	
	// Storage tracking for metrics
	WasNewlyStored bool `json:"was_newly_stored,omitempty"` // true if this was a new storage operation, false if block already existed
	
	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at,omitempty"`
}

// BackendInfo provides information about a storage backend
type BackendInfo struct {
	// Backend identification
	Name        string `json:"name"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	
	// Capabilities
	Capabilities []string `json:"capabilities"`
	
	// Configuration
	Config map[string]interface{} `json:"config,omitempty"`
	
	// Network information
	NetworkID string   `json:"network_id,omitempty"`
	Peers     []string `json:"peers,omitempty"`
}

// HealthStatus represents the health status of a storage backend
type HealthStatus struct {
	// Overall health
	Healthy bool   `json:"healthy"`
	Status  string `json:"status"` // "healthy", "degraded", "unhealthy", "offline"
	
	// Performance metrics
	Latency    time.Duration `json:"latency"`
	Throughput float64       `json:"throughput"` // bytes per second
	ErrorRate  float64       `json:"error_rate"`  // percentage
	
	// Capacity information
	UsedStorage      int64 `json:"used_storage"`
	AvailableStorage int64 `json:"available_storage"`
	
	// Network status
	ConnectedPeers int    `json:"connected_peers"`
	NetworkHealth  string `json:"network_health"`
	
	// Last check
	LastCheck time.Time `json:"last_check"`
	
	// Issues
	Issues []HealthIssue `json:"issues,omitempty"`
}

// HealthIssue represents a specific health issue
type HealthIssue struct {
	Severity    string    `json:"severity"` // "warning", "error", "critical"
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// StorageError represents errors from storage operations
type StorageError struct {
	Code        string                 `json:"code"`
	Message     string                 `json:"message"`
	BackendType string                 `json:"backend_type"`
	Address     *BlockAddress          `json:"address,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Cause       error                  `json:"-"`
}

func (e *StorageError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *StorageError) Unwrap() error {
	return e.Cause
}

// Common error codes
const (
	ErrCodeNotFound        = "NOT_FOUND"
	ErrCodeAlreadyExists   = "ALREADY_EXISTS"
	ErrCodeInvalidAddress  = "INVALID_ADDRESS"
	ErrCodeConnectionFailed = "CONNECTION_FAILED"
	ErrCodeTimeout         = "TIMEOUT"
	ErrCodeQuotaExceeded   = "QUOTA_EXCEEDED"
	ErrCodeIntegrityFailure = "INTEGRITY_FAILURE"
	ErrCodeUnauthorized    = "UNAUTHORIZED"
	ErrCodeBackendOffline  = "BACKEND_OFFLINE"
	ErrCodeInvalidConfig   = "INVALID_CONFIG"
	ErrCodeBackendInit     = "BACKEND_INIT_FAILED"
	ErrCodeManagerNotStarted = "MANAGER_NOT_STARTED"
	ErrCodeNoBackends      = "NO_BACKENDS_AVAILABLE"
	ErrCodeValidationFailed = "VALIDATION_FAILED"
)

// Helper functions for creating storage errors
func NewStorageError(code, message, backendType string, cause error) *StorageError {
	return &StorageError{
		Code:        code,
		Message:     message,
		BackendType: backendType,
		Cause:       cause,
		Metadata:    make(map[string]interface{}),
	}
}

func NewNotFoundError(backendType string, address *BlockAddress) *StorageError {
	return &StorageError{
		Code:        ErrCodeNotFound,
		Message:     "block not found",
		BackendType: backendType,
		Address:     address,
	}
}

func NewConnectionError(backendType string, cause error) *StorageError {
	return &StorageError{
		Code:        ErrCodeConnectionFailed,
		Message:     "failed to connect to storage backend",
		BackendType: backendType,
		Cause:       cause,
	}
}

func NewConfigError(backendType string, message string, cause error) *StorageError {
	return &StorageError{
		Code:        ErrCodeInvalidConfig,
		Message:     message,
		BackendType: backendType,
		Cause:       cause,
	}
}

func NewBackendInitError(backendType string, cause error) *StorageError {
	return &StorageError{
		Code:        ErrCodeBackendInit,
		Message:     "failed to initialize storage backend",
		BackendType: backendType,
		Cause:       cause,
	}
}

func NewManagerNotStartedError() *StorageError {
	return &StorageError{
		Code:        ErrCodeManagerNotStarted,
		Message:     "storage manager not started",
		BackendType: "manager",
	}
}

func NewNoBackendsError() *StorageError {
	return &StorageError{
		Code:        ErrCodeNoBackends,
		Message:     "no storage backends available",
		BackendType: "manager",
	}
}

// Capability constants
const (
	CapabilityPinning         = "pinning"
	CapabilityStreaming       = "streaming"
	CapabilityPeerAware       = "peer_aware"
	CapabilityBatch           = "batch"
	CapabilityContentAddress  = "content_addressing"
	CapabilityEncryption      = "encryption"
	CapabilityDeduplication   = "deduplication"
	CapabilityVersioning      = "versioning"
	CapabilityReplication     = "replication"
	CapabilityDistributed     = "distributed"
)

// Backend type constants
const (
	BackendTypeIPFS      = "ipfs"
	BackendTypeFilecoin  = "filecoin"
	BackendTypeArweave   = "arweave"
	BackendTypeStorJ     = "storj"
	BackendTypeLocal     = "local"
	BackendTypeS3        = "s3"
	BackendTypeGCS       = "gcs"
	BackendTypeAzure     = "azure"
)