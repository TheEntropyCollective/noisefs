package storage

import (
	"fmt"
	"sync"
)

// BackendConstructor is a function that creates a new backend instance
type BackendConstructor func(config *BackendConfig) (Backend, error)

// backendRegistry holds registered backend constructors
var backendRegistry = struct {
	sync.RWMutex
	constructors map[string]BackendConstructor
}{
	constructors: make(map[string]BackendConstructor),
}

// RegisterBackend registers a backend constructor
func RegisterBackend(backendType string, constructor BackendConstructor) {
	backendRegistry.Lock()
	defer backendRegistry.Unlock()
	
	backendRegistry.constructors[backendType] = constructor
}

// CreateBackend creates a backend instance using the registered constructor
func CreateBackend(config *BackendConfig) (Backend, error) {
	backendRegistry.RLock()
	constructor, exists := backendRegistry.constructors[config.Type]
	backendRegistry.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("backend type %s not registered", config.Type)
	}
	
	return constructor(config)
}

// GetRegisteredBackends returns a list of registered backend types
func GetRegisteredBackends() []string {
	backendRegistry.RLock()
	defer backendRegistry.RUnlock()
	
	types := make([]string, 0, len(backendRegistry.constructors))
	for backendType := range backendRegistry.constructors {
		types = append(types, backendType)
	}
	
	return types
}