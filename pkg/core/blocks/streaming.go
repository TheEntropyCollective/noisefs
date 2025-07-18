package blocks

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
)

// BlockProcessor is a callback interface for processing blocks as they are created
type BlockProcessor interface {
	// ProcessBlock is called for each block as it's created during streaming
	ProcessBlock(blockIndex int, block *Block) error
}

// ProgressCallback is called to report streaming progress
type ProgressCallback func(bytesProcessed int64, blocksProcessed int)

// StreamingSplitter handles file splitting into blocks from an io.Reader without buffering entire file
type StreamingSplitter struct {
	blockSize int
}

// NewStreamingSplitter creates a new streaming file splitter
func NewStreamingSplitter(blockSize int) (*StreamingSplitter, error) {
	if blockSize <= 0 {
		return nil, errors.New("block size must be positive")
	}
	
	return &StreamingSplitter{
		blockSize: blockSize,
	}, nil
}

// Split splits data from a reader into blocks using callback-based processing
func (s *StreamingSplitter) Split(reader io.Reader, processor BlockProcessor) error {
	return s.SplitWithContext(context.Background(), reader, processor)
}

// SplitWithContext splits data from a reader into blocks with context support
func (s *StreamingSplitter) SplitWithContext(ctx context.Context, reader io.Reader, processor BlockProcessor) error {
	if reader == nil {
		return errors.New("reader cannot be nil")
	}
	
	if processor == nil {
		return errors.New("processor cannot be nil")
	}
	
	buffer := make([]byte, s.blockSize)
	blockIndex := 0
	
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		n, err := reader.Read(buffer)
		if n > 0 {
			// Always create full-sized blocks with padding for optimal cache efficiency
			blockData := make([]byte, s.blockSize)
			copy(blockData, buffer[:n])
			// Remaining bytes are zero-padded automatically
			
			block, blockErr := NewBlock(blockData)
			if blockErr != nil {
				return blockErr
			}
			
			if procErr := processor.ProcessBlock(blockIndex, block); procErr != nil {
				return procErr
			}
			
			blockIndex++
		}
		
		if err == io.EOF {
			break
		}
		
		if err != nil {
			return err
		}
	}
	
	return nil
}

// SplitWithProgress splits data with progress reporting
func (s *StreamingSplitter) SplitWithProgress(reader io.Reader, processor BlockProcessor, progress ProgressCallback) error {
	return s.SplitWithProgressAndContext(context.Background(), reader, processor, progress)
}

// SplitWithProgressAndContext splits data with progress reporting and context support
func (s *StreamingSplitter) SplitWithProgressAndContext(ctx context.Context, reader io.Reader, processor BlockProcessor, progress ProgressCallback) error {
	if reader == nil {
		return errors.New("reader cannot be nil")
	}
	
	if processor == nil {
		return errors.New("processor cannot be nil")
	}
	
	buffer := make([]byte, s.blockSize)
	blockIndex := 0
	bytesProcessed := int64(0)
	
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		
		n, err := reader.Read(buffer)
		if n > 0 {
			// Always create full-sized blocks with padding for optimal cache efficiency
			blockData := make([]byte, s.blockSize)
			copy(blockData, buffer[:n])
			// Remaining bytes are zero-padded automatically
			
			block, blockErr := NewBlock(blockData)
			if blockErr != nil {
				return blockErr
			}
			
			if procErr := processor.ProcessBlock(blockIndex, block); procErr != nil {
				return procErr
			}
			
			blockIndex++
			bytesProcessed += int64(n)
			
			if progress != nil {
				progress(bytesProcessed, blockIndex)
			}
		}
		
		if err == io.EOF {
			break
		}
		
		if err != nil {
			return err
		}
	}
	
	return nil
}

// StreamingAssembler handles reconstruction of files from blocks with out-of-order arrival support
type StreamingAssembler struct {
	writer        io.Writer
	blockBuffer   map[int]*Block
	nextIndex     int
	totalBlocks   int
	writtenBlocks int
	mutex         sync.RWMutex
	complete      bool
}

// NewStreamingAssembler creates a new streaming file assembler
func NewStreamingAssembler(writer io.Writer) (*StreamingAssembler, error) {
	if writer == nil {
		return nil, errors.New("writer cannot be nil")
	}
	
	return &StreamingAssembler{
		writer:        writer,
		blockBuffer:   make(map[int]*Block),
		nextIndex:     0,
		totalBlocks:   -1,
		writtenBlocks: 0,
		complete:      false,
	}, nil
}

