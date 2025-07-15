package blocks

import (
	"errors"
	"io"
)

// Splitter handles file splitting into blocks
type Splitter struct {
	blockSize int
}

// NewSplitter creates a new file splitter
func NewSplitter(blockSize int) (*Splitter, error) {
	if blockSize <= 0 {
		return nil, errors.New("block size must be positive")
	}
	
	return &Splitter{
		blockSize: blockSize,
	}, nil
}

// Split splits data from a reader into blocks
func (s *Splitter) Split(reader io.Reader) ([]*Block, error) {
	if reader == nil {
		return nil, errors.New("reader cannot be nil")
	}
	
	var blocks []*Block
	buffer := make([]byte, s.blockSize)
	
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			// Create a new block with the data read
			blockData := make([]byte, n)
			copy(blockData, buffer[:n])
			
			block, err := NewBlock(blockData)
			if err != nil {
				return nil, err
			}
			
			blocks = append(blocks, block)
		}
		
		if err == io.EOF {
			break
		}
		
		if err != nil {
			return nil, err
		}
	}
	
	return blocks, nil
}

// SplitBytes splits a byte slice into blocks
func (s *Splitter) SplitBytes(data []byte) ([]*Block, error) {
	if len(data) == 0 {
		return nil, errors.New("data cannot be empty")
	}
	
	var blocks []*Block
	
	for i := 0; i < len(data); i += s.blockSize {
		end := i + s.blockSize
		if end > len(data) {
			end = len(data)
		}
		
		block, err := NewBlock(data[i:end])
		if err != nil {
			return nil, err
		}
		
		blocks = append(blocks, block)
	}
	
	return blocks, nil
}

// DefaultSplitter returns a splitter with the default block size
func DefaultSplitter() *Splitter {
	splitter, _ := NewSplitter(DefaultBlockSize)
	return splitter
}

// StreamingSplitter handles streaming file splitting with constant memory usage
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

// StreamingProgressCallback is called during streaming operations to report progress
type StreamingProgressCallback func(bytesProcessed int64, blocksProcessed int)

// StreamBlocks processes blocks from a stream with progress reporting
func (s *StreamingSplitter) StreamBlocks(reader io.Reader, 
	blockCallback func(*Block) error, 
	progressCallback StreamingProgressCallback) error {
	
	if reader == nil {
		return errors.New("reader cannot be nil")
	}
	
	buffer := make([]byte, s.blockSize)
	var bytesProcessed int64
	var blocksProcessed int
	
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			// Create a new block with the data read
			blockData := make([]byte, n)
			copy(blockData, buffer[:n])
			
			block, err := NewBlock(blockData)
			if err != nil {
				return err
			}
			
			// Process the block through callback
			if err := blockCallback(block); err != nil {
				return err
			}
			
			// Update progress
			bytesProcessed += int64(n)
			blocksProcessed++
			
			if progressCallback != nil {
				progressCallback(bytesProcessed, blocksProcessed)
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

