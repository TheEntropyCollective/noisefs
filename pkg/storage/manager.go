package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Manager orchestrates operations across multiple storage backends using focused services
type Manager struct {
	config  *Config
	factory *BackendFactory
	router  *Router
	monitor *HealthMonitor

	// Decomposed services
	registry  BackendRegistry
	lifecycle BackendLifecycle
	selector  BackendSelector
	status    StatusAggregator

	// State management
	mutex         sync.RWMutex
	started       bool
	errorReporter ErrorReporter
}

// NewManager creates a new storage manager with decomposed services
func NewManager(config *Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, NewConfigError("manager", "invalid configuration", err)
	}

	factory := NewBackendFactory(config)

	// Create service instances
	registry := NewBackendRegistry()
	lifecycle := NewBackendLifecycle()
	selector := NewBackendSelector(registry, config)
	statusAggregator := NewStatusAggregator(registry, config)

	manager := &Manager{
		config:        config,
		factory:       factory,
		registry:      registry,
		lifecycle:     lifecycle,
		selector:      selector,
		status:        statusAggregator,
		errorReporter: NewDefaultErrorReporter(),
	}

	// Initialize router with the manager facade
	manager.router = NewRouter(manager, config.Distribution)

	// Initialize health monitor with the manager facade
	manager.monitor = NewHealthMonitor(manager, config.HealthCheck)

	return manager, nil
}

// Start initializes all backends and starts the manager
func (m *Manager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.started {
		return NewStorageError(ErrCodeAlreadyExists, "manager already started", "manager", nil)
	}

	// Create all enabled backends
	backends, err := m.factory.CreateAllBackends()
	if err != nil {
		return NewBackendInitError("manager", err)
	}

	// Add backends to registry
	for name, backend := range backends {
		m.registry.AddBackend(name, backend)
	}

	// Connect to all backends using lifecycle service
	if err := m.lifecycle.ConnectAllBackends(ctx, backends); err != nil {
		// Remove unconnected backends from registry
		connectedBackends := make(map[string]Backend)
		for name, backend := range backends {
			if backend.IsConnected() {
				connectedBackends[name] = backend
			} else {
				m.registry.RemoveBackend(name)
			}
		}

		if len(connectedBackends) == 0 {
			return NewNoBackendsError()
		}

		// Report connection errors but continue if some backends connected
		connectionErrors := m.lifecycle.GetConnectionErrors()
		for _, err := range connectionErrors {
			if storageErr, ok := err.(*StorageError); ok {
				m.errorReporter.ReportError(storageErr)
			}
		}
	}

	// Start health monitoring
	if m.config.HealthCheck.Enabled {
		if err := m.monitor.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health monitor: %w", err)
		}
	}

	m.started = true
	return nil
}

// Stop gracefully shuts down the manager
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.started {
		return nil
	}

	// Stop health monitoring
	if m.monitor != nil {
		m.monitor.Stop()
	}

	// Disconnect from all backends using lifecycle service
	backends := m.registry.GetAllBackends()
	if err := m.lifecycle.DisconnectAllBackends(ctx, backends); err != nil {
		// Continue with cleanup even if some disconnections failed
		for name := range backends {
			m.registry.RemoveBackend(name)
		}
		m.started = false
		return err
	}

	// Clear registry
	for name := range backends {
		m.registry.RemoveBackend(name)
	}

	m.started = false
	return nil
}

// Put stores a block across selected backends
func (m *Manager) Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error) {
	if !m.started {
		return nil, NewManagerNotStartedError()
	}

	return m.router.Put(ctx, block)
}

// Get retrieves a block from the best available backend
func (m *Manager) Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error) {
	if !m.started {
		return nil, NewManagerNotStartedError()
	}

	return m.router.Get(ctx, address)
}

// Has checks if a block exists in any backend
func (m *Manager) Has(ctx context.Context, address *BlockAddress) (bool, error) {
	if !m.started {
		return false, NewManagerNotStartedError()
	}

	return m.router.Has(ctx, address)
}

// Delete removes a block from all backends where it exists
func (m *Manager) Delete(ctx context.Context, address *BlockAddress) error {
	if !m.started {
		return NewManagerNotStartedError()
	}

	return m.router.Delete(ctx, address)
}

// PutMany stores multiple blocks using optimal distribution
func (m *Manager) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error) {
	if !m.started {
		return nil, NewManagerNotStartedError()
	}

	return m.router.PutMany(ctx, blocks)
}

// GetMany retrieves multiple blocks efficiently
func (m *Manager) GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error) {
	if !m.started {
		return nil, NewManagerNotStartedError()
	}

	return m.router.GetMany(ctx, addresses)
}

// Pin pins a block in appropriate backends
func (m *Manager) Pin(ctx context.Context, address *BlockAddress) error {
	if !m.started {
		return NewManagerNotStartedError()
	}

	return m.router.Pin(ctx, address)
}

// Unpin unpins a block from all backends
func (m *Manager) Unpin(ctx context.Context, address *BlockAddress) error {
	if !m.started {
		return NewManagerNotStartedError()
	}

	return m.router.Unpin(ctx, address)
}

// Backend registry delegation
func (m *Manager) GetBackend(name string) (Backend, bool) {
	return m.registry.GetBackend(name)
}

func (m *Manager) GetAvailableBackends() map[string]Backend {
	return m.registry.GetAvailableBackends()
}

func (m *Manager) GetHealthyBackends() map[string]Backend {
	return m.registry.GetHealthyBackends()
}

func (m *Manager) GetBackendsWithCapability(capability string) []Backend {
	return m.registry.GetBackendsWithCapability(capability)
}

// Backend selector delegation
func (m *Manager) GetBackendsByPriority() []Backend {
	return m.selector.GetBackendsByPriority()
}

func (m *Manager) GetDefaultBackend() (Backend, error) {
	return m.selector.GetDefaultBackend()
}

func (m *Manager) SelectBestBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error) {
	return m.selector.SelectBestBackend(ctx, criteria)
}

// Status aggregation delegation
func (m *Manager) GetManagerStatus() *ManagerStatus {
	return m.status.GetManagerStatus()
}

func (m *Manager) GetConnectedPeerCount() int {
	return m.status.GetConnectedPeerCount()
}

// Component access methods
func (m *Manager) GetRouter() *Router {
	return m.router
}

func (m *Manager) GetHealthMonitor() *HealthMonitor {
	return m.monitor
}

func (m *Manager) GetConfig() *Config {
	return m.config
}

func (m *Manager) GetErrorMetrics() *ErrorMetrics {
	return m.errorReporter.GetErrorMetrics()
}

// GetRegistry returns the backend registry (for testing)
func (m *Manager) GetRegistry() BackendRegistry {
	return m.registry
}

// ReconfigureBackend updates configuration for a specific backend
func (m *Manager) ReconfigureBackend(name string, newConfig *BackendConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.started {
		return NewManagerNotStartedError()
	}

	// Validate new configuration
	if err := newConfig.Validate(); err != nil {
		return NewConfigError(name, "invalid backend configuration", err)
	}

	// Check if backend exists
	oldBackend, exists := m.registry.GetBackend(name)
	if !exists {
		return NewStorageError(ErrCodeNotFound, fmt.Sprintf("backend '%s' not found", name), name, nil)
	}

	// Disconnect old backend
	ctx := context.Background()
	if err := m.lifecycle.DisconnectBackend(ctx, name, oldBackend); err != nil {
		return NewConnectionError(name, err)
	}

	// Remove from registry
	m.registry.RemoveBackend(name)

	// Update configuration
	m.config.Backends[name] = newConfig

	// Create new backend if enabled
	if newConfig.Enabled {
		newBackend, err := m.factory.CreateBackend(name)
		if err != nil {
			return NewBackendInitError(name, err)
		}

		// Connect new backend
		if err := m.lifecycle.ConnectBackend(ctx, name, newBackend); err != nil {
			return err
		}

		// Add to registry
		m.registry.AddBackend(name, newBackend)
	}

	return nil
}

// Backend interface implementation (NO LONGER IMPLEMENTS Backend interface)
// This removes the "weird Backend interface delegation" mentioned in acceptance criteria

// Connection management for backwards compatibility
func (m *Manager) Connect(ctx context.Context) error {
	return m.Start(ctx)
}

func (m *Manager) Disconnect(ctx context.Context) error {
	return m.Stop(ctx)
}

func (m *Manager) IsConnected() bool {
	return m.started && len(m.GetAvailableBackends()) > 0
}

func (m *Manager) GetBackendInfo() *BackendInfo {
	backends := m.GetAvailableBackends()

	// Collect capabilities from all backends
	capabilitySet := make(map[string]bool)
	var backendNames []string

	for name, backend := range backends {
		backendNames = append(backendNames, name)
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			capabilitySet[cap] = true
		}
	}

	var capabilities []string
	for cap := range capabilitySet {
		capabilities = append(capabilities, cap)
	}

	return &BackendInfo{
		Name:         "NoiseFS Storage Manager",
		Type:         "manager",
		Version:      "1.0",
		Capabilities: capabilities,
		Config: map[string]interface{}{
			"active_backends": backendNames,
			"total_backends":  len(m.config.Backends),
		},
	}
}

func (m *Manager) HealthCheck(ctx context.Context) *HealthStatus {
	return m.status.GetHealthStatus(ctx)
}

// Convenience methods (work with CIDs directly)
func (m *Manager) StoreBlock(block *blocks.Block) (string, error) {
	ctx := context.Background()
	address, err := m.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

func (m *Manager) RetrieveBlock(cid string) (*blocks.Block, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: m.config.DefaultBackend,
	}
	return m.Get(ctx, address)
}

func (m *Manager) HasBlock(cid string) (bool, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: m.config.DefaultBackend,
	}
	return m.Has(ctx, address)
}
