package fuse

import (
	"fmt"
	"log"
	"time"
)

// ExampleConfigUsage demonstrates how to use the configuration system
func ExampleConfigUsage() {
	// Example 1: Use default configuration
	defaultConfig := DefaultFuseConfig()
	fmt.Printf("Default cache size: %d\n", defaultConfig.Cache.DirectoryMaxSize)
	
	// Example 2: Use performance-optimized configuration
	perfConfig := PerformanceFuseConfig()
	fmt.Printf("Performance cache size: %d\n", perfConfig.Cache.DirectoryMaxSize)
	
	// Example 3: Use security-focused configuration
	secureConfig := SecureFuseConfig()
	fmt.Printf("Secure encryption enabled: %t\n", secureConfig.Security.EnableEncryption)
	
	// Example 4: Load configuration from environment
	envConfig := LoadConfigFromEnv()
	fmt.Printf("Env-based config volume name: %s\n", envConfig.Mount.DefaultVolumeName)
	
	// Example 5: Customize configuration
	customConfig := DefaultFuseConfig()
	
	// Customize cache settings
	customConfig.Cache.DirectoryMaxSize = 300
	customConfig.Cache.DirectoryTTL = 45 * time.Minute
	
	// Customize security settings
	customConfig.Security.DefaultFileMode = 0640
	customConfig.Security.SecureDeletion = true
	customConfig.Security.SecureDeletionPasses = 5
	
	// Customize mount settings
	customConfig.Mount.DefaultVolumeName = "my-noisefs"
	customConfig.Mount.AllowOther = true
	
	// Validate the custom configuration
	if err := ValidateConfig(customConfig); err != nil {
		log.Printf("Invalid configuration: %v", err)
		return
	}
	
	fmt.Printf("Custom config is valid\n")
	
	// Example 6: Save and load configuration to/from file
	configPath := "/tmp/noisefs-config.json"
	
	// Save custom config to file
	if err := SaveConfigToFile(customConfig, configPath); err != nil {
		log.Printf("Failed to save config: %v", err)
		return
	}
	
	// Load config from file
	loadedConfig, err := LoadConfigFromFile(configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		return
	}
	
	fmt.Printf("Loaded config cache size: %d\n", loadedConfig.Cache.DirectoryMaxSize)
}

// ExampleMountWithConfig demonstrates using configuration with mount operations
func ExampleMountWithConfig() {
	// This would be used in actual mount code:
	/*
	// Create custom configuration
	config := PerformanceFuseConfig()
	config.Mount.DefaultVolumeName = "fast-noisefs"
	config.Performance.MaxConcurrentOperations = 100
	
	// Use in mount options
	opts := MountOptions{
		MountPath: "/mnt/noisefs",
		Config:    config,  // Pass custom configuration
	}
	
	// Mount with custom configuration
	err := MountWithIndex(client, storageManager, opts, "")
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}
	*/
	
	fmt.Println("Example mount configuration setup")
}