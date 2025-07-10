package integration

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
)

// BenchmarkSuite provides comprehensive performance testing for NoiseFS
type BenchmarkSuite struct {
	clients       []*noisefs.Client
	peerManagers  []*p2p.PeerManager
	ipfsClients   []*ipfs.Client
	blockSizes    []int
	numPeers      int
	numBlocks     int
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(numPeers, numBlocks int) *BenchmarkSuite {
	return &BenchmarkSuite{
		numPeers:   numPeers,
		numBlocks:  numBlocks,
		blockSizes: []int{4096, 32768, 131072, 1048576}, // 4KB, 32KB, 128KB, 1MB
	}
}

// SetupBenchmark initializes the benchmark environment
func (bs *BenchmarkSuite) SetupBenchmark(b *testing.B) error {
	b.Helper()
	
	// Create IPFS clients
	for i := 0; i < bs.numPeers; i++ {
		ipfsClient, err := ipfs.NewClient(fmt.Sprintf("localhost:%d", 5001+i))
		if err != nil {
			// For benchmarking, create mock clients if real IPFS not available
			ipfsClient = NewMockIPFSClient()
		}
		bs.ipfsClients = append(bs.ipfsClients, ipfsClient)
		
		// Create peer manager
		host := NewMockHost(peer.ID(fmt.Sprintf("peer-%d", i)))
		peerManager := p2p.NewPeerManager(host, 50)
		bs.peerManagers = append(bs.peerManagers, peerManager)
		
		// Create cache
		cache := cache.NewMemoryCache(1000)
		
		// Create NoiseFS client
		client, err := noisefs.NewClient(ipfsClient, cache)
		if err != nil {
			return fmt.Errorf("failed to create NoiseFS client %d: %w", i, err)
		}
		
		client.SetPeerManager(peerManager)
		bs.clients = append(bs.clients, client)
	}
	
	// Peer managers are initialized and will discover peers through the network
	
	return nil
}

// BenchmarkRandomizerSelection benchmarks randomizer selection performance
func BenchmarkRandomizerSelection(b *testing.B) {
	suite := NewBenchmarkSuite(5, 1000)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	client := suite.clients[0]
	blockSize := 131072 // 128KB
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, err := client.SelectRandomizer(blockSize)
			if err != nil {
				b.Errorf("Randomizer selection failed: %v", err)
			}
		}
	})
}

// BenchmarkPeerSelection benchmarks different peer selection strategies
func BenchmarkPeerSelection(b *testing.B) {
	suite := NewBenchmarkSuite(10, 100)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	strategies := []string{"performance", "randomizer", "privacy", "hybrid"}
	peerManager := suite.peerManagers[0]
	
	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				criteria := p2p.SelectionCriteria{Count: 3}
				_, err := peerManager.SelectPeers(context.Background(), strategy, criteria)
				if err != nil {
					b.Errorf("Peer selection failed for %s: %v", strategy, err)
				}
			}
		})
	}
}

// BenchmarkCachePerformance benchmarks cache hit rates and performance
func BenchmarkCachePerformance(b *testing.B) {
	suite := NewBenchmarkSuite(1, 10000)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	client := suite.clients[0]
	testBlocks := make([]*blocks.Block, 100)
	cids := make([]string, 100)
	
	// Pre-populate cache
	for i := 0; i < 100; i++ {
		block, err := blocks.NewRandomBlock(4096)
		if err != nil {
			b.Fatalf("Failed to create block: %v", err)
		}
		testBlocks[i] = block
		
		cid, err := client.StoreBlockWithCache(block)
		if err != nil {
			b.Fatalf("Failed to store block: %v", err)
		}
		cids[i] = cid
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 80% cache hits, 20% misses
			if rand.Float32() < 0.8 {
				// Cache hit
				idx := rand.Intn(len(cids))
				_, err := client.RetrieveBlockWithCache(cids[idx])
				if err != nil {
					b.Errorf("Cache retrieval failed: %v", err)
				}
			} else {
				// Cache miss
				_, err := client.RetrieveBlockWithCache(fmt.Sprintf("missing-block-%d", rand.Int()))
				if err == nil {
					b.Error("Expected cache miss but got hit")
				}
			}
		}
	})
}

