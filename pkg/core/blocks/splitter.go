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


