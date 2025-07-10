package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	fixtures "github.com/TheEntropyCollective/noisefs/tests/fixtures"
)

func main() {
	var (
		nodes     = flag.Int("nodes", 1, "Number of IPFS nodes (1=single-node, 2+=multi-node)")
		fileSize  = flag.Int("file-size", 65536, "Test file size in bytes")
		numFiles  = flag.Int("files", 10, "Number of files to test")
		verbose   = flag.Bool("verbose", false, "Verbose output")
		help      = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		fmt.Println("NoiseFS Performance Benchmark")
		fmt.Println("=============================")
		fmt.Println("Simple unified performance testing for NoiseFS")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  go run cmd/benchmarks/benchmark/main.go                    # Quick single-node test")
		fmt.Println("  go run cmd/benchmarks/benchmark/main.go -nodes 3 -verbose  # Multi-node cluster test")
		fmt.Println("  go run cmd/benchmarks/benchmark/main.go -files 50          # Stress test")
		fmt.Println()
		flag.PrintDefaults()
		return
	}

	fmt.Println("🚀 NoiseFS Performance Benchmark")
	fmt.Println("=================================")
	
	if *nodes == 1 {
		fmt.Println("Mode: Single-node testing")
		runSingleNodeBenchmark(*fileSize, *numFiles, *verbose)
	} else if *nodes == 2 {
		fmt.Printf("Mode: Hybrid multi-node testing (existing + %d new nodes)\n", *nodes-1)
		runHybridMultiNodeBenchmark(*nodes-1, *fileSize, *numFiles, *verbose)
	} else {
		fmt.Printf("Mode: Full multi-node testing (%d nodes)\n", *nodes)
		runMultiNodeBenchmark(*nodes, *fileSize, *numFiles, *verbose)
	}
}

