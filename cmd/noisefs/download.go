package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// downloadFile downloads a file from NoiseFS using a descriptor CID
func downloadFile(storageManager *storage.Manager, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Downloading file: %s\n", descriptorCID)
	}

	startTime := time.Now()

	// Download the file
	data, err := client.Download(descriptorCID)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
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
				outputPath = descriptor.Name
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

	logger.Debug("File download completed",
		"descriptor_cid", descriptorCID,
		"output_path", outputPath,
		"size", len(data),
		"duration", totalDuration)

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

	// Download the directory
	err = client.DownloadDirectory(directoryCID, outputDir)
	if err != nil {
		return fmt.Errorf("directory download failed: %w", err)
	}

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
		logger.Warn("Failed to calculate directory statistics", "error", err)
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

	logger.Debug("Directory download completed",
		"descriptor_cid", directoryCID,
		"output_dir", outputDir,
		"total_files", totalFiles,
		"total_size", totalSize,
		"duration", downloadDuration)

	return nil
}

// streamingDownloadDirectory downloads a directory using streaming mode for memory efficiency
func streamingDownloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Implementation would use streaming interfaces
	logger.Info("Streaming directory download", "directory_cid", directoryCID)
	
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