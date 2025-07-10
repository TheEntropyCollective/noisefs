package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
)

func main() {
	// Get default index path
	indexPath, err := fuse.GetDefaultIndexPath()
	if err != nil {
		log.Fatalf("Failed to get default index path: %v", err)
	}

	// Allow custom index path as argument
	if len(os.Args) > 1 {
		indexPath = os.Args[1]
	}

	fmt.Printf("Inspecting index file: %s\n", indexPath)
	fmt.Printf("=====================================\n")

	// Check if index file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		fmt.Printf("Index file does not exist.\n")
		return
	}

	// Read raw index file
	data, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatalf("Failed to read index file: %v", err)
	}

	fmt.Printf("Raw index file size: %d bytes\n", len(data))
	fmt.Printf("First 500 characters:\n%s\n", string(data[:min(500, len(data))]))
	fmt.Printf("=====================================\n")

	// Try to parse as JSON
	var rawIndex interface{}
	if err := json.Unmarshal(data, &rawIndex); err != nil {
		fmt.Printf("Failed to parse as JSON: %v\n", err)
		return
	}

	// Pretty print the JSON structure
	prettyData, err := json.MarshalIndent(rawIndex, "", "  ")
	if err != nil {
		fmt.Printf("Failed to pretty print JSON: %v\n", err)
		return
	}

	fmt.Printf("Pretty printed JSON:\n%s\n", string(prettyData))
	fmt.Printf("=====================================\n")

	// Load using FileIndex
	index := fuse.NewFileIndex(indexPath)
	if err := index.LoadIndex(); err != nil {
		fmt.Printf("Failed to load index with FileIndex: %v\n", err)
		return
	}

	// Display index statistics
	files := index.ListFiles()
	fmt.Printf("Index Statistics:\n")
	fmt.Printf("- Total files: %d\n", len(files))
	fmt.Printf("- Index size: %d\n", index.GetSize())
	fmt.Printf("- Is dirty: %v\n", index.IsDirty())
	fmt.Printf("\n")

	// Display directory structure
	directories := make(map[string]int)
	for _, entry := range files {
		if entry.Directory == "" {
			directories["<root>"]++
		} else {
			directories[entry.Directory]++
		}
	}

	fmt.Printf("Directory Structure:\n")
	for dir, count := range directories {
		fmt.Printf("- %s: %d files\n", dir, count)
	}
	fmt.Printf("\n")

	// Display individual files
	fmt.Printf("Individual Files:\n")
	for filepath, entry := range files {
		fmt.Printf("- File: %s\n", filepath)
		fmt.Printf("  Directory: %s\n", entry.Directory)
		fmt.Printf("  Filename: %s\n", entry.Filename)
		fmt.Printf("  Size: %d bytes\n", entry.FileSize)
		fmt.Printf("  Descriptor CID: %s\n", entry.DescriptorCID)
		fmt.Printf("  Created: %s\n", entry.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Modified: %s\n", entry.ModifiedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("\n")
	}

	// Test directory operations
	fmt.Printf("Testing Directory Operations:\n")
	fmt.Printf("=====================================\n")
	
	// Test GetFilesInDirectory for root
	rootFiles := index.GetFilesInDirectory("")
	fmt.Printf("GetFilesInDirectory(''): %d files\n", len(rootFiles))
	for _, entry := range rootFiles {
		fmt.Printf("  - %s\n", entry.Filename)
	}
	
	// Test IsDirectory for known directories
	for dir := range directories {
		if dir != "<root>" {
			isDir := index.IsDirectory(dir)
			fmt.Printf("IsDirectory('%s'): %v\n", dir, isDir)
		}
	}
	
	// Test GetFilesInDirectory for known directories
	for dir := range directories {
		if dir != "<root>" {
			dirFiles := index.GetFilesInDirectory(dir)
			fmt.Printf("GetFilesInDirectory('%s'): %d files\n", dir, len(dirFiles))
			for _, entry := range dirFiles {
				fmt.Printf("  - %s\n", entry.Filename)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}