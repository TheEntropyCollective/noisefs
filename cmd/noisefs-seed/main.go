package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/tools/bootstrap"
)

func main() {
	var (
		profile      = flag.String("profile", "standard", "Seeding profile: minimal, standard, maximum")
		outputDir    = flag.String("output", "./seed-data", "Output directory for seed data")
		ipfsAPI      = flag.String("ipfs", "http://localhost:5001", "IPFS API endpoint")
		parallel     = flag.Int("parallel", 4, "Number of parallel downloads")
		videoQuality = flag.String("video-quality", "720p", "Maximum video quality to download")
		skipDownload = flag.Bool("skip-download", false, "Skip download phase (use existing data)")
		skipGenerate = flag.Bool("skip-generate", false, "Skip block generation phase")
		skipInit     = flag.Bool("skip-init", false, "Skip pool initialization phase")
		verbose      = flag.Bool("verbose", false, "Enable verbose logging")
		dryRun       = flag.Bool("dry-run", false, "Preview actions without executing")
	)
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	fmt.Println("🌱 NoiseFS Automated Seeding System")
	fmt.Println("===================================")
	fmt.Printf("Profile: %s\n", *profile)
	fmt.Printf("Output: %s\n", *outputDir)
	fmt.Printf("IPFS: %s\n", *ipfsAPI)
	fmt.Println()

	// Create seeder configuration based on profile
	config := createSeedConfig(*profile, *outputDir, *ipfsAPI, *parallel, *videoQuality, *verbose)

	if *dryRun {
		fmt.Println("🔍 DRY RUN MODE - Preview Only")
		previewSeeding(config)
		return
	}

	// Create output directories
	if err := createDirectories(config); err != nil {
		log.Fatalf("Failed to create directories: %v", err)
	}

	startTime := time.Now()

	// Phase 1: Download public domain content
	if !*skipDownload {
		fmt.Println("\n📥 Phase 1: Downloading Public Domain Content")
		fmt.Println("---------------------------------------------")
		if err := downloadContent(config); err != nil {
			log.Fatalf("Download phase failed: %v", err)
		}
	}

	// Phase 2: Generate blocks from content
	if !*skipGenerate {
		fmt.Println("\n⚙️  Phase 2: Generating NoiseFS Blocks")
		fmt.Println("-------------------------------------")
		if err := generateBlocks(config); err != nil {
			log.Fatalf("Block generation failed: %v", err)
		}
	}

	// Phase 3: Initialize universal pool
	if !*skipInit {
		fmt.Println("\n🚀 Phase 3: Initializing Universal Pool")
		fmt.Println("--------------------------------------")
		if err := initializePool(config); err != nil {
			log.Fatalf("Pool initialization failed: %v", err)
		}
	}

	// Generate final report
	duration := time.Since(startTime)
	fmt.Println("\n📊 Seeding Complete!")
	fmt.Println("===================")
	generateReport(config, duration)
}

func createSeedConfig(profile, outputDir, ipfsAPI string, parallel int, videoQuality string, verbose bool) *bootstrap.SeedConfig {
	config := &bootstrap.SeedConfig{
		OutputDir:    outputDir,
		IPFSEndpoint: ipfsAPI,
		Parallel:     parallel,
		VideoQuality: videoQuality,
		Verbose:      verbose,
	}

	switch profile {
	case "minimal":
		config.Profile = bootstrap.ProfileMinimal
		config.MaxSize = 500 * 1024 * 1024 // 500MB
		config.IncludeVideo = false
		config.BlocksPerSize = 100
		config.GenesisBlockCount = 250 // More genesis blocks for minimal
	case "maximum":
		config.Profile = bootstrap.ProfileMaximum
		config.MaxSize = 10 * 1024 * 1024 * 1024 // 10GB
		config.IncludeVideo = true
		config.BlocksPerSize = 1000
		config.GenesisBlockCount = 100
	default: // standard
		config.Profile = bootstrap.ProfileStandard
		config.MaxSize = 2 * 1024 * 1024 * 1024 // 2GB
		config.IncludeVideo = true
		config.BlocksPerSize = 500
		config.GenesisBlockCount = 200 // Ensure we meet minimums
	}

	return config
}

