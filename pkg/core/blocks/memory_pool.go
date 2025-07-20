package blocks

import (
	"sync"
	"sync/atomic"
	"time"
)

// BlockInfo contains block metadata for optimization and cache management
type BlockInfo struct {
	CID        string
	Block      *Block
	Size       int
	Popularity int
}

// MemoryPoolManager provides object pooling for frequently allocated structures
type MemoryPoolManager struct {
	// Block data pools for different sizes
	smallBlockPool  sync.Pool // For blocks <= 4KB
	mediumBlockPool sync.Pool // For blocks <= 64KB  
	largeBlockPool  sync.Pool // For blocks <= 1MB
	
	// Structure pools
	blockInfoPool     sync.Pool // For BlockInfo objects
	processorPool     sync.Pool // For FileBlockProcessor objects
	manifestPool      sync.Pool // For DirectoryManifest objects
	
	// Buffer pools
	bufferPool        sync.Pool // For general-purpose byte buffers
	
	// Metrics
	allocations   int64 // Total allocations served
	poolHits      int64 // Allocations served from pool
	poolMisses    int64 // Allocations requiring new objects
}

// GlobalMemoryPool is the singleton instance for memory pooling
var GlobalMemoryPool = NewMemoryPoolManager()

// NewMemoryPoolManager creates a new memory pool manager with optimized pools
func NewMemoryPoolManager() *MemoryPoolManager {
	manager := &MemoryPoolManager{}
	
	// Initialize small block pool (up to 4KB)
	manager.smallBlockPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 4*1024) // 4KB capacity
		},
	}
	
	// Initialize medium block pool (up to 64KB)
	manager.mediumBlockPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 64*1024) // 64KB capacity
		},
	}
	
	// Initialize large block pool (up to 1MB)
	manager.largeBlockPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024*1024) // 1MB capacity
		},
	}
	
	// Initialize structure pools
	manager.blockInfoPool = sync.Pool{
		New: func() interface{} {
			return &BlockInfo{}
		},
	}
	
	manager.processorPool = sync.Pool{
		New: func() interface{} {
			return &FileBlockProcessor{}
		},
	}
	
	manager.manifestPool = sync.Pool{
		New: func() interface{} {
			return NewDirectoryManifest()
		},
	}
	
	// Initialize buffer pool for general use
	manager.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 32*1024) // 32KB default buffer
		},
	}
	
	return manager
}

// GetByteBuffer retrieves a byte buffer from the appropriate pool based on required size
func (mp *MemoryPoolManager) GetByteBuffer(size int) []byte {
	atomic.AddInt64(&mp.allocations, 1)
	
	var buffer []byte
	
	switch {
	case size <= 4*1024:
		// Use small block pool
		buffer = mp.smallBlockPool.Get().([]byte)
		atomic.AddInt64(&mp.poolHits, 1)
	case size <= 64*1024:
		// Use medium block pool
		buffer = mp.mediumBlockPool.Get().([]byte)
		atomic.AddInt64(&mp.poolHits, 1)
	case size <= 1024*1024:
		// Use large block pool
		buffer = mp.largeBlockPool.Get().([]byte)
		atomic.AddInt64(&mp.poolHits, 1)
	default:
		// Size too large for pooling, allocate directly
		buffer = make([]byte, 0, size)
		atomic.AddInt64(&mp.poolMisses, 1)
	}
	
	// Ensure buffer has adequate capacity
	if cap(buffer) < size {
		// Pool buffer too small, allocate new one
		buffer = make([]byte, 0, size)
		atomic.AddInt64(&mp.poolMisses, 1)
	} else {
		// Reset buffer length while preserving capacity
		buffer = buffer[:0]
	}
	
	return buffer
}

// ReturnByteBuffer returns a byte buffer to the appropriate pool
func (mp *MemoryPoolManager) ReturnByteBuffer(buffer []byte) {
	// Clear sensitive data
	for i := range buffer {
		buffer[i] = 0
	}
	
	// Reset length while preserving capacity
	buffer = buffer[:0]
	
	// Return to appropriate pool based on capacity
	switch cap(buffer) {
	case 4 * 1024:
		mp.smallBlockPool.Put(buffer)
	case 64 * 1024:
		mp.mediumBlockPool.Put(buffer)
	case 1024 * 1024:
		mp.largeBlockPool.Put(buffer)
	default:
		// Non-standard size, let GC handle it
	}
}

// GetBlockInfo retrieves a BlockInfo structure from the pool
func (mp *MemoryPoolManager) GetBlockInfo() *BlockInfo {
	atomic.AddInt64(&mp.allocations, 1)
	atomic.AddInt64(&mp.poolHits, 1)
	
	info := mp.blockInfoPool.Get().(*BlockInfo)
	
	// Reset fields
	info.CID = ""
	info.Block = nil
	info.Size = 0
	info.Popularity = 0
	
	return info
}

