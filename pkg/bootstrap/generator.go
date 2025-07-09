package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config holds configuration for the bootstrap data generator
type Config struct {
	ConfigFile        string
	OutputDir         string
	Dataset           string
	MaxSize           int64
	Verbose           bool
	ParallelDownloads int
}

// DatasetGenerator handles downloading and organizing public domain content
type DatasetGenerator struct {
	config    *Config
	datasets  map[string]*Dataset
	downloaded int64
	mutex     sync.RWMutex
}

// Dataset represents a collection of public domain content
type Dataset struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Sources     []DataSource  `json:"sources"`
	Directory   string        `json:"directory"`
	MaxFiles    int          `json:"max_files"`
}

// DataSource represents a source of public domain content
type DataSource struct {
	URL         string            `json:"url"`
	Type        string            `json:"type"` // "file", "archive", "api"
	Filename    string            `json:"filename"`
	Size        int64             `json:"size"`
	License     string            `json:"license"`
	Metadata    map[string]string `json:"metadata"`
}

// Summary contains statistics about the generated dataset
type Summary struct {
	TotalFiles     int
	TotalSize      int64
	FileTypes      []string
	DirectoryCount int
}

// NewDatasetGenerator creates a new dataset generator
func NewDatasetGenerator(config *Config) *DatasetGenerator {
	return &DatasetGenerator{
		config:   config,
		datasets: getBuiltinDatasets(),
	}
}

// GenerateDataset downloads and organizes the specified dataset
func (g *DatasetGenerator) GenerateDataset() error {
	dataset, exists := g.datasets[g.config.Dataset]
	if !exists {
		return fmt.Errorf("unknown dataset: %s", g.config.Dataset)
	}

	// Create output directory
	outputPath := filepath.Join(g.config.OutputDir, dataset.Directory)
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Download files in parallel
	semaphore := make(chan struct{}, g.config.ParallelDownloads)
	var wg sync.WaitGroup
	var errors []error
	var errorMutex sync.Mutex

	for i, source := range dataset.Sources {
		// Check if we've exceeded max size
		g.mutex.RLock()
		if g.downloaded >= g.config.MaxSize {
			g.mutex.RUnlock()
			break
		}
		g.mutex.RUnlock()

		// Check if we've exceeded max files
		if dataset.MaxFiles > 0 && i >= dataset.MaxFiles {
			break
		}

		wg.Add(1)
		go func(source DataSource, index int) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := g.downloadSource(source, outputPath); err != nil {
				errorMutex.Lock()
				errors = append(errors, fmt.Errorf("failed to download %s: %w", source.URL, err))
				errorMutex.Unlock()
			}
		}(source, i)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("download errors: %v", errors)
	}

	if g.config.Verbose {
		fmt.Printf("Downloaded %d bytes to %s\n", g.downloaded, outputPath)
	}

	return nil
}

// downloadSource downloads a single source file
func (g *DatasetGenerator) downloadSource(source DataSource, outputPath string) error {
	// Check size limit
	g.mutex.RLock()
	if g.downloaded+source.Size > g.config.MaxSize {
		g.mutex.RUnlock()
		return nil // Skip this file
	}
	g.mutex.RUnlock()

	// Create subdirectory if needed
	subdir := filepath.Join(outputPath, source.Type)
	if err := os.MkdirAll(subdir, 0755); err != nil {
		return fmt.Errorf("failed to create subdirectory: %w", err)
	}

	// Download file
	resp, err := http.Get(source.URL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create output file
	filename := source.Filename
	if filename == "" {
		filename = fmt.Sprintf("file_%d", time.Now().Unix())
	}
	
	filePath := filepath.Join(subdir, filename)
	outFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer outFile.Close()

	// Copy data with size tracking
	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Update downloaded size
	g.mutex.Lock()
	g.downloaded += written
	g.mutex.Unlock()

	if g.config.Verbose {
		fmt.Printf("Downloaded: %s (%d bytes)\n", filename, written)
	}

	// Write metadata file
	metadataPath := filePath + ".meta"
	metadataFile, err := os.Create(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %w", err)
	}
	defer metadataFile.Close()

	fmt.Fprintf(metadataFile, "URL: %s\n", source.URL)
	fmt.Fprintf(metadataFile, "License: %s\n", source.License)
	fmt.Fprintf(metadataFile, "Size: %d\n", written)
	fmt.Fprintf(metadataFile, "Downloaded: %s\n", time.Now().Format(time.RFC3339))
	for key, value := range source.Metadata {
		fmt.Fprintf(metadataFile, "%s: %s\n", key, value)
	}

	return nil
}

// PreviewDataset shows information about the dataset without downloading
func (g *DatasetGenerator) PreviewDataset() error {
	dataset, exists := g.datasets[g.config.Dataset]
	if !exists {
		return fmt.Errorf("unknown dataset: %s", g.config.Dataset)
	}

	fmt.Printf("Dataset: %s\n", dataset.Name)
	fmt.Printf("Description: %s\n", dataset.Description)
	fmt.Printf("Sources: %d\n", len(dataset.Sources))
	fmt.Printf("Directory: %s\n", dataset.Directory)
	
	totalSize := int64(0)
	fileTypes := make(map[string]int)
	
	for _, source := range dataset.Sources {
		totalSize += source.Size
		fileTypes[source.Type]++
	}
	
	fmt.Printf("Estimated Total Size: %.2f MB\n", float64(totalSize)/(1024*1024))
	fmt.Printf("File Types:\n")
	for ftype, count := range fileTypes {
		fmt.Printf("  %s: %d files\n", ftype, count)
	}
	
	return nil
}

// GetSummary returns statistics about the generated dataset
func (g *DatasetGenerator) GetSummary() (*Summary, error) {
	summary := &Summary{
		FileTypes: make([]string, 0),
	}

	if _, err := os.Stat(g.config.OutputDir); os.IsNotExist(err) {
		return summary, nil
	}

	fileTypes := make(map[string]bool)
	
	err := filepath.Walk(g.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			summary.DirectoryCount++
			return nil
		}

		// Skip metadata files
		if filepath.Ext(path) == ".meta" {
			return nil
		}

		summary.TotalFiles++
		summary.TotalSize += info.Size()
		
		ext := filepath.Ext(path)
		if ext != "" {
			fileTypes[ext] = true
		}
		
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	for ftype := range fileTypes {
		summary.FileTypes = append(summary.FileTypes, ftype)
	}

	return summary, nil
}

// ListAvailableDatasets prints all available datasets
func ListAvailableDatasets() {
	datasets := getBuiltinDatasets()
	
	fmt.Printf("Available Datasets:\n")
	fmt.Printf("==================\n")
	
	for name, dataset := range datasets {
		fmt.Printf("\n%s:\n", name)
		fmt.Printf("  Description: %s\n", dataset.Description)
		fmt.Printf("  Sources: %d\n", len(dataset.Sources))
		fmt.Printf("  Directory: %s\n", dataset.Directory)
		
		totalSize := int64(0)
		for _, source := range dataset.Sources {
			totalSize += source.Size
		}
		fmt.Printf("  Estimated Size: %.2f MB\n", float64(totalSize)/(1024*1024))
	}
}