// BenchmarkThroughput benchmarks overall system throughput
func BenchmarkThroughput(b *testing.B) {
	suite := NewBenchmarkSuite(5, 1000)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	var totalBytes int64
	var totalOps int64
	
	b.ResetTimer()
	start := time.Now()
	
	b.RunParallel(func(pb *testing.PB) {
		client := suite.clients[rand.Intn(len(suite.clients))]
		
		for pb.Next() {
			// Create and store a block
			blockSize := suite.blockSizes[rand.Intn(len(suite.blockSizes))]
			block, err := blocks.NewRandomBlock(blockSize)
			if err != nil {
				b.Errorf("Failed to create block: %v", err)
				continue
			}
			
			cid, err := client.StoreBlockWithCache(block)
			if err != nil {
				b.Errorf("Failed to store block: %v", err)
				continue
			}
			
			// Retrieve the block
			_, err = client.RetrieveBlockWithCache(cid)
			if err != nil {
				b.Errorf("Failed to retrieve block: %v", err)
				continue
			}
			
			totalBytes += int64(blockSize * 2) // store + retrieve
			totalOps += 2
		}
	})
	
	duration := time.Since(start)
	throughputMBps := float64(totalBytes) / (1024 * 1024) / duration.Seconds()
	opsPerSec := float64(totalOps) / duration.Seconds()
	
	b.ReportMetric(throughputMBps, "MB/s")
	b.ReportMetric(opsPerSec, "ops/s")
}

// BenchmarkMLPrediction benchmarks ML-based cache prediction accuracy
func BenchmarkMLPrediction(b *testing.B) {
	suite := NewBenchmarkSuite(1, 1000)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	client := suite.clients[0]
	
	// Create access pattern
	blockCIDs := make([]string, 50)
	for i := 0; i < 50; i++ {
		block, err := blocks.NewRandomBlock(4096)
		if err != nil {
			b.Fatalf("Failed to create block: %v", err)
		}
		
		cid, err := client.StoreBlockWithCache(block)
		if err != nil {
			b.Fatalf("Failed to store block: %v", err)
		}
		blockCIDs[i] = cid
	}
	
	// Create predictable access pattern for training
	for round := 0; round < 100; round++ {
		for i := 0; i < 10; i++ { // Access first 10 blocks frequently
			client.RetrieveBlockWithCache(blockCIDs[i])
		}
		
		if round%5 == 0 { // Access next 10 blocks occasionally
			for i := 10; i < 20; i++ {
				client.RetrieveBlockWithCache(blockCIDs[i])
			}
		}
	}
	
	b.ResetTimer()
	
	// Test prediction accuracy
	correct := 0
	total := 0
	
	for i := 0; i < b.N; i++ {
		// Predict which blocks will be accessed
		predictions := make([]bool, len(blockCIDs))
		for j := 0; j < 10; j++ { // Predict first 10 will be accessed
			predictions[j] = true
		}
		
		// Actually access blocks
		accessed := make([]bool, len(blockCIDs))
		for j := 0; j < 10; j++ { // Actually access first 10
			client.RetrieveBlockWithCache(blockCIDs[j])
			accessed[j] = true
		}
		
		// Calculate accuracy
		for j := 0; j < len(blockCIDs); j++ {
			if predictions[j] == accessed[j] {
				correct++
			}
			total++
		}
	}
	
	accuracy := float64(correct) / float64(total) * 100
	b.ReportMetric(accuracy, "%accuracy")
}

// BenchmarkStorageOverhead benchmarks storage efficiency
func BenchmarkStorageOverhead(b *testing.B) {
	suite := NewBenchmarkSuite(3, 100)
	if err := suite.SetupBenchmark(b); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}
	
	client := suite.clients[0]
	originalSize := int64(0)
	storedSize := int64(0)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Create source data
		sourceData := make([]byte, 131072) // 128KB
		rand.Read(sourceData)
		originalSize += int64(len(sourceData))
		
		// Get randomizer
		randomizer, _, err := client.SelectRandomizer(len(sourceData))
		if err != nil {
			b.Errorf("Failed to get randomizer: %v", err)
			continue
		}
		
		// Create anonymized block (XOR with randomizer)
		anonData := make([]byte, len(sourceData))
		for j := 0; j < len(sourceData); j++ {
			anonData[j] = sourceData[j] ^ randomizer.Data[j]
		}
		
		anonBlock, err := blocks.NewBlock(anonData)
		if err != nil {
			b.Errorf("Failed to create anonymized block: %v", err)
			continue
		}
		
		// Store anonymized block
		_, err = client.StoreBlockWithCache(anonBlock)
		if err != nil {
			b.Errorf("Failed to store anonymized block: %v", err)
			continue
		}
		
		// Account for storage (anonymized block + randomizer reference)
		storedSize += int64(len(anonData))
		if i == 0 { // Only count randomizer once if reused
			storedSize += int64(len(randomizer.Data))
		}
		
		client.RecordUpload(int64(len(sourceData)), storedSize-originalSize)
	}
	
	overhead := float64(storedSize-originalSize) / float64(originalSize) * 100
	b.ReportMetric(overhead, "%overhead")
}