func runSingleNodeBenchmark(fileSize, numFiles int, verbose bool) {
	fmt.Printf("File size: %d bytes\n", fileSize)
	fmt.Printf("Number of files: %d\n", numFiles)
	fmt.Println()

	// Check if IPFS is running
	ipfsAPI := "127.0.0.1:5001"
	if !isIPFSRunning(ipfsAPI) {
		fmt.Println("❌ IPFS is not running. Please start IPFS first:")
		fmt.Println("   ipfs daemon")
		os.Exit(1)
	}

	// Create client
	ipfsClient, err := ipfs.NewClient(ipfsAPI)
	if err != nil {
		log.Fatalf("Failed to create IPFS client: %v", err)
	}

	cache := cache.NewMemoryCache(100)
	noiseClient, err := noisefs.NewClient(ipfsClient, cache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	fmt.Println("✅ Connected to IPFS")
	fmt.Printf("📊 Running performance test with %d files...\n", numFiles)
	
	// Run tests
	results := runTests(noiseClient, fileSize, numFiles, verbose, "Node 1")
	
	// Print results
	printSingleNodeResults(results, time.Duration(0))
}

func runHybridMultiNodeBenchmark(newNodeCount, fileSize, numFiles int, verbose bool) {
	fmt.Printf("File size: %d bytes\n", fileSize)
	fmt.Printf("Number of files: %d\n", numFiles)
	fmt.Println()

	// Check existing IPFS
	ipfsAPI := "127.0.0.1:5001"
	if !isIPFSRunning(ipfsAPI) {
		fmt.Println("❌ IPFS is not running. Please start IPFS first:")
		fmt.Println("   ipfs daemon")
		os.Exit(1)
	}

	// Create client for existing IPFS
	ipfsClient, err := ipfs.NewClient(ipfsAPI)
	if err != nil {
		log.Fatalf("Failed to create IPFS client: %v", err)
	}

	existingCache := cache.NewMemoryCache(100)
	existingClient, err := noisefs.NewClient(ipfsClient, existingCache)
	if err != nil {
		log.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	fmt.Println("✅ Connected to existing IPFS")

	// Start additional nodes
	launcher := fixtures.NewMultiNodeLauncher(newNodeCount)
	defer launcher.StopAllNodes()

	startTime := time.Now()
	err = launcher.StartNodes()
	if err != nil {
		log.Printf("Warning: Failed to start additional nodes: %v", err)
		fmt.Println("Falling back to single-node testing...")
		runSingleNodeBenchmark(fileSize, numFiles, verbose)
		return
	}

	// Create clients for new nodes
	clients := []*noisefs.Client{existingClient} // Start with existing node
	nodeClients := launcher.GetNodes()
	
	fmt.Println("🔌 Creating NoiseFS clients for new nodes...")
	for _, node := range nodeClients {
		ipfsClient, err := ipfs.NewClient(node.APIAddress)
		if err != nil {
			fmt.Printf("  ⚠️  Failed to connect to node %d: %v\n", node.ID, err)
			continue
		}

		nodeCache := cache.NewMemoryCache(100)
		noiseClient, err := noisefs.NewClient(ipfsClient, nodeCache)
		if err != nil {
			fmt.Printf("  ⚠️  Failed to create NoiseFS client for node %d: %v\n", node.ID, err)
			continue
		}

		clients = append(clients, noiseClient)
		if verbose {
			fmt.Printf("  ✅ Node %d ready\n", node.ID)
		}
	}

	totalNodes := len(clients)
	fmt.Printf("✅ %d total nodes ready (1 existing + %d new)\n", totalNodes, totalNodes-1)

	// Run tests
	fmt.Println("\n📊 Phase 1: Single Node Performance")
	singleResults := runTests(clients[0], fileSize, numFiles, verbose, "Existing Node")

	var crossResults []TestResult
	if len(clients) > 1 {
		fmt.Println("\n📊 Phase 2: Cross-Node Replication")
		crossResults = runCrossNodeTests(clients, fileSize, numFiles/2, verbose)
	}

	setupTime := time.Since(startTime)
	printMultiNodeResults(singleResults, crossResults, setupTime, totalNodes)
}

func runMultiNodeBenchmark(nodeCount, fileSize, numFiles int, verbose bool) {
	fmt.Printf("File size: %d bytes\n", fileSize)
	fmt.Printf("Number of files: %d\n", numFiles)
	fmt.Println()

	// Create and start multi-node launcher
	launcher := fixtures.NewMultiNodeLauncher(nodeCount)

	// Setup cleanup
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\n⚠️  Interrupt received, cleaning up...")
		launcher.StopAllNodes()
		os.Exit(1)
	}()

	defer func() {
		launcher.StopAllNodes()
	}()

	// Start nodes
	fmt.Printf("⚙️  Starting %d IPFS nodes...\n", nodeCount)
	startTime := time.Now()
	
	err := launcher.StartNodes()
	if err != nil {
		log.Fatalf("Failed to start nodes: %v", err)
	}

	// Create clients
	clients := make([]*noisefs.Client, 0)
	nodeClients := launcher.GetNodes()
	
	fmt.Println("🔌 Creating NoiseFS clients...")
	for _, node := range nodeClients {
		ipfsClient, err := ipfs.NewClient(node.APIAddress)
		if err != nil {
			fmt.Printf("  ⚠️  Failed to connect to node %d: %v\n", node.ID, err)
			continue
		}

		nodeCache := cache.NewMemoryCache(100)
		noiseClient, err := noisefs.NewClient(ipfsClient, nodeCache)
		if err != nil {
			fmt.Printf("  ⚠️  Failed to create NoiseFS client for node %d: %v\n", node.ID, err)
			continue
		}

		clients = append(clients, noiseClient)
		if verbose {
			fmt.Printf("  ✅ Node %d ready\n", node.ID)
		}
	}

	if len(clients) == 0 {
		log.Fatalf("No working clients - benchmark failed")
	}

	fmt.Printf("✅ %d nodes ready\n", len(clients))

	// Run single-node test
	fmt.Println("\n📊 Phase 1: Single Node Performance")
	singleResults := runTests(clients[0], fileSize, numFiles, verbose, "Node 1")

	// Run cross-node test if multiple nodes
	var crossResults []TestResult
	if len(clients) > 1 {
		fmt.Println("\n📊 Phase 2: Cross-Node Replication")
		crossResults = runCrossNodeTests(clients, fileSize, numFiles/2, verbose)
		
		// Run concurrent test
		fmt.Println("\n📊 Phase 3: Concurrent Multi-Node Testing")
		concurrentResults := runConcurrentTests(clients, fileSize, numFiles, verbose)
		crossResults = append(crossResults, concurrentResults...)
	}

	setupTime := time.Since(startTime)
	
	// Print results
	printMultiNodeResults(singleResults, crossResults, setupTime, len(clients))
}

type TestResult struct {
	TestName        string
	FileSize        int
	UploadLatency   time.Duration
	DownloadLatency time.Duration
	Success         bool
	CID             string
	NodeInfo        string
}

func runTests(client *noisefs.Client, fileSize, numFiles int, verbose bool, nodeInfo string) []TestResult {
	results := make([]TestResult, 0, numFiles)
	
	for i := 0; i < numFiles; i++ {
		testData := make([]byte, fileSize)
		rand.Read(testData)

		testName := fmt.Sprintf("file_%d", i+1)
		if verbose {
			fmt.Printf("  Testing %s (%d bytes)...\n", testName, fileSize)
		}

		// Create block
		block, err := blocks.NewBlock(testData)
		if err != nil {
			results = append(results, TestResult{TestName: testName, Success: false})
			continue
		}

		// Upload
		uploadStart := time.Now()
		cid, err := client.StoreBlockWithCache(block)
		uploadLatency := time.Since(uploadStart)

		if err != nil {
			if verbose {
				fmt.Printf("    ❌ Upload failed: %v\n", err)
			}
			results = append(results, TestResult{TestName: testName, Success: false})
			continue
		}

		// Download
		downloadStart := time.Now()
		retrievedBlock, err := client.RetrieveBlockWithCache(cid)
		downloadLatency := time.Since(downloadStart)

		if err != nil {
			if verbose {
				fmt.Printf("    ❌ Download failed: %v\n", err)
			}
			results = append(results, TestResult{TestName: testName, Success: false})
			continue
		}

		// Verify
		success := equalBytes(retrievedBlock.Data, testData)

		result := TestResult{
			TestName:        testName,
			FileSize:        fileSize,
			UploadLatency:   uploadLatency,
			DownloadLatency: downloadLatency,
			Success:         success,
			CID:             cid,
			NodeInfo:        nodeInfo,
		}
		results = append(results, result)

		if verbose {
			fmt.Printf("    ✅ Upload: %v, Download: %v, CID: %s\n", 
				uploadLatency, downloadLatency, cid[:12]+"...")
		}
	}

	return results
}

func runCrossNodeTests(clients []*noisefs.Client, fileSize, numTests int, verbose bool) []TestResult {
	results := make([]TestResult, 0, numTests)
	
	for i := 0; i < numTests; i++ {
		sourceIdx := i % len(clients)
		targetIdx := (i + 1) % len(clients)

		testData := make([]byte, fileSize)
		rand.Read(testData)

		testName := fmt.Sprintf("cross_node_%d", i+1)
		nodeInfo := fmt.Sprintf("Node %d -> Node %d", sourceIdx+1, targetIdx+1)
		
		if verbose {
			fmt.Printf("  Testing %s (%s)...\n", testName, nodeInfo)
		}

		// Upload to source
		block, err := blocks.NewBlock(testData)
		if err != nil {
			results = append(results, TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo})
			continue
		}

		cid, err := clients[sourceIdx].StoreBlockWithCache(block)
		if err != nil {
			if verbose {
				fmt.Printf("    ❌ Upload to source failed: %v\n", err)
			}
			results = append(results, TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo})
			continue
		}

		// Wait for replication
		time.Sleep(2 * time.Second)

		// Download from target
		downloadStart := time.Now()
		retrievedBlock, err := clients[targetIdx].RetrieveBlockWithCache(cid)
		downloadLatency := time.Since(downloadStart)

		if err != nil {
			if verbose {
				fmt.Printf("    ❌ Download from target failed: %v\n", err)
			}
			results = append(results, TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo})
			continue
		}

		// Verify
		success := equalBytes(retrievedBlock.Data, testData)

		result := TestResult{
			TestName:        testName,
			FileSize:        fileSize,
			DownloadLatency: downloadLatency,
			Success:         success,
			CID:             cid,
			NodeInfo:        nodeInfo,
		}
		results = append(results, result)

		if verbose {
			fmt.Printf("    ✅ Replication: %v, CID: %s\n", downloadLatency, cid[:12]+"...")
		}
	}

	return results
}

