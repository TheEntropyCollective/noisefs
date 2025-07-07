package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/config"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

func main() {
	var (
		configFile = flag.String("config", "", "Configuration file path")
		ipfsAPI    = flag.String("api", "", "IPFS API endpoint (overrides config)")
		upload     = flag.String("upload", "", "File to upload to NoiseFS")
		download   = flag.String("download", "", "Descriptor CID to download from NoiseFS")
		output     = flag.String("output", "", "Output file path for download")
		blockSize  = flag.Int("block-size", 0, "Block size in bytes (overrides config)")
		cacheSize  = flag.Int("cache-size", 0, "Number of blocks to cache in memory (overrides config)")
	)
	
	flag.Parse()
	
	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}
	
	// Apply command-line overrides
	if *ipfsAPI != "" {
		cfg.IPFS.APIEndpoint = *ipfsAPI
	}
	if *blockSize != 0 {
		cfg.Performance.BlockSize = *blockSize
	}
	if *cacheSize != 0 {
		cfg.Cache.BlockCacheSize = *cacheSize
	}
	
	// Create IPFS client
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.APIEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to IPFS: %v\n", err)
		os.Exit(1)
	}
	
	// Create cache
	blockCache := cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	
	// Create NoiseFS client
	client, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create NoiseFS client: %v\n", err)
		os.Exit(1)
	}
	
	if *upload != "" {
		if err := uploadFile(ipfsClient, client, *upload, cfg.Performance.BlockSize); err != nil {
			fmt.Fprintf(os.Stderr, "Upload failed: %v\n", err)
			os.Exit(1)
		}
		showMetrics(client)
	} else if *download != "" {
		if *output == "" {
			fmt.Fprintf(os.Stderr, "Output file path required for download\n")
			os.Exit(1)
		}
		if err := downloadFile(ipfsClient, client, *download, *output); err != nil {
			fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
			os.Exit(1)
		}
		showMetrics(client)
	} else {
		flag.Usage()
	}
}

// loadConfig loads configuration from file or uses defaults
func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		// Try default config path
		defaultPath, err := config.GetDefaultConfigPath()
		if err == nil {
			configPath = defaultPath
		}
	}
	
	return config.LoadConfig(configPath)
}

func uploadFile(ipfsClient *ipfs.Client, client *noisefs.Client, filePath string, blockSize int) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return fmt.Errorf("failed to create splitter: %w", err)
	}
	
	// Split file into blocks
	fmt.Printf("Splitting file into %d byte blocks...\n", blockSize)
	fileBlocks, err := splitter.Split(file)
	if err != nil {
		return fmt.Errorf("failed to split file: %w", err)
	}
	
	fmt.Printf("Created %d blocks\n", len(fileBlocks))
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(
		filepath.Base(filePath),
		fileInfo.Size(),
		blockSize,
	)
	
	// Generate or select randomizer blocks (using 3-tuple format)
	randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer1CIDs := make([]string, len(fileBlocks))
	randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
	randomizer2CIDs := make([]string, len(fileBlocks))
	
	fmt.Println("Selecting randomizer blocks (3-tuple format)...")
	for i := range fileBlocks {
		randBlock1, cid1, randBlock2, cid2, err := client.SelectTwoRandomizers(fileBlocks[i].Size())
		if err != nil {
			return fmt.Errorf("failed to select randomizer blocks: %w", err)
		}
		randomizer1Blocks[i] = randBlock1
		randomizer1CIDs[i] = cid1
		randomizer2Blocks[i] = randBlock2
		randomizer2CIDs[i] = cid2
	}
	
	// XOR blocks with randomizers (3-tuple: data XOR randomizer1 XOR randomizer2)
	anonymizedBlocks := make([]*blocks.Block, len(fileBlocks))
	for i := range fileBlocks {
		xorBlock, err := fileBlocks[i].XOR3(randomizer1Blocks[i], randomizer2Blocks[i])
		if err != nil {
			return fmt.Errorf("failed to XOR blocks: %w", err)
		}
		anonymizedBlocks[i] = xorBlock
	}
	
	// Store anonymized blocks in IPFS with caching
	fmt.Println("Storing anonymized blocks in IPFS...")
	dataCIDs := make([]string, len(anonymizedBlocks))
	for i, block := range anonymizedBlocks {
		cid, err := client.StoreBlockWithCache(block)
		if err != nil {
			return fmt.Errorf("failed to store data block %d: %w", i, err)
		}
		dataCIDs[i] = cid
	}
	
	// Add block triples to descriptor (3-tuple format)
	for i := range dataCIDs {
		if err := descriptor.AddBlockTriple(dataCIDs[i], randomizer1CIDs[i], randomizer2CIDs[i]); err != nil {
			return fmt.Errorf("failed to add block triple to descriptor: %w", err)
		}
	}
	
	// Store descriptor in IPFS
	store, err := descriptors.NewStore(ipfsClient)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptorCID, err := store.Save(descriptor)
	if err != nil {
		return fmt.Errorf("failed to store descriptor: %w", err)
	}
	
	// Display results
	fmt.Println("\nUpload complete!")
	fmt.Printf("Descriptor CID: %s\n", descriptorCID)
	fmt.Println("\nDescriptor content:")
	
	descJSON, _ := descriptor.ToJSON()
	fmt.Println(string(descJSON))
	
	// Record upload metrics
	totalStoredBytes := int64(0)
	for _, block := range anonymizedBlocks {
		totalStoredBytes += int64(len(block.Data))
	}
	// Add randomizer blocks size (they're stored but already exist)
	// For 3-tuple: data + randomizer1 + randomizer2 = 3x the data size
	client.RecordUpload(fileInfo.Size(), totalStoredBytes*3) // *3 for data + 2 randomizer blocks
	
	return nil
}