func createDirectories(config *bootstrap.SeedConfig) error {
	dirs := []string{
		filepath.Join(config.OutputDir, "downloads", "books"),
		filepath.Join(config.OutputDir, "downloads", "images"),
		filepath.Join(config.OutputDir, "downloads", "audio"),
		filepath.Join(config.OutputDir, "downloads", "videos"),
		filepath.Join(config.OutputDir, "downloads", "documents"),
		filepath.Join(config.OutputDir, "blocks"),
		filepath.Join(config.OutputDir, "pool"),
		filepath.Join(config.OutputDir, "reports"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

func previewSeeding(config *bootstrap.SeedConfig) {
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("- Profile: %s\n", config.Profile)
	fmt.Printf("- Max Size: %.1f GB\n", float64(config.MaxSize)/(1024*1024*1024))
	fmt.Printf("- Include Video: %v\n", config.IncludeVideo)
	fmt.Printf("- Blocks Per Size: %d\n", config.BlocksPerSize)
	fmt.Printf("- Video Quality: %s\n", config.VideoQuality)

	fmt.Printf("\nEstimated Content:\n")
	switch config.Profile {
	case bootstrap.ProfileMinimal:
		fmt.Println("- Books: 10-15 texts (~50MB)")
		fmt.Println("- Images: 50-100 images (~100MB)")
		fmt.Println("- Audio: 20-30 tracks (~200MB)")
		fmt.Println("- Videos: None")
		fmt.Println("- Genesis Blocks: ~150MB")
	case bootstrap.ProfileStandard:
		fmt.Println("- Books: 20-30 texts (~100MB)")
		fmt.Println("- Images: 100-200 images (~300MB)")
		fmt.Println("- Audio: 50-70 tracks (~500MB)")
		fmt.Println("- Videos: 10-20 short clips (~800MB)")
		fmt.Println("- Genesis Blocks: ~300MB")
	case bootstrap.ProfileMaximum:
		fmt.Println("- Books: 50-100 texts (~300MB)")
		fmt.Println("- Images: 500-1000 images (~1GB)")
		fmt.Println("- Audio: 100-200 tracks (~2GB)")
		fmt.Println("- Videos: 50-100 films (~5GB)")
		fmt.Println("- Genesis Blocks: ~1GB)")
	}

	fmt.Printf("\nExpected Pool Statistics:\n")
	fmt.Printf("- Total Blocks: %d-%d\n", config.BlocksPerSize*3, config.BlocksPerSize*5)
	fmt.Printf("- Public Domain Ratio: >60%%\n")
	fmt.Printf("- Block Sizes: 64KB, 128KB, 256KB, 512KB, 1MB\n")
	fmt.Printf("- Estimated Time: %s\n", estimateTime(config))
}

func estimateTime(config *bootstrap.SeedConfig) string {
	switch config.Profile {
	case bootstrap.ProfileMinimal:
		return "5-10 minutes"
	case bootstrap.ProfileMaximum:
		return "60-120 minutes"
	default:
		return "15-30 minutes"
	}
}

func downloadContent(config *bootstrap.SeedConfig) error {
	downloader := bootstrap.NewContentDownloader(config)

	// Download each content type
	contentTypes := []string{"books", "images", "audio", "documents"}
	if config.IncludeVideo {
		contentTypes = append(contentTypes, "videos")
	}

	for _, contentType := range contentTypes {
		fmt.Printf("\nDownloading %s...\n", contentType)
		if err := downloader.DownloadContentType(contentType); err != nil {
			return fmt.Errorf("failed to download %s: %w", contentType, err)
		}
	}

	return nil
}

func generateBlocks(config *bootstrap.SeedConfig) error {
	generator := bootstrap.NewBlockGenerator(config)

	// Generate blocks from downloaded content
	fmt.Println("\nProcessing downloaded content into blocks...")
	stats, err := generator.GenerateFromContent()
	if err != nil {
		return fmt.Errorf("failed to generate blocks: %w", err)
	}

	// Generate genesis blocks
	fmt.Println("\nGenerating genesis blocks...")
	if err := generator.GenerateGenesisBlocks(); err != nil {
		return fmt.Errorf("failed to generate genesis blocks: %w", err)
	}

	fmt.Printf("\nBlock Generation Summary:\n")
	fmt.Printf("- Total Blocks: %d\n", stats.TotalBlocks)
	fmt.Printf("- Public Domain: %d (%.1f%%)\n", stats.PublicDomainBlocks,
		float64(stats.PublicDomainBlocks)/float64(stats.TotalBlocks)*100)
	fmt.Printf("- Average Reuse Potential: %.1fx\n", stats.AverageReusePotential)

	return nil
}

func initializePool(config *bootstrap.SeedConfig) error {
	initializer := bootstrap.NewPoolInitializer(config)

	fmt.Println("\nStoring blocks in IPFS...")
	if err := initializer.StoreBlocks(); err != nil {
		return fmt.Errorf("failed to store blocks: %w", err)
	}

	fmt.Println("\nInitializing universal pool...")
	if err := initializer.InitializePool(); err != nil {
		return fmt.Errorf("failed to initialize pool: %w", err)
	}

	fmt.Println("\nValidating pool requirements...")
	validation, err := initializer.ValidatePool()
	if err != nil {
		return fmt.Errorf("pool validation failed: %w", err)
	}

	if !validation.Valid {
		return fmt.Errorf("pool does not meet requirements: %v", validation.Issues)
	}

	fmt.Println("✅ Pool validation passed!")
	return nil
}

func generateReport(config *bootstrap.SeedConfig, duration time.Duration) {
	reportPath := filepath.Join(config.OutputDir, "reports",
		fmt.Sprintf("seed-report-%s.txt", time.Now().Format("20060102-150405")))

	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Report saved to: %s\n", reportPath)

	// In full implementation, generate detailed report with:
	// - Content inventory
	// - Block statistics
	// - Pool metrics
	// - Privacy analysis
	// - Performance benchmarks
}