func runConcurrentTests(clients []*noisefs.Client, fileSize, numFiles int, verbose bool) []TestResult {
	fmt.Printf("  Running concurrent operations across %d nodes...\n", len(clients))
	
	resultChan := make(chan TestResult, numFiles)
	doneChan := make(chan struct{})

	// Launch concurrent uploads
	for i := 0; i < numFiles; i++ {
		go func(fileIndex int) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Goroutine %d panicked: %v", fileIndex, r)
				}
			}()

			clientIdx := fileIndex % len(clients)
			testData := make([]byte, fileSize)
			rand.Read(testData)

			testName := fmt.Sprintf("concurrent_%d", fileIndex+1)
			nodeInfo := fmt.Sprintf("Node %d", clientIdx+1)

			// Create block
			block, err := blocks.NewBlock(testData)
			if err != nil {
				resultChan <- TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo}
				return
			}

			// Upload
			uploadStart := time.Now()
			cid, err := clients[clientIdx].StoreBlockWithCache(block)
			uploadLatency := time.Since(uploadStart)

			if err != nil {
				resultChan <- TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo}
				return
			}

			// Download
			downloadStart := time.Now()
			retrievedBlock, err := clients[clientIdx].RetrieveBlockWithCache(cid)
			downloadLatency := time.Since(downloadStart)

			if err != nil {
				resultChan <- TestResult{TestName: testName, Success: false, NodeInfo: nodeInfo}
				return
			}

			// Verify
			success := equalBytes(retrievedBlock.Data, testData)

			result := TestResult{
				TestName:        testName,
				FileSize:        fileSize,
				UploadLatency:   uploadLatency,
				DownloadLatency: downloadLatency,
				Success:         success,
				CID:             cid,
				NodeInfo:        nodeInfo,
			}
			resultChan <- result
		}(i)
	}

	// Timeout goroutine
	go func() {
		time.Sleep(2 * time.Minute)
		close(doneChan)
	}()

	// Collect results
	results := make([]TestResult, 0, numFiles)
	collected := 0

	for collected < numFiles {
		select {
		case result := <-resultChan:
			results = append(results, result)
			collected++

			if verbose && collected <= 3 {
				if result.Success {
					fmt.Printf("    Concurrent %d (%s): Upload %v, Download %v\n", 
						collected, result.NodeInfo, result.UploadLatency, result.DownloadLatency)
				}
			}
		case <-doneChan:
			fmt.Printf("    Timeout reached, collected %d/%d results\n", collected, numFiles)
			goto done
		}
	}