func downloadFile(ipfsClient *ipfs.Client, client *noisefs.Client, descriptorCID string, outputPath string) error {
	// Create descriptor store
	store, err := descriptors.NewStore(ipfsClient)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Load descriptor from IPFS
	fmt.Printf("Loading descriptor from CID: %s\n", descriptorCID)
	descriptor, err := store.Load(descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	fmt.Printf("Downloading file: %s (%d bytes)\n", descriptor.Filename, descriptor.FileSize)
	fmt.Printf("Blocks to retrieve: %d\n", len(descriptor.Blocks))
	
	// Retrieve all data blocks
	dataCIDs := make([]string, len(descriptor.Blocks))
	randomizer1CIDs := make([]string, len(descriptor.Blocks))
	randomizer2CIDs := make([]string, len(descriptor.Blocks))
	
	for i, block := range descriptor.Blocks {
		dataCIDs[i] = block.DataCID
		randomizer1CIDs[i] = block.RandomizerCID1
		if descriptor.IsThreeTuple() {
			randomizer2CIDs[i] = block.RandomizerCID2
		}
	}
	
	// Retrieve anonymized data blocks
	fmt.Println("Retrieving anonymized data blocks...")
	dataBlocks, err := ipfsClient.RetrieveBlocks(dataCIDs)
	if err != nil {
		return fmt.Errorf("failed to retrieve data blocks: %w", err)
	}
	
	// Retrieve first randomizer blocks
	fmt.Println("Retrieving randomizer blocks...")
	randomizer1Blocks, err := ipfsClient.RetrieveBlocks(randomizer1CIDs)
	if err != nil {
		return fmt.Errorf("failed to retrieve randomizer1 blocks: %w", err)
	}
	
	// Retrieve second randomizer blocks if using 3-tuple format
	var randomizer2Blocks []*blocks.Block
	if descriptor.IsThreeTuple() {
		randomizer2Blocks, err = ipfsClient.RetrieveBlocks(randomizer2CIDs)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer2 blocks: %w", err)
		}
	}
	
	// XOR blocks to reconstruct original data
	fmt.Println("Reconstructing original blocks...")
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		var origBlock *blocks.Block
		var err error
		
		if descriptor.IsThreeTuple() && randomizer2Blocks != nil {
			// Use 3-tuple XOR for version 2.0
			origBlock, err = dataBlocks[i].XOR3(randomizer1Blocks[i], randomizer2Blocks[i])
		} else {
			// Use 2-tuple XOR for legacy format
			origBlock, err = dataBlocks[i].XOR(randomizer1Blocks[i])
		}
		
		if err != nil {
			return fmt.Errorf("failed to XOR blocks: %w", err)
		}
		originalBlocks[i] = origBlock
	}
	
	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()
	
	// Assemble file
	assembler := blocks.NewAssembler()
	if err := assembler.AssembleToWriter(originalBlocks, outputFile); err != nil {
		return fmt.Errorf("failed to assemble file: %w", err)
	}
	
	fmt.Printf("\nDownload complete! File saved to: %s\n", outputPath)
	
	// Record download
	client.RecordDownload()
	
	return nil
}

// showMetrics displays current NoiseFS metrics
func showMetrics(client *noisefs.Client) {
	metrics := client.GetMetrics()
	
	fmt.Println("\n--- NoiseFS Metrics ---")
	fmt.Printf("Block Reuse Rate: %.1f%% (%d reused, %d generated)\n", 
		metrics.BlockReuseRate, metrics.BlocksReused, metrics.BlocksGenerated)
	fmt.Printf("Cache Hit Rate: %.1f%% (%d hits, %d misses)\n", 
		metrics.CacheHitRate, metrics.CacheHits, metrics.CacheMisses)
	fmt.Printf("Storage Efficiency: %.1f%% overhead\n", metrics.StorageEfficiency)
	fmt.Printf("Total Operations: %d uploads, %d downloads\n", 
		metrics.TotalUploads, metrics.TotalDownloads)
	
	if metrics.BytesUploadedOriginal > 0 {
		fmt.Printf("Data: %d bytes original → %d bytes stored\n", 
			metrics.BytesUploadedOriginal, metrics.BytesStoredIPFS)
	}
}