// SetTotalBlocks sets the expected total number of blocks
func (a *StreamingAssembler) SetTotalBlocks(total int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.totalBlocks = total
}

// AddBlock adds a block to the assembler and writes any sequential blocks that are ready
func (a *StreamingAssembler) AddBlock(blockIndex int, block *Block) error {
	if block == nil {
		return errors.New("block cannot be nil")
	}
	
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	if a.complete {
		return errors.New("assembly already complete")
	}
	
	if blockIndex < 0 {
		return errors.New("block index cannot be negative")
	}
	
	if _, exists := a.blockBuffer[blockIndex]; exists {
		return errors.New("block already exists")
	}
	
	a.blockBuffer[blockIndex] = block
	return a.writeSequentialBlocks()
}

// writeSequentialBlocks writes blocks starting from nextIndex while they're available
func (a *StreamingAssembler) writeSequentialBlocks() error {
	for {
		block, exists := a.blockBuffer[a.nextIndex]
		if !exists {
			break
		}
		
		if _, err := a.writer.Write(block.Data); err != nil {
			return err
		}
		
		delete(a.blockBuffer, a.nextIndex)
		a.nextIndex++
		a.writtenBlocks++
		
		if a.totalBlocks > 0 && a.writtenBlocks >= a.totalBlocks {
			a.complete = true
			break
		}
	}
	
	return nil
}

// IsComplete returns whether all blocks have been written
func (a *StreamingAssembler) IsComplete() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.complete
}

// Finalize completes the assembly by writing any remaining buffered blocks in order
func (a *StreamingAssembler) Finalize() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	if a.complete {
		return nil
	}
	
	indices := make([]int, 0, len(a.blockBuffer))
	for index := range a.blockBuffer {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	
	for _, index := range indices {
		block := a.blockBuffer[index]
		if _, err := a.writer.Write(block.Data); err != nil {
			return err
		}
		delete(a.blockBuffer, index)
		a.writtenBlocks++
	}
	
	a.complete = true
	return nil
}

// RandomizerProvider provides randomizer blocks for XOR operations during streaming
type RandomizerProvider interface {
	GetRandomizers(blockIndex int) (*Block, *Block, error)
}

// StreamingXORProcessor processes blocks with XOR operations during streaming upload
type StreamingXORProcessor struct {
	provider   RandomizerProvider
	downstream BlockProcessor
}

// NewStreamingXORProcessor creates a new streaming XOR processor
func NewStreamingXORProcessor(provider RandomizerProvider, downstream BlockProcessor) (*StreamingXORProcessor, error) {
	if provider == nil {
		return nil, errors.New("randomizer provider cannot be nil")
	}
	
	if downstream == nil {
		return nil, errors.New("downstream processor cannot be nil")
	}
	
	return &StreamingXORProcessor{
		provider:   provider,
		downstream: downstream,
	}, nil
}

// ProcessBlock implements BlockProcessor interface with XOR operations
func (p *StreamingXORProcessor) ProcessBlock(blockIndex int, block *Block) error {
	if block == nil {
		return errors.New("block cannot be nil")
	}
	
	randomizer1, randomizer2, err := p.provider.GetRandomizers(blockIndex)
	if err != nil {
		return err
	}
	
	xorBlock, err := block.XOR(randomizer1, randomizer2)
	if err != nil {
		return err
	}
	
	return p.downstream.ProcessBlock(blockIndex, xorBlock)
}

// SimpleRandomizerProvider provides the same randomizers for all blocks
type SimpleRandomizerProvider struct {
	randomizer1 *Block
	randomizer2 *Block
}

// NewSimpleRandomizerProvider creates a simple provider with fixed randomizers
func NewSimpleRandomizerProvider(randomizer1, randomizer2 *Block) (*SimpleRandomizerProvider, error) {
	if randomizer1 == nil || randomizer2 == nil {
		return nil, errors.New("randomizers cannot be nil")
	}
	
	return &SimpleRandomizerProvider{
		randomizer1: randomizer1,
		randomizer2: randomizer2,
	}, nil
}

// GetRandomizers returns the fixed randomizers for any block index
func (p *SimpleRandomizerProvider) GetRandomizers(blockIndex int) (*Block, *Block, error) {
	return p.randomizer1, p.randomizer2, nil
}