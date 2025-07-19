package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// uploadFile uploads a single file to NoiseFS
func uploadFile(storageManager *storage.Manager, client *noisefs.Client, filePath string, blockSize int, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
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

	// Upload the file
	descriptorCID, err := client.Upload(file, filename)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
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
			"blocks_stored":  metrics.BlocksStored,
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
		fmt.Printf("Blocks stored: %d\n", metrics.BlocksStored)
		fmt.Printf("Total bytes stored in IPFS: %s\n", formatBytes(metrics.BytesStoredIPFS))
		
		// Calculate and display storage efficiency
		if fileInfo.Size() > 0 {
			overhead := float64(metrics.BytesStoredIPFS) / float64(fileInfo.Size())
			fmt.Printf("Storage efficiency: %.1fx overhead\n", overhead)
		}
	} else {
		fmt.Println(descriptorCID)
	}

	logger.Debug("File upload completed", 
		"file", filePath,
		"descriptor_cid", descriptorCID,
		"size", fileInfo.Size(),
		"duration", uploadDuration)

	return nil
}

// uploadDirectory uploads a directory to NoiseFS
func uploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Uploading directory: %s\n", dirPath)
	}

	startTime := time.Now()

	// Parse exclude patterns
	var excludes []string
	if excludePatterns != "" {
		excludes = strings.Split(excludePatterns, ",")
		for i, pattern := range excludes {
			excludes[i] = strings.TrimSpace(pattern)
		}
	}

	// Upload the directory
	descriptorCID, err := client.UploadDirectory(dirPath, excludes)
	if err != nil {
		return fmt.Errorf("directory upload failed: %w", err)
	}

	uploadDuration := time.Since(startTime)

	// Get metrics for the uploaded directory
	metrics := client.GetMetrics()

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": descriptorCID,
			"directory":      dirPath,
			"upload_time":    uploadDuration.String(),
			"blocks_stored":  metrics.BlocksStored,
			"bytes_stored":   metrics.BytesStoredIPFS,
		}
		
		if len(excludes) > 0 {
			result["excluded_patterns"] = excludes
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Directory upload completed successfully!\n")
		fmt.Printf("Descriptor CID: %s\n", descriptorCID)
		fmt.Printf("Upload time: %v\n", uploadDuration)
		fmt.Printf("Blocks stored: %d\n", metrics.BlocksStored)
		fmt.Printf("Total bytes stored in IPFS: %s\n", formatBytes(metrics.BytesStoredIPFS))
		
		if len(excludes) > 0 {
			fmt.Printf("Excluded patterns: %s\n", strings.Join(excludes, ", "))
		}
	} else {
		fmt.Println(descriptorCID)
	}

	logger.Debug("Directory upload completed",
		"directory", dirPath,
		"descriptor_cid", descriptorCID,
		"duration", uploadDuration,
		"excludes", excludes)

	return nil
}

// streamingUploadDirectory uploads a directory using streaming mode for memory efficiency
func streamingUploadDirectory(storageManager *storage.Manager, client *noisefs.Client, dirPath string, blockSize int, excludePatterns string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Implementation would use streaming interfaces
	logger.Info("Streaming directory upload", "directory", dirPath)
	
	// For now, fall back to regular directory upload
	// TODO: Implement actual streaming upload when streaming interfaces are available
	return uploadDirectory(storageManager, client, dirPath, blockSize, excludePatterns, quiet, jsonOutput, cfg, logger)
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
	
	dbp.Logger.Debug("Processing directory block",
		"block_index", blockIndex,
		"block_size", len(block.Data))
	
	// Block processing logic would go here
	return nil
}

// ProcessDirectoryManifest processes the directory manifest during streaming
func (dbp *DirectoryBlockProcessor) ProcessDirectoryManifest(dirPath string, manifestBlock *blocks.Block) error {
	if !dbp.Quiet && !dbp.JSONOutput {
		fmt.Printf("Processing directory manifest for: %s\n", dirPath)
	}
	
	dbp.Logger.Debug("Processing directory manifest",
		"directory", dirPath,
		"manifest_size", len(manifestBlock.Data))
	
	// Manifest processing logic would go here
	return nil
}