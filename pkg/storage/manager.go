package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Manager orchestrates operations across multiple storage backends
type Manager struct {
	config    *Config
	backends  map[string]Backend
	factory   *BackendFactory
	router    *Router
	monitor   *HealthMonitor
	
	// State management
	mutex      sync.RWMutex
	started    bool
	errorReporter ErrorReporter
}

// NewManager creates a new storage manager
func NewManager(config *Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, NewConfigError("manager", "invalid configuration", err)
	}
	
	factory := NewBackendFactory(config)
	
	manager := &Manager{
		config:        config,
		backends:      make(map[string]Backend),
		factory:       factory,
		errorReporter: NewDefaultErrorReporter(),
	}
	
	// Initialize router
	manager.router = NewRouter(manager, config.Distribution)
	
	// Initialize health monitor
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
	
	// Connect to all backends
	var errors ErrorAggregator
	for name, backend := range backends {
		if err := backend.Connect(ctx); err != nil {
			connectionErr := NewConnectionError(name, err)
			errors.Add(connectionErr)
			continue
		}
		m.backends[name] = backend
	}
	
	if len(m.backends) == 0 {
		if errors.HasErrors() {
			return errors.CreateAggregateError()
		}
		return NewNoBackendsError()
	}
	
	// Start health monitoring
	if m.config.HealthCheck.Enabled {
		if err := m.monitor.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health monitor: %w", err)
		}
	}
	
	m.started = true
	
	// Report any connection errors but don't fail startup if some backends are available
	if errors.HasErrors() {
		// Log the errors but continue
		for _, err := range errors.GetAllErrors() {
			if storageErr, ok := err.(*StorageError); ok {
				m.errorReporter.ReportError(storageErr)
			}
		}
	}
	
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
	
	// Disconnect from all backends
	var errors ErrorAggregator
	for name, backend := range m.backends {
		if err := backend.Disconnect(ctx); err != nil {
			errors.Add(fmt.Errorf("failed to disconnect from backend '%s': %w", name, err))
		}
	}
	
	m.backends = make(map[string]Backend)
	m.started = false
	
	if errors.HasErrors() {
		return errors.CreateAggregateError()
	}
	
	return nil
}

// Put stores a block across selected backends
func (m *Manager) Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error) {
	if !m.started {
		return nil, NewManagerNotStartedError()
	}
	
	// Use router to determine storage strategy
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

// GetBackend returns a specific backend by name
func (m *Manager) GetBackend(name string) (Backend, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	backend, exists := m.backends[name]
	return backend, exists
}

// GetAvailableBackends returns all currently available backends
func (m *Manager) GetAvailableBackends() map[string]Backend {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Return a copy to prevent concurrent access issues
	available := make(map[string]Backend)
	for name, backend := range m.backends {
		if backend.IsConnected() {
			available[name] = backend
		}
	}
	
	return available
}

// GetHealthyBackends returns backends that are currently healthy
func (m *Manager) GetHealthyBackends() map[string]Backend {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	healthy := make(map[string]Backend)
	for name, backend := range m.backends {
		if backend.IsConnected() {
			status := backend.HealthCheck(context.Background())
			if status.Healthy {
				healthy[name] = backend
			}
		}
	}
	
	return healthy
}

// GetBackendsByPriority returns backends sorted by priority and health
func (m *Manager) GetBackendsByPriority() []Backend {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	type backendInfo struct {
		backend  Backend
		name     string
		priority int
		healthy  bool
	}
	
	var infos []backendInfo
	for name, backend := range m.backends {
		if !backend.IsConnected() {
			continue
		}
		
		config, exists := m.config.Backends[name]
		if !exists {
			continue
		}
		
		status := backend.HealthCheck(context.Background())
		
		infos = append(infos, backendInfo{
			backend:  backend,
			name:     name,
			priority: config.Priority,
			healthy:  status.Healthy,
		})
	}
	
	// Sort by health first, then priority
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].healthy != infos[j].healthy {
			return infos[i].healthy // Healthy backends first
		}
		return infos[i].priority > infos[j].priority // Higher priority first
	})
	
	backends := make([]Backend, len(infos))
	for i, info := range infos {
		backends[i] = info.backend
	}
	
	return backends
}

// GetManagerStatus returns the current status of the manager
func (m *Manager) GetManagerStatus() *ManagerStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	status := &ManagerStatus{
		Started:        m.started,
		TotalBackends:  len(m.config.Backends),
		ActiveBackends: len(m.backends),
		HealthyBackends: 0,
		BackendStatus:  make(map[string]*BackendStatus),
		LastCheck:     time.Now(),
	}
	
	for name, backend := range m.backends {
		backendHealth := backend.HealthCheck(context.Background())
		backendInfo := backend.GetBackendInfo()
		
		backendStatus := &BackendStatus{
			Name:         name,
			Type:         backendInfo.Type,
			Connected:    backend.IsConnected(),
			Healthy:      backendHealth.Healthy,
			Status:       backendHealth.Status,
			Latency:      backendHealth.Latency,
			ErrorRate:    backendHealth.ErrorRate,
			LastCheck:    backendHealth.LastCheck,
			Capabilities: backendInfo.Capabilities,
		}
		
		if backendStatus.Healthy {
			status.HealthyBackends++
		}
		
		status.BackendStatus[name] = backendStatus
	}
	
	return status
}