// ReturnBlockInfo returns a BlockInfo structure to the pool
func (mp *MemoryPoolManager) ReturnBlockInfo(info *BlockInfo) {
	if info == nil {
		return
	}
	
	// Clear sensitive data
	info.CID = ""
	info.Block = nil
	info.Size = 0
	info.Popularity = 0
	
	mp.blockInfoPool.Put(info)
}

// GetFileBlockProcessor retrieves a FileBlockProcessor from the pool
func (mp *MemoryPoolManager) GetFileBlockProcessor() *FileBlockProcessor {
	atomic.AddInt64(&mp.allocations, 1)
	atomic.AddInt64(&mp.poolHits, 1)
	
	processor := mp.processorPool.Get().(*FileBlockProcessor)
	
	// Reset processor state (implement Reset method on FileBlockProcessor)
	// processor.Reset()
	
	return processor
}

// ReturnFileBlockProcessor returns a FileBlockProcessor to the pool
func (mp *MemoryPoolManager) ReturnFileBlockProcessor(processor *FileBlockProcessor) {
	if processor == nil {
		return
	}
	
	// Reset processor state
	// processor.Reset()
	
	mp.processorPool.Put(processor)
}

// GetDirectoryManifest retrieves a DirectoryManifest from the pool
func (mp *MemoryPoolManager) GetDirectoryManifest() *DirectoryManifest {
	atomic.AddInt64(&mp.allocations, 1)
	atomic.AddInt64(&mp.poolHits, 1)
	
	manifest := mp.manifestPool.Get().(*DirectoryManifest)
	
	// Reset manifest state (clear entries, reset timestamps)
	manifest.Entries = manifest.Entries[:0]
	manifest.CreatedAt = time.Now()
	manifest.ModifiedAt = time.Now()
	manifest.SnapshotInfo = nil
	
	return manifest
}

// ReturnDirectoryManifest returns a DirectoryManifest to the pool
func (mp *MemoryPoolManager) ReturnDirectoryManifest(manifest *DirectoryManifest) {
	if manifest == nil {
		return
	}
	
	// Clear sensitive data
	manifest.Entries = manifest.Entries[:0]
	manifest.SnapshotInfo = nil
	
	mp.manifestPool.Put(manifest)
}

// GetGeneralBuffer retrieves a general-purpose buffer from the pool
func (mp *MemoryPoolManager) GetGeneralBuffer() []byte {
	atomic.AddInt64(&mp.allocations, 1)
	atomic.AddInt64(&mp.poolHits, 1)
	
	buffer := mp.bufferPool.Get().([]byte)
	return buffer[:0] // Reset length
}

// ReturnGeneralBuffer returns a general-purpose buffer to the pool
func (mp *MemoryPoolManager) ReturnGeneralBuffer(buffer []byte) {
	// Clear sensitive data
	for i := range buffer {
		buffer[i] = 0
	}
	
	mp.bufferPool.Put(buffer[:0])
}

// GetPoolMetrics returns current memory pool performance metrics
func (mp *MemoryPoolManager) GetPoolMetrics() MemoryPoolMetrics {
	allocations := atomic.LoadInt64(&mp.allocations)
	hits := atomic.LoadInt64(&mp.poolHits)
	misses := atomic.LoadInt64(&mp.poolMisses)
	
	hitRate := float64(0)
	if allocations > 0 {
		hitRate = float64(hits) / float64(allocations) * 100
	}
	
	return MemoryPoolMetrics{
		TotalAllocations: allocations,
		PoolHits:        hits,
		PoolMisses:      misses,
		HitRate:         hitRate,
	}
}

// MemoryPoolMetrics contains performance metrics for memory pools
type MemoryPoolMetrics struct {
	TotalAllocations int64   // Total allocation requests
	PoolHits        int64   // Requests served from pool
	PoolMisses      int64   // Requests requiring new allocation
	HitRate         float64 // Pool hit rate percentage
}

// OptimizedBlockAllocation demonstrates optimized block allocation pattern
func OptimizedBlockAllocation(size int) ([]byte, func()) {
	// Get buffer from pool
	buffer := GlobalMemoryPool.GetByteBuffer(size)
	
	// Return cleanup function
	cleanup := func() {
		GlobalMemoryPool.ReturnByteBuffer(buffer)
	}
	
	return buffer, cleanup
}

// OptimizedBlockInfoAllocation demonstrates optimized BlockInfo allocation
func OptimizedBlockInfoAllocation() (*BlockInfo, func()) {
	// Get BlockInfo from pool
	info := GlobalMemoryPool.GetBlockInfo()
	
	// Return cleanup function
	cleanup := func() {
		GlobalMemoryPool.ReturnBlockInfo(info)
	}
	
	return info, cleanup
}

// OptimizedManifestAllocation demonstrates optimized DirectoryManifest allocation
func OptimizedManifestAllocation() (*DirectoryManifest, func()) {
	// Get manifest from pool
	manifest := GlobalMemoryPool.GetDirectoryManifest()
	
	// Return cleanup function
	cleanup := func() {
		GlobalMemoryPool.ReturnDirectoryManifest(manifest)
	}
	
	return manifest, cleanup
}