done:
	fmt.Printf("  ✅ Concurrent testing completed: %d files processed\n", len(results))
	return results
}

func printSingleNodeResults(results []TestResult, setupTime time.Duration) {
	fmt.Println("\n🎉 Performance Results")
	fmt.Println("======================")
	
	successful := 0
	var totalUpload, totalDownload time.Duration
	var totalBytes int64

	for _, result := range results {
		if result.Success {
			successful++
			totalUpload += result.UploadLatency
			totalDownload += result.DownloadLatency
			totalBytes += int64(result.FileSize)
		}
	}

	fmt.Printf("Files tested: %d\n", len(results))
	fmt.Printf("Successful: %d (%.1f%%)\n", successful, float64(successful)/float64(len(results))*100)
	
	if successful > 0 {
		fmt.Printf("Average upload latency: %v\n", totalUpload/time.Duration(successful))
		fmt.Printf("Average download latency: %v\n", totalDownload/time.Duration(successful))
		fmt.Printf("Total data: %.2f MB\n", float64(totalBytes)/(1024*1024))
		
		avgLatency := (totalUpload + totalDownload) / time.Duration(successful*2)
		fmt.Printf("Overall latency: %v\n", avgLatency)
		
		if avgLatency < 100*time.Millisecond {
			fmt.Println("Performance: ✅ Excellent!")
		} else if avgLatency < 500*time.Millisecond {
			fmt.Println("Performance: ⚠️  Good")
		} else {
			fmt.Println("Performance: ❌ Needs optimization")
		}
	}
}

func printMultiNodeResults(singleResults, crossResults []TestResult, setupTime time.Duration, nodeCount int) {
	fmt.Println("\n🎉 Multi-Node Performance Results")
	fmt.Println("=================================")
	fmt.Printf("Cluster setup time: %v\n", setupTime)
	fmt.Printf("Nodes: %d\n", nodeCount)
	fmt.Println()

	// Single node stats
	fmt.Println("📊 Single Node Performance:")
	printResultStats(singleResults, "  ")

	// Cross-node stats
	if len(crossResults) > 0 {
		fmt.Println("\n🌐 Cross-Node Replication:")
		printResultStats(crossResults, "  ")
	}

	fmt.Printf("\n✨ Multi-node benchmark completed with %d nodes!\n", nodeCount)
}

func printResultStats(results []TestResult, prefix string) {
	successful := 0
	var totalLatency time.Duration

	for _, result := range results {
		if result.Success {
			successful++
			totalLatency += result.UploadLatency + result.DownloadLatency
		}
	}

	fmt.Printf("%sTotal tests: %d\n", prefix, len(results))
	fmt.Printf("%sSuccessful: %d (%.1f%%)\n", prefix, successful, float64(successful)/float64(len(results))*100)
	
	if successful > 0 {
		avgLatency := totalLatency / time.Duration(successful*2)
		fmt.Printf("%sAverage latency: %v\n", prefix, avgLatency)
	}
}

func isIPFSRunning(apiAddr string) bool {
	client, err := ipfs.NewClient(apiAddr)
	if err != nil {
		return false
	}
	
	// Try a simple operation
	defer func() {
		if r := recover(); r != nil {
			// If any panic occurs, IPFS is not properly running
		}
	}()
	
	_ = client.GetConnectedPeers()
	return true
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}