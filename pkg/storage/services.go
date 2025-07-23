package storage

import (
	"context"
)

// BackendRegistry manages backend instances and lookups
type BackendRegistry interface {
	// Backend management
	GetBackend(name string) (Backend, bool)
	GetAvailableBackends() map[string]Backend
	GetHealthyBackends() map[string]Backend
	GetBackendsWithCapability(capability string) []Backend
	AddBackend(name string, backend Backend)
	RemoveBackend(name string)

	// Registry information
	GetAllBackends() map[string]Backend
	GetBackendNames() []string
}

// BackendLifecycle handles backend connect/disconnect operations
type BackendLifecycle interface {
	// Connection management
	ConnectBackend(ctx context.Context, name string, backend Backend) error
	DisconnectBackend(ctx context.Context, name string, backend Backend) error
	ConnectAllBackends(ctx context.Context, backends map[string]Backend) error
	DisconnectAllBackends(ctx context.Context, backends map[string]Backend) error

	// Lifecycle status
	IsBackendConnected(name string) bool
	GetConnectionErrors() []error
}

// BackendSelector handles backend selection by various criteria
type BackendSelector interface {
	// Selection methods
	SelectBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error)
	GetBackendsByPriority() []Backend
	GetDefaultBackend() (Backend, error)
	SelectBestBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error)

	// Priority and health-based selection
	SelectHealthyBackends(count int) []Backend
	SelectBackendByCapability(capability string) (Backend, error)
}

// StatusAggregator collects and aggregates status from all backends
type StatusAggregator interface {
	// Status collection
	GetManagerStatus() *ManagerStatus
	GetBackendStatuses() map[string]*BackendStatus
	GetHealthStatus(ctx context.Context) *HealthStatus

	// Aggregated metrics
	GetTotalBackends() int
	GetActiveBackends() int
	GetHealthyBackends() int
	GetConnectedPeerCount() int

	// Individual backend status
	GetBackendStatus(name string, backend Backend) *BackendStatus
	GetBackendHealth(name string, backend Backend) *HealthStatus
}

// SelectionCriteria defines criteria for backend selection
type SelectionCriteria struct {
	// Required capabilities
	RequiredCapabilities []string

	// Preferred capabilities (nice to have)
	PreferredCapabilities []string

	// Performance requirements
	MaxLatency    float64 // milliseconds
	MinThroughput float64 // bytes per second
	MaxErrorRate  float64 // percentage (0.0-1.0)

	// Backend type restrictions
	AllowedTypes    []string
	DisallowedTypes []string

	// Health and priority preferences
	RequireHealthy     bool
	PreferHealthy      bool
	PreferHighPriority bool
	PreferLowLatency   bool

	// Load balancing
	LoadBalance bool

	// Size constraints
	MinAvailableStorage int64

	// Operation hints
	OperationType string // "read", "write", "pin", etc.
	BlockSize     int64

	// Exclusions
	ExcludeBackends []string
}

// DefaultSelectionCriteria returns default selection criteria
func DefaultSelectionCriteria() SelectionCriteria {
	return SelectionCriteria{
		RequiredCapabilities: []string{},
		PreferHighPriority:   true,
		PreferHealthy:        true,
		PreferLowLatency:     false,
		MinAvailableStorage:  0,
		OperationType:        "",
		BlockSize:            0,
		ExcludeBackends:      []string{},
	}
}

// ReadSelectionCriteria returns optimized criteria for read operations
func ReadSelectionCriteria() SelectionCriteria {
	criteria := DefaultSelectionCriteria()
	criteria.OperationType = "read"
	criteria.PreferLowLatency = true
	return criteria
}

// WriteSelectionCriteria returns optimized criteria for write operations
func WriteSelectionCriteria(blockSize int64) SelectionCriteria {
	criteria := DefaultSelectionCriteria()
	criteria.OperationType = "write"
	criteria.BlockSize = blockSize
	criteria.MinAvailableStorage = blockSize * 2 // Ensure some buffer space
	return criteria
}
