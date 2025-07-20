package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// uploadFile uploads a single file to NoiseFS
func uploadFile(storageManager *storage.Manager, client *noisefs.Client, filePath string, blockSize int, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger, encrypt bool, password string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if !quiet && !jsonOutput {
		fmt.Printf("Uploading file: %s (%s)\n", filePath, formatBytes(fileInfo.Size()))
	}

	startTime := time.Now()

	// Get filename from path
	filename := filepath.Base(filePath)

	// Upload the file (encrypted or unencrypted)
	var descriptorCID string
	if encrypt {
		if password == "" {
			// TODO: Implement interactive password prompt
			return fmt.Errorf("password required for encrypted upload - use -p flag or NOISEFS_PASSWORD environment variable")
		}
		descriptorCID, err = client.EncryptedUpload(file, filename, password)
		if err != nil {
			return fmt.Errorf("encrypted upload failed: %w", err)
		}
	} else {
		descriptorCID, err = client.Upload(file, filename)
		if err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
	}

	uploadDuration := time.Since(startTime)

	// Get metrics for the uploaded file
	metrics := client.GetMetrics()

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": descriptorCID,
			"filename":       filename,
			"size_bytes":     fileInfo.Size(),
			"upload_time":    uploadDuration.String(),
			"blocks_generated": metrics.BlocksGenerated,
			"bytes_stored":   metrics.BytesStoredIPFS,
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Upload completed successfully!\n")
		fmt.Printf("Descriptor CID: %s\n", descriptorCID)
		fmt.Printf("Upload time: %v\n", uploadDuration)
		fmt.Printf("Blocks generated: %d\n", metrics.BlocksGenerated)
		fmt.Printf("Total bytes stored in IPFS: %s\n", formatBytes(metrics.BytesStoredIPFS))
		
		// Calculate and display storage efficiency
		if fileInfo.Size() > 0 {
			overhead := float64(metrics.BytesStoredIPFS) / float64(fileInfo.Size())
			fmt.Printf("Storage efficiency: %.1fx overhead\n", overhead)
		}
	} else {
		fmt.Println(descriptorCID)
	}

	logger.Debug("File upload completed", map[string]interface{}{
		"file":           filePath,
		"descriptor_cid": descriptorCID,
		"size":           fileInfo.Size(),
		"duration":       uploadDuration.String(),
	})

	return nil
}

// uploadDirectory uploads a directory to NoiseFS
func uploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger, encrypt bool, password string) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Uploading directory: %s\n", dirPath)
	}

	// Parse exclude patterns
	var excludes []string
	if excludePatterns != "" {
		excludes = strings.Split(excludePatterns, ",")
		for i, pattern := range excludes {
			excludes[i] = strings.TrimSpace(pattern)
		}
	}

	// TODO: Implement directory upload functionality
	// For now, return an error indicating feature is not yet implemented
	return fmt.Errorf("directory upload not yet implemented")
}

// streamingUploadDirectory uploads a directory using streaming mode for memory efficiency
func streamingUploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger, encrypt bool, password string) error {
	// Implementation would use streaming interfaces
	logger.Info("Streaming directory upload", map[string]interface{}{
		"directory": dirPath,
	})
	
	// For now, fall back to regular directory upload
	// TODO: Implement actual streaming upload when streaming interfaces are available
	return uploadDirectory(storageManager, client, dirPath, blockSize, excludePatterns, quiet, jsonOutput, cfg, logger, encrypt, password)
}

// DirectoryBlockProcessor handles directory block processing during streaming uploads
type DirectoryBlockProcessor struct {
	Client        *noisefs.Client
	OutputDir     string
	Quiet         bool
	JSONOutput    bool
	Logger        *logging.Logger
}

// ProcessDirectoryBlock processes individual blocks during directory streaming
func (dbp *DirectoryBlockProcessor) ProcessDirectoryBlock(blockIndex int, block *blocks.Block) error {
	if !dbp.Quiet && !dbp.JSONOutput {
		fmt.Printf("Processing block %d (%s)\n", blockIndex, formatBytes(int64(len(block.Data))))
	}
	
	dbp.Logger.Debug("Processing directory block", map[string]interface{}{
		"block_index": blockIndex,
		"block_size":  len(block.Data),
	})
	
	// Block processing logic would go here
	return nil
}

// ProcessDirectoryManifest processes the directory manifest during streaming
func (dbp *DirectoryBlockProcessor) ProcessDirectoryManifest(dirPath string, manifestBlock *blocks.Block) error {
	if !dbp.Quiet && !dbp.JSONOutput {
		fmt.Printf("Processing directory manifest for: %s\n", dirPath)
	}
	
	dbp.Logger.Debug("Processing directory manifest", map[string]interface{}{
		"directory":     dirPath,
		"manifest_size": len(manifestBlock.Data),
	})
	
	// Manifest processing logic would go here
	return nil
}