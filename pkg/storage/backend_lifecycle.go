package storage

import (
	"context"
	"fmt"
	"sync"
)

// defaultBackendLifecycle implements the BackendLifecycle interface
type defaultBackendLifecycle struct {
	connectionErrors []error
	mutex            sync.RWMutex
}

// NewBackendLifecycle creates a new backend lifecycle manager
func NewBackendLifecycle() BackendLifecycle {
	return &defaultBackendLifecycle{
		connectionErrors: make([]error, 0),
	}
}

// ConnectBackend connects a single backend
func (l *defaultBackendLifecycle) ConnectBackend(ctx context.Context, name string, backend Backend) error {
	if err := backend.Connect(ctx); err != nil {
		connectionErr := NewConnectionError(name, err)
		l.addConnectionError(connectionErr)
		return connectionErr
	}
	return nil
}

// DisconnectBackend disconnects a single backend
func (l *defaultBackendLifecycle) DisconnectBackend(ctx context.Context, name string, backend Backend) error {
	if err := backend.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from backend '%s': %w", name, err)
	}
	return nil
}

// ConnectAllBackends connects multiple backends, collecting errors
func (l *defaultBackendLifecycle) ConnectAllBackends(ctx context.Context, backends map[string]Backend) error {
	l.clearConnectionErrors()

	var errors ErrorAggregator
	for name, backend := range backends {
		if err := l.ConnectBackend(ctx, name, backend); err != nil {
			errors.Add(err)
		}
	}

	if errors.HasErrors() {
		return errors.CreateAggregateError()
	}

	return nil
}

// DisconnectAllBackends disconnects multiple backends, collecting errors
func (l *defaultBackendLifecycle) DisconnectAllBackends(ctx context.Context, backends map[string]Backend) error {
	var errors ErrorAggregator
	for name, backend := range backends {
		if err := l.DisconnectBackend(ctx, name, backend); err != nil {
			errors.Add(err)
		}
	}

	if errors.HasErrors() {
		return errors.CreateAggregateError()
	}

	return nil
}

// IsBackendConnected checks if a specific backend is connected
func (l *defaultBackendLifecycle) IsBackendConnected(name string) bool {
	// This method would need access to the registry to check the backend's connection status
	// For now, we'll return a placeholder - this will be properly implemented when integrated with Manager
	return false
}

// GetConnectionErrors returns all connection errors that occurred
func (l *defaultBackendLifecycle) GetConnectionErrors() []error {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// Return a copy to prevent concurrent access issues
	errors := make([]error, len(l.connectionErrors))
	copy(errors, l.connectionErrors)
	return errors
}

// addConnectionError adds a connection error to the list
func (l *defaultBackendLifecycle) addConnectionError(err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.connectionErrors = append(l.connectionErrors, err)
}

// clearConnectionErrors clears all connection errors
func (l *defaultBackendLifecycle) clearConnectionErrors() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.connectionErrors = l.connectionErrors[:0]
}
