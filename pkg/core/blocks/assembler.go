package blocks

import (
	"bytes"
	"errors"
	"io"
)

// Assembler handles reconstruction of files from blocks
type Assembler struct{}

// NewAssembler creates a new file assembler
func NewAssembler() *Assembler {
	return &Assembler{}
}

// Assemble reconstructs data from blocks
func (a *Assembler) Assemble(blocks []*Block) ([]byte, error) {
	if len(blocks) == 0 {
		return nil, errors.New("no blocks to assemble")
	}
	
	var buffer bytes.Buffer
	
	for _, block := range blocks {
		if block == nil {
			return nil, errors.New("nil block in assembly")
		}
		
		if _, err := buffer.Write(block.Data); err != nil {
			return nil, err
		}
	}
	
	return buffer.Bytes(), nil
}

// AssembleToWriter reconstructs data from blocks and writes to an io.Writer
func (a *Assembler) AssembleToWriter(blocks []*Block, writer io.Writer) error {
	if len(blocks) == 0 {
		return errors.New("no blocks to assemble")
	}
	
	if writer == nil {
		return errors.New("writer cannot be nil")
	}
	
	for _, block := range blocks {
		if block == nil {
			return errors.New("nil block in assembly")
		}
		
		if _, err := writer.Write(block.Data); err != nil {
			return err
		}
	}
	
	return nil
}

// StreamingAssembler handles reconstruction of files from blocks with streaming support
type StreamingAssembler struct {
	writer io.Writer
}

// NewStreamingAssembler creates a new streaming file assembler
func NewStreamingAssembler(writer io.Writer) (*StreamingAssembler, error) {
	if writer == nil {
		return nil, errors.New("writer cannot be nil")
	}
	
	return &StreamingAssembler{
		writer: writer,
	}, nil
}

// StreamingAssemblyProgressCallback is called during streaming assembly
type StreamingAssemblyProgressCallback func(blocksProcessed int, bytesWritten int64)

// ProcessBlockWithXOR reconstructs a block from XOR operation and writes it immediately
func (sa *StreamingAssembler) ProcessBlockWithXOR(dataBlock, randBlock1, randBlock2 *Block) error {
	if dataBlock == nil || randBlock1 == nil || randBlock2 == nil {
		return errors.New("all blocks must be non-nil for XOR operation")
	}
	
	// Perform XOR to reconstruct original block
	origBlock, err := dataBlock.XOR(randBlock1, randBlock2)
	if err != nil {
		return err
	}
	
	// Write immediately to output
	_, err = sa.writer.Write(origBlock.Data)
	return err
}

