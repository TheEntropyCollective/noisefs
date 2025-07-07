package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

func main() {
	var (
		ipfsAPI = flag.String("api", "localhost:5001", "IPFS API endpoint")
		upload  = flag.String("upload", "", "File to upload to NoiseFS")
		download = flag.String("download", "", "Descriptor CID to download from NoiseFS")
		output = flag.String("output", "", "Output file path for download")
		blockSize = flag.Int("block-size", blocks.DefaultBlockSize, "Block size in bytes")
		cacheSize = flag.Int("cache-size", 1000, "Number of blocks to cache in memory")
	)
	
	flag.Parse()
	
	// Create IPFS client
	ipfsClient, err := ipfs.NewClient(*ipfsAPI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to IPFS: %v\n", err)
		os.Exit(1)
	}
	
	// Create cache
	blockCache := cache.NewMemoryCache(*cacheSize)
	
	// Create NoiseFS client
	client, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create NoiseFS client: %v\n", err)
		os.Exit(1)
	}
	
	if *upload != "" {
		if err := uploadFile(ipfsClient, client, *upload, *blockSize); err != nil {
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
	
	// Generate or select randomizer blocks
	randomizerBlocks := make([]*blocks.Block, len(fileBlocks))
	randomizerCIDs := make([]string, len(fileBlocks))
	
	fmt.Println("Selecting randomizer blocks...")
	for i := range fileBlocks {
		randBlock, cid, err := client.SelectRandomizer(fileBlocks[i].Size())
		if err != nil {
			return fmt.Errorf("failed to select randomizer block: %w", err)
		}
		randomizerBlocks[i] = randBlock
		randomizerCIDs[i] = cid
	}
	
	// XOR blocks with randomizers
	anonymizedBlocks := make([]*blocks.Block, len(fileBlocks))
	for i := range fileBlocks {
		xorBlock, err := fileBlocks[i].XOR(randomizerBlocks[i])
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
	
	// Add block pairs to descriptor
	for i := range dataCIDs {
		if err := descriptor.AddBlockPair(dataCIDs[i], randomizerCIDs[i]); err != nil {
			return fmt.Errorf("failed to add block pair to descriptor: %w", err)
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
	client.RecordUpload(fileInfo.Size(), totalStoredBytes*2) // *2 for data + randomizer blocks
	
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
	randomizerCIDs := make([]string, len(descriptor.Blocks))
	
	for i, block := range descriptor.Blocks {
		dataCIDs[i] = block.DataCID
		randomizerCIDs[i] = block.RandomizerCID
	}
	
	// Retrieve anonymized data blocks
	fmt.Println("Retrieving anonymized data blocks...")
	dataBlocks, err := ipfsClient.RetrieveBlocks(dataCIDs)
	if err != nil {
		return fmt.Errorf("failed to retrieve data blocks: %w", err)
	}
	
	// Retrieve randomizer blocks
	fmt.Println("Retrieving randomizer blocks...")
	randomizerBlocks, err := ipfsClient.RetrieveBlocks(randomizerCIDs)
	if err != nil {
		return fmt.Errorf("failed to retrieve randomizer blocks: %w", err)
	}
	
	// XOR blocks to reconstruct original data
	fmt.Println("Reconstructing original blocks...")
	originalBlocks := make([]*blocks.Block, len(dataBlocks))
	for i := range dataBlocks {
		origBlock, err := dataBlocks[i].XOR(randomizerBlocks[i])
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