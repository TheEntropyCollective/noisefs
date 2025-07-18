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

// Split splits data from a reader into blocks with padding for optimal cache efficiency
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
			blockData := make([]byte, s.blockSize) // Always allocate full block size
			copy(blockData, buffer[:n])
			// Remaining bytes are zero-padded automatically
			
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

// SplitBytes splits a byte slice into blocks with padding for optimal cache efficiency
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
		
		// Always create full-sized blocks with padding
		blockData := make([]byte, s.blockSize)
		copy(blockData, data[i:end])
		// Remaining bytes are zero-padded automatically
		
		block, err := NewBlock(blockData)
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


