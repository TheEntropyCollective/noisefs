package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// Simple demonstration of the new storage abstraction layer
func main() {
	fmt.Println("NoiseFS Storage Abstraction Demo")
	fmt.Println("================================")

	// Create storage manager with default IPFS backend
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = "127.0.0.1:5001"
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		log.Fatalf("Failed to create storage manager: %v", err)
	}
	
	err = storageManager.Start(context.Background())
	if err != nil {
		log.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())

	// Test connectivity
	if !storageManager.IsConnected() {
		log.Fatalf("Storage manager not connected")
	}

	fmt.Printf("✅ Connected to storage backend: %s\n", storageManager.GetBackendInfo().Name)
	
	// Create a test block
	testData := []byte("Hello, NoiseFS Storage Abstraction!")
	testBlock, err := blocks.NewBlock(testData)
	if err != nil {
		log.Fatalf("Failed to create test block: %v", err)
	}

	fmt.Printf("📦 Created test block with %d bytes\n", len(testData))

	// Store the block
	cid, err := storageManager.StoreBlock(testBlock)
	if err != nil {
		log.Fatalf("Failed to store block: %v", err)
	}

	fmt.Printf("💾 Stored block with CID: %s\n", cid)

	// Check if block exists
	exists, err := storageManager.HasBlock(cid)
	if err != nil {
		log.Fatalf("Failed to check block existence: %v", err)
	}

	if !exists {
		log.Fatalf("Block should exist but doesn't")
	}

	fmt.Printf("✓ Confirmed block exists in storage\n")

	// Retrieve the block
	retrievedBlock, err := storageManager.RetrieveBlock(cid)
	if err != nil {
		log.Fatalf("Failed to retrieve block: %v", err)
	}

	fmt.Printf("📥 Retrieved block with %d bytes\n", len(retrievedBlock.Data))

	// Verify data integrity
	if string(retrievedBlock.Data) != string(testData) {
		log.Fatalf("Data integrity check failed")
	}

	fmt.Printf("✅ Data integrity verified\n")

	// Get health status
	health := storageManager.HealthCheck(context.Background())
	fmt.Printf("🏥 Backend health: %s\n", health.Status)

	fmt.Println("\n🎉 Storage abstraction demo completed successfully!")
	fmt.Println("\nKey Benefits:")
	fmt.Println("- ✅ Unified interface for different storage backends")
	fmt.Println("- ✅ Backward compatibility with existing IPFS operations")
	fmt.Println("- ✅ Health monitoring and error handling")
	fmt.Println("- ✅ Ready for multi-backend support (S3, Filecoin, etc.)")
}