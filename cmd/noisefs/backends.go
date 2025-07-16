package main

import (
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
)

func init() {
	// Register only the mock backend for search testing
	storage.RegisterBackend("mock", func(config *storage.BackendConfig) (storage.Backend, error) {
		return backends.NewMockBackend("mock", config)
	})
}