// GetRouter returns the storage router
func (m *Manager) GetRouter() *Router {
	return m.router
}

// GetHealthMonitor returns the health monitor
func (m *Manager) GetHealthMonitor() *HealthMonitor {
	return m.monitor
}

// GetConfig returns the manager configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// GetErrorMetrics returns error metrics from the error reporter
func (m *Manager) GetErrorMetrics() *ErrorMetrics {
	return m.errorReporter.GetErrorMetrics()
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
	oldBackend, exists := m.backends[name]
	if !exists {
		return NewStorageError(ErrCodeNotFound, fmt.Sprintf("backend '%s' not found", name), name, nil)
	}
	
	// Disconnect old backend
	ctx := context.Background()
	if err := oldBackend.Disconnect(ctx); err != nil {
		return NewConnectionError(name, err)
	}
	
	// Update configuration
	m.config.Backends[name] = newConfig
	
	// Create new backend if enabled
	if newConfig.Enabled {
		newBackend, err := m.factory.CreateBackend(name)
		if err != nil {
			return NewBackendInitError(name, err)
		}
		
		// Connect new backend
		if err := newBackend.Connect(ctx); err != nil {
			return NewConnectionError(name, err)
		}
		
		m.backends[name] = newBackend
	} else {
		// Remove from active backends if disabled
		delete(m.backends, name)
	}
	
	return nil
}

// Status types

// ManagerStatus represents the overall status of the storage manager
type ManagerStatus struct {
	Started         bool                     `json:"started"`
	TotalBackends   int                      `json:"total_backends"`
	ActiveBackends  int                      `json:"active_backends"`
	HealthyBackends int                      `json:"healthy_backends"`
	BackendStatus   map[string]*BackendStatus `json:"backend_status"`
	LastCheck       time.Time                `json:"last_check"`
}

// BackendStatus represents the status of a single backend
type BackendStatus struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"`
	Connected    bool          `json:"connected"`
	Healthy      bool          `json:"healthy"`
	Status       string        `json:"status"`
	Latency      time.Duration `json:"latency"`
	ErrorRate    float64       `json:"error_rate"`
	LastCheck    time.Time     `json:"last_check"`
	Capabilities []string      `json:"capabilities"`
}

// Helper methods for common operations

// GetDefaultBackend returns the default backend instance
func (m *Manager) GetDefaultBackend() (Backend, error) {
	defaultName := m.config.DefaultBackend
	backend, exists := m.GetBackend(defaultName)
	if !exists {
		return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("default backend '%s' not available", defaultName), defaultName, nil)
	}
	return backend, nil
}

// SelectBestBackend selects the best available backend for an operation
func (m *Manager) SelectBestBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error) {
	return m.router.SelectBackend(ctx, criteria)
}

// GetBackendsWithCapability returns backends that support a specific capability
func (m *Manager) GetBackendsWithCapability(capability string) []Backend {
	var result []Backend
	
	for _, backend := range m.GetAvailableBackends() {
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			if cap == capability {
				result = append(result, backend)
				break
			}
		}
	}
	
	return result
}

// Ensure Manager implements the Backend interface for unified access
var _ Backend = (*Manager)(nil)

// Implement Backend interface methods that delegate to the router

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
	status := m.GetManagerStatus()
	
	healthy := status.HealthyBackends > 0
	healthStr := "healthy"
	var issues []HealthIssue
	
	if status.HealthyBackends == 0 {
		healthy = false
		healthStr = "critical"
		issues = append(issues, HealthIssue{
			Severity:    "critical",
			Code:        "NO_HEALTHY_BACKENDS",
			Description: "No healthy backends available",
			Timestamp:   time.Now(),
		})
	} else if status.HealthyBackends < status.ActiveBackends {
		healthStr = "degraded"
		issues = append(issues, HealthIssue{
			Severity:    "warning",
			Code:        "SOME_BACKENDS_UNHEALTHY",
			Description: fmt.Sprintf("%d of %d backends unhealthy", 
				status.ActiveBackends-status.HealthyBackends, status.ActiveBackends),
			Timestamp:   time.Now(),
		})
	}
	
	return &HealthStatus{
		Healthy:   healthy,
		Status:    healthStr,
		LastCheck: time.Now(),
		Issues:    issues,
	}
}

// Convenience methods (work with CIDs directly)

// StoreBlock stores a block and returns its CID (convenience method)
func (m *Manager) StoreBlock(block *blocks.Block) (string, error) {
	ctx := context.Background()
	address, err := m.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// RetrieveBlock retrieves a block by CID (convenience method)
func (m *Manager) RetrieveBlock(cid string) (*blocks.Block, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: m.config.DefaultBackend,
	}
	return m.Get(ctx, address)
}

// HasBlock checks if a block exists by CID (convenience method)
func (m *Manager) HasBlock(cid string) (bool, error) {
	ctx := context.Background()
	address := &BlockAddress{
		ID:          cid,
		BackendType: m.config.DefaultBackend,
	}
	return m.Has(ctx, address)
}