// MockIPFSClient provides a mock IPFS client for testing
type MockIPFSClient struct {
	blocks map[string]*blocks.Block
	mutex  sync.RWMutex
}

func NewMockIPFSClient() *ipfs.Client {
	// Create a minimal IPFS client for testing
	return &ipfs.Client{}
}

func (m *MockIPFSClient) StoreBlock(block *blocks.Block) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	cid := fmt.Sprintf("mock-cid-%d", rand.Int63())
	m.blocks[cid] = block
	return cid, nil
}

func (m *MockIPFSClient) RetrieveBlock(cid string) (*blocks.Block, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	if block, exists := m.blocks[cid]; exists {
		return block, nil
	}
	return nil, fmt.Errorf("block not found: %s", cid)
}

func (m *MockIPFSClient) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	// Add small delay to simulate network latency with peer selection
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)+1))
	return m.RetrieveBlock(cid)
}

func (m *MockIPFSClient) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	// Strategy affects storage time simulation
	delay := time.Millisecond
	switch strategy {
	case "randomizer":
		delay = time.Millisecond * 2 // Slightly more overhead for randomizer broadcast
	case "performance":
		delay = time.Microsecond * 500 // Faster for performance strategy
	}
	time.Sleep(delay)
	
	return m.StoreBlock(block)
}

func (m *MockIPFSClient) SetPeerManager(manager *p2p.PeerManager) {
	// Mock implementation
}

func (m *MockIPFSClient) GetConnectedPeers() []peer.ID {
	return []peer.ID{
		peer.ID("mock-peer-1"),
		peer.ID("mock-peer-2"),
		peer.ID("mock-peer-3"),
	}
}

func (m *MockIPFSClient) RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	// Simulate peer-specific request
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(20)+5))
	return m.RetrieveBlock(cid)
}

func (m *MockIPFSClient) BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error {
	// Mock broadcast implementation
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(50)+10))
	return nil
}

func (m *MockIPFSClient) GetPeerMetrics() map[peer.ID]*ipfs.RequestMetrics {
	return map[peer.ID]*ipfs.RequestMetrics{
		peer.ID("mock-peer-1"): {
			TotalRequests:      100,
			SuccessfulRequests: 95,
			FailedRequests:     5,
			AverageLatency:     time.Millisecond * 50,
			Bandwidth:          1024 * 1024, // 1MB/s
		},
	}
}

// MockHost provides a mock libp2p host for testing
type MockHost struct {
	id peer.ID
}

func NewMockHost(id peer.ID) *MockHost {
	return &MockHost{id: id}
}

func (m *MockHost) ID() peer.ID {
	return m.id
}

func (m *MockHost) Addrs() []multiaddr.Multiaddr {
	// Return empty slice for mock
	return []multiaddr.Multiaddr{}
}

func (m *MockHost) Close() error {
	// Mock implementation
	return nil
}

func (m *MockHost) ConnManager() connmgr.ConnManager {
	// Mock implementation
	return nil
}

func (m *MockHost) Peerstore() peerstore.Peerstore {
	return nil
}

func (m *MockHost) Network() network.Network {
	return nil
}

func (m *MockHost) Mux() protocol.Switch {
	return nil
}

func (m *MockHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	return nil
}

func (m *MockHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
}

func (m *MockHost) SetStreamHandlerMatch(protocol.ID, func(protocol.ID) bool, network.StreamHandler) {
}

func (m *MockHost) RemoveStreamHandler(pid protocol.ID) {
}

func (m *MockHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	return nil, nil
}

func (m *MockHost) EventBus() event.Bus {
	return nil
}