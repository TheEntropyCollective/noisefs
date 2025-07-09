package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/TheEntropyCollective/noisefs/pkg/bootstrap"
)

func main() {
	var (
		configFile   = flag.String("config", "config.json", "Configuration file path")
		outputDir    = flag.String("output", "./bootstrap_data", "Output directory for downloaded content")
		dataset      = flag.String("dataset", "mixed", "Dataset to download (mixed, books, images, documents, code)")
		maxSize      = flag.Int64("max-size", 500*1024*1024, "Maximum total size in bytes (default: 500MB)")
		verbose      = flag.Bool("verbose", false, "Enable verbose logging")
		listDatasets = flag.Bool("list", false, "List available datasets")
		preview      = flag.Bool("preview", false, "Preview dataset without downloading")
		parallel     = flag.Int("parallel", 4, "Number of parallel downloads")
	)
	flag.Parse()

	if *listDatasets {
		bootstrap.ListAvailableDatasets()
		return
	}

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	fmt.Printf("NoiseFS Bootstrap Data Generator\n")
	fmt.Printf("================================\n")
	fmt.Printf("Dataset: %s\n", *dataset)
	fmt.Printf("Output: %s\n", *outputDir)
	fmt.Printf("Max Size: %d MB\n", *maxSize/(1024*1024))
	fmt.Printf("Parallel Downloads: %d\n", *parallel)
	fmt.Printf("\n")

	config := &bootstrap.Config{
		ConfigFile:        *configFile,
		OutputDir:         *outputDir,
		Dataset:           *dataset,
		MaxSize:           *maxSize,
		Verbose:           *verbose,
		ParallelDownloads: *parallel,
	}

	generator := bootstrap.NewDatasetGenerator(config)

	if *preview {
		fmt.Printf("Previewing dataset '%s'...\n", *dataset)
		err := generator.PreviewDataset()
		if err != nil {
			log.Fatalf("Failed to preview dataset: %v", err)
		}
		return
	}

	fmt.Printf("Downloading dataset '%s'...\n", *dataset)
	err := generator.GenerateDataset()
	if err != nil {
		log.Fatalf("Failed to generate dataset: %v", err)
	}

	fmt.Printf("\nBootstrap data generation completed successfully!\n")
	fmt.Printf("Files saved to: %s\n", *outputDir)
	
	// Display summary
	summary, err := generator.GetSummary()
	if err != nil {
		log.Printf("Warning: Could not generate summary: %v", err)
	} else {
		fmt.Printf("\nSummary:\n")
		fmt.Printf("- Total Files: %d\n", summary.TotalFiles)
		fmt.Printf("- Total Size: %.2f MB\n", float64(summary.TotalSize)/(1024*1024))
		fmt.Printf("- File Types: %v\n", summary.FileTypes)
		fmt.Printf("- Directory Structure: %d directories\n", summary.DirectoryCount)
	}
}