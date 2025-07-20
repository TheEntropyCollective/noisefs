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
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// downloadFile downloads a file from NoiseFS using a descriptor CID
func downloadFile(storageManager *storage.Manager, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Downloading file: %s\n", descriptorCID)
	}

	startTime := time.Now()

	// Check if descriptor is encrypted
	encryptedStore, err := descriptors.NewEncryptedStore(storageManager, nil)
	if err != nil {
		return fmt.Errorf("failed to create encrypted store: %w", err)
	}

	isEncrypted, err := encryptedStore.IsEncrypted(descriptorCID)
	if err != nil {
		// If we can't determine encryption status, try regular download first
		isEncrypted = false
	}

	var data []byte
	if isEncrypted {
		// This is an encrypted descriptor, we need to handle it specially
		password := os.Getenv("NOISEFS_PASSWORD")
		if password == "" {
			password, err = util.PromptPassword("Enter decryption password: ")
			if err != nil {
				return fmt.Errorf("failed to get password: %w", err)
			}
		}

		// Create encrypted store with password
		encStoreWithPassword, err := descriptors.NewEncryptedStoreWithPassword(storageManager, password)
		if err != nil {
			return fmt.Errorf("failed to create encrypted store with password: %w", err)
		}

		// Load encrypted descriptor
		descriptor, err := encStoreWithPassword.Load(descriptorCID)
		if err != nil {
			return fmt.Errorf("failed to load encrypted descriptor (wrong password?): %w", err)
		}

		// Manually perform download using the decrypted descriptor
		data, err = downloadUsingDescriptor(client, descriptor)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	} else {
		// Regular unencrypted download
		data, err = client.Download(descriptorCID)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	downloadDuration := time.Since(startTime)

	// Determine output path
	if outputPath == "" {
		// Try to get filename from descriptor
		descriptorStore, err := descriptors.NewStoreWithManager(storageManager)
		if err != nil {
			outputPath = fmt.Sprintf("downloaded-file-%s", descriptorCID[:8])
		} else {
			descriptor, err := descriptorStore.Load(descriptorCID)
			if err != nil {
				outputPath = fmt.Sprintf("downloaded-file-%s", descriptorCID[:8])
			} else {
				outputPath = descriptor.Filename
			}
		}
	}

	// Write the downloaded data to file
	writeStartTime := time.Now()
	err = os.WriteFile(outputPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	writeDuration := time.Since(writeStartTime)

	totalDuration := time.Since(startTime)

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": descriptorCID,
			"output_path":    outputPath,
			"size_bytes":     len(data),
			"download_time":  downloadDuration.String(),
			"write_time":     writeDuration.String(),
			"total_time":     totalDuration.String(),
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Download completed successfully!\n")
		fmt.Printf("Output file: %s\n", outputPath)
		fmt.Printf("File size: %s\n", formatBytes(int64(len(data))))
		fmt.Printf("Download time: %v\n", downloadDuration)
		fmt.Printf("Write time: %v\n", writeDuration)
		fmt.Printf("Total time: %v\n", totalDuration)
	} else {
		fmt.Println(outputPath)
	}

	logger.Debug("File download completed", map[string]interface{}{
		"descriptor_cid": descriptorCID,
		"output_path":    outputPath,
		"size":           len(data),
		"duration":       totalDuration.String(),
	})

	return nil
}

// downloadDirectory downloads a directory from NoiseFS using a directory descriptor CID
func downloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Downloading directory: %s\n", directoryCID)
	}

	startTime := time.Now()

	// Ensure output directory exists
	if outputDir == "" {
		outputDir = fmt.Sprintf("downloaded-directory-%s", directoryCID[:8])
	}

	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// TODO: Implement directory download functionality
	// For now, return an error indicating feature is not yet implemented
	return fmt.Errorf("directory download not yet implemented")

	downloadDuration := time.Since(startTime)

	// Count files and calculate total size
	var totalFiles int
	var totalSize int64
	
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalFiles++
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		logger.Warn("Failed to calculate directory statistics", map[string]interface{}{
			"error": err.Error(),
		})
	}

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": directoryCID,
			"output_dir":     outputDir,
			"total_files":    totalFiles,
			"total_size":     totalSize,
			"download_time":  downloadDuration.String(),
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Directory download completed successfully!\n")
		fmt.Printf("Output directory: %s\n", outputDir)
		fmt.Printf("Total files: %d\n", totalFiles)
		fmt.Printf("Total size: %s\n", formatBytes(totalSize))
		fmt.Printf("Download time: %v\n", downloadDuration)
	} else {
		fmt.Println(outputDir)
	}

	logger.Debug("Directory download completed", map[string]interface{}{
		"descriptor_cid": directoryCID,
		"output_dir":     outputDir,
		"total_files":    totalFiles,
		"total_size":     totalSize,
		"duration":       downloadDuration,
	})

	return nil
}

// streamingDownloadDirectory downloads a directory using streaming mode for memory efficiency
func streamingDownloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Implementation would use streaming interfaces
	logger.Info("Streaming directory download", map[string]interface{}{
		"directory_cid": directoryCID,
	})
	
	// For now, fall back to regular directory download
	// TODO: Implement actual streaming download when streaming interfaces are available
	return downloadDirectory(storageManager, client, directoryCID, outputDir, quiet, jsonOutput, cfg, logger)
}

// detectDirectoryDescriptor checks if a CID represents a directory descriptor
func detectDirectoryDescriptor(storageManager *storage.Manager, cid string) (bool, error) {
	descriptorStore, err := descriptors.NewStoreWithManager(storageManager)
	if err != nil {
		return false, fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptor, err := descriptorStore.Load(cid)
	if err != nil {
		return false, fmt.Errorf("failed to load descriptor: %w", err)
	}

	// Check if this is a directory descriptor by examining its structure
	// Directory descriptors typically have specific metadata or structure
	return descriptor.IsDirectory(), nil
}

// downloadUsingDescriptor downloads file data using a pre-loaded descriptor
// This is used for encrypted descriptors where we've already decrypted the descriptor
func downloadUsingDescriptor(client *noisefs.Client, descriptor *descriptors.Descriptor) ([]byte, error) {
	// This function replicates the core logic from client.Download but uses a provided descriptor
	// rather than loading it from a CID
	
	if descriptor == nil {
		return nil, fmt.Errorf("descriptor cannot be nil")
	}

	// Retrieve and reconstruct blocks (similar to client.DownloadWithMetadataAndProgress)
	var originalBlocks []*blocks.Block

	for _, blockInfo := range descriptor.Blocks {
		// Retrieve anonymized data block
		dataBlock, err := client.RetrieveBlockWithCache(blockInfo.DataCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve data block: %w", err)
		}

		// Retrieve randomizer blocks
		randBlock1, err := client.RetrieveBlockWithCache(blockInfo.RandomizerCID1)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer1 block: %w", err)
		}

		// Retrieve second randomizer block (3-tuple XOR)
		randBlock2, err := client.RetrieveBlockWithCache(blockInfo.RandomizerCID2)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer2 block: %w", err)
		}

		// XOR to get original block
		origBlock, err := dataBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return nil, fmt.Errorf("failed to XOR blocks: %w", err)
		}

		originalBlocks = append(originalBlocks, origBlock)
	}

	// Assemble file
	assembler := blocks.NewAssembler()
	var buf strings.Builder
	if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
		return nil, fmt.Errorf("failed to assemble file: %w", err)
	}

	// Handle padding removal (all files are padded)
	assembledData := []byte(buf.String())

	// Trim to original size (all files have padding)
	originalSize := descriptor.GetOriginalFileSize()
	if int64(len(assembledData)) > originalSize {
		assembledData = assembledData[:originalSize]
	}

	return assembledData, nil
}