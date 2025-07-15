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


