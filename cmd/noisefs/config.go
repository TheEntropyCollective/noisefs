package main

import (
	"context"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// loadConfig loads NoiseFS configuration from file or uses defaults
func loadConfig(configPath string) (*config.Config, error) {
	if configPath != "" {
		return config.LoadConfig(configPath)
	}
	
	// Use default configuration
	return config.DefaultConfig(), nil
}

// initializeStorageManager creates and starts a storage manager
func initializeStorageManager(cfg *config.Config) (*storage.Manager, error) {
	// Create storage configuration
	storageConfig := storage.DefaultConfig()
	
	// Apply IPFS configuration from main config
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}
	
	// Create storage manager
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage manager: %w", err)
	}
	
	// Start storage manager
	ctx := context.Background()
	err = storageManager.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start storage manager: %w", err)
	}
	
	return storageManager